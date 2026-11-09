package handler_test

import (
	"appointment_system/internal/apps/appointment/adapters/callhub/memory"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/call"
	"appointment_system/internal/apps/appointment/handler"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/apps/appointment/utils"
	"appointment_system/middleware"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const testCallSecret = "0123456789abcdef0123456789abcdef"

type callHandlerFixture struct {
	now          time.Time
	appointments *fakeAppointmentRepo
	doctors      *fakeDoctorRepo
	userPatients *fakeUserPatientRepo
	rooms        *fakeCallRoomRepo
	participants *fakeCallParticipantRepo
}

func newCallHandlerFixture() *callHandlerFixture {
	now := time.Now().UTC()

	return &callHandlerFixture{
		now: now,
		appointments: &fakeAppointmentRepo{byID: &models.AppointmentDetails{Appointment: models.Appointment{
			ID:            5,
			PatientID:     3,
			DoctorID:      2,
			StartsAt:      now,
			EndsAt:        now.Add(30 * time.Minute),
			Status:        models.AppointmentStatusScheduled,
			PaymentStatus: models.AppointmentPaymentPaid,
		}}},
		doctors: &fakeDoctorRepo{doctor: &models.Doctor{ID: 2, UserID: 11, VerificationStatus: models.VerificationStatusVerified}},
		userPatients: &fakeUserPatientRepo{userPatient: &models.UserPatient{
			UserID:              7,
			PatientID:           3,
			PatientRelationship: models.RelationshipMother,
			AccessLevel:         models.AccessLevelFull,
			Status:              models.UserPatientStatusActive,
		}},
		rooms:        &fakeCallRoomRepo{},
		participants: &fakeCallParticipantRepo{},
	}
}

func newCallRouter(t *testing.T, f *callHandlerFixture) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tokens, err := utils.NewCallTokenManager(testCallSecret, "test-issuer", constant.CallTokenTTL)
	if err != nil {
		t.Fatalf("NewCallTokenManager returned error: %v", err)
	}
	hub := memory.New()
	ice := services.ICESettings{STUNURLs: []string{"stun:stun.example.test:3478"}}
	clock := func() time.Time { return f.now }
	events := &fakeCallEventRepo{}
	attempts := &fakeCallJoinAttemptRepo{}

	callService, err := services.NewCallService(f.appointments, f.doctors, f.userPatients, &fakeUserRepo{}, f.rooms, f.participants, &fakeCallGuestTokenRepo{}, events, attempts, &fakeOutboxRepo{}, &fakeTxManager{}, tokens, hub, ice, clock, discardLogger)
	if err != nil {
		t.Fatalf("NewCallService returned error: %v", err)
	}
	codeRoomService, err := services.NewCodeRoomService(f.doctors, &fakeUserRepo{}, f.rooms, f.participants, events, attempts, &fakeOutboxRepo{}, &fakeTxManager{}, tokens, hub, ice, clock, discardLogger)
	if err != nil {
		t.Fatalf("NewCodeRoomService returned error: %v", err)
	}
	sessionService, err := services.NewCallSessionService(f.rooms, f.participants, events, &fakeOutboxRepo{}, &fakeTxManager{}, tokens, hub, clock, discardLogger)
	if err != nil {
		t.Fatalf("NewCallSessionService returned error: %v", err)
	}
	callController, err := controller.NewCallController(callService, codeRoomService, sessionService)
	if err != nil {
		t.Fatalf("NewCallController returned error: %v", err)
	}
	callHandler, err := handler.NewCallHandler(callController, &fakeRoleTokenVerifier{}, discardLogger)
	if err != nil {
		t.Fatalf("NewCallHandler returned error: %v", err)
	}

	router := gin.New()
	router.Use(middleware.AppContext())
	callHandler.RegisterRoutes(router.Group("/api/v1"))

	return router
}

func serveCall(router http.Handler, method string, path string, token string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	return response
}

// unknownJoinCode is a well-formed code no room has.
func unknownJoinCode(t *testing.T) string {
	t.Helper()

	check, err := utils.DammCheckDigit("000000")
	if err != nil {
		t.Fatalf("DammCheckDigit returned error: %v", err)
	}
	return "000000" + string(check)
}

func TestCallHandlerStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, f *callHandlerFixture, router http.Handler)
		method     string
		path       string
		token      string
		body       string
		wantStatus int
	}{
		{name: "doctor opens the call", method: http.MethodPost, path: "/api/v1/appointments/5/call", token: "doctor-token", wantStatus: http.StatusOK},
		{name: "patient opens the call with video", method: http.MethodPost, path: "/api/v1/appointments/5/call", token: "patient-token", body: `{"mediaMode":"video"}`, wantStatus: http.StatusOK},
		{name: "open without a token", method: http.MethodPost, path: "/api/v1/appointments/5/call", wantStatus: http.StatusUnauthorized},
		{name: "open an unknown appointment", method: http.MethodPost, path: "/api/v1/appointments/99/call", token: "doctor-token", wantStatus: http.StatusNotFound},
		{name: "open with a bad media mode", method: http.MethodPost, path: "/api/v1/appointments/5/call", token: "doctor-token", body: `{"mediaMode":"screen"}`, wantStatus: http.StatusBadRequest},
		{name: "open outside the call window", setup: func(t *testing.T, f *callHandlerFixture, router http.Handler) {
			f.appointments.byID.StartsAt = f.now.Add(time.Hour)
		}, method: http.MethodPost, path: "/api/v1/appointments/5/call", token: "doctor-token", wantStatus: http.StatusConflict},
		{name: "get without a room", method: http.MethodGet, path: "/api/v1/appointments/5/call", token: "patient-token", wantStatus: http.StatusNotFound},
		{name: "get an open room", setup: func(t *testing.T, f *callHandlerFixture, router http.Handler) {
			serveCall(router, http.MethodPost, "/api/v1/appointments/5/call", "doctor-token", "")
		}, method: http.MethodGet, path: "/api/v1/appointments/5/call", token: "patient-token", wantStatus: http.StatusOK},
		{name: "end without a room", method: http.MethodPost, path: "/api/v1/appointments/5/call/end", token: "patient-token", wantStatus: http.StatusNotFound},
		{name: "end an open room", setup: func(t *testing.T, f *callHandlerFixture, router http.Handler) {
			serveCall(router, http.MethodPost, "/api/v1/appointments/5/call", "doctor-token", "")
		}, method: http.MethodPost, path: "/api/v1/appointments/5/call/end", token: "patient-token", wantStatus: http.StatusOK},
		{name: "patient cannot create a guest link", method: http.MethodPost, path: "/api/v1/appointments/5/call/guest-links", token: "patient-token", wantStatus: http.StatusForbidden},
		{name: "doctor creates a guest link", method: http.MethodPost, path: "/api/v1/appointments/5/call/guest-links", token: "doctor-token", body: `{"maxUses":2}`, wantStatus: http.StatusCreated},
		{name: "guest link with too many uses", method: http.MethodPost, path: "/api/v1/appointments/5/call/guest-links", token: "doctor-token", body: `{"maxUses":9}`, wantStatus: http.StatusBadRequest},
		{name: "revoke an unknown guest link", method: http.MethodDelete, path: "/api/v1/appointments/5/call/guest-links/9", token: "doctor-token", wantStatus: http.StatusNotFound},
		{name: "join with an unknown guest link", method: http.MethodPost, path: "/api/v1/calls/guest/unknown", body: `{"displayName":"Ravi"}`, wantStatus: http.StatusNotFound},
		{name: "join with a malformed code", method: http.MethodPost, path: "/api/v1/calls/code/12345", body: `{"displayName":"Ravi"}`, wantStatus: http.StatusBadRequest},
		{name: "join without a name", method: http.MethodPost, path: "/api/v1/calls/code/1234567", body: `{}`, wantStatus: http.StatusBadRequest},
		{name: "patient cannot create a room", method: http.MethodPost, path: "/api/v1/doctors/me/call-rooms", token: "patient-token", wantStatus: http.StatusForbidden},
		{name: "doctor creates a room", method: http.MethodPost, path: "/api/v1/doctors/me/call-rooms", token: "doctor-token", body: `{"title":"Family briefing"}`, wantStatus: http.StatusCreated},
		{name: "unverified doctor cannot create a room", setup: func(t *testing.T, f *callHandlerFixture, router http.Handler) {
			f.doctors.doctor.VerificationStatus = models.VerificationStatusPending
		}, method: http.MethodPost, path: "/api/v1/doctors/me/call-rooms", token: "doctor-token", wantStatus: http.StatusForbidden},
		{name: "room with too many people", method: http.MethodPost, path: "/api/v1/doctors/me/call-rooms", token: "doctor-token", body: `{"maxParticipants":9}`, wantStatus: http.StatusBadRequest},
		{name: "doctor lists rooms", method: http.MethodGet, path: "/api/v1/doctors/me/call-rooms", token: "doctor-token", wantStatus: http.StatusOK},
		{name: "end an unknown room", method: http.MethodPost, path: "/api/v1/doctors/me/call-rooms/9/end", token: "doctor-token", wantStatus: http.StatusNotFound},
		{name: "join an unknown room as host", method: http.MethodPost, path: "/api/v1/doctors/me/call-rooms/9/join", token: "doctor-token", wantStatus: http.StatusNotFound},
		{name: "participants of an unknown room", method: http.MethodGet, path: "/api/v1/doctors/me/call-rooms/9/participants", token: "doctor-token", wantStatus: http.StatusNotFound},
		{name: "socket without a token", method: http.MethodGet, path: "/api/v1/calls/ws", wantStatus: http.StatusBadRequest},
		{name: "socket with a bad token", method: http.MethodGet, path: "/api/v1/calls/ws?token=bad", wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newCallHandlerFixture()
			router := newCallRouter(t, f)
			if tt.setup != nil {
				tt.setup(t, f, router)
			}

			response := serveCall(router, tt.method, tt.path, tt.token, tt.body)
			if response.Code != tt.wantStatus {
				t.Fatalf("%s %s status = %d, want %d; body %s", tt.method, tt.path, response.Code, tt.wantStatus, response.Body.String())
			}
		})
	}
}

func TestCallHandlerJoinCodeAttempts(t *testing.T) {
	router := newCallRouter(t, newCallHandlerFixture())
	path := "/api/v1/calls/code/" + unknownJoinCode(t)

	for attempt := 1; attempt <= constant.MaxFailedCallJoinAttempts; attempt++ {
		if response := serveCall(router, http.MethodPost, path, "", `{"displayName":"Ravi"}`); response.Code != http.StatusNotFound {
			t.Fatalf("attempt %d status = %d, want %d", attempt, response.Code, http.StatusNotFound)
		}
	}
	if response := serveCall(router, http.MethodPost, path, "", `{"displayName":"Ravi"}`); response.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt after the limit status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}
}

func TestCallHandlerCreateRoomShowsCodeOnce(t *testing.T) {
	router := newCallRouter(t, newCallHandlerFixture())

	created := serveCall(router, http.MethodPost, "/api/v1/doctors/me/call-rooms", "doctor-token", "")
	var createdBody struct {
		Data call.CallRoomResponse `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &createdBody); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if len(createdBody.Data.Code) != constant.CodeRoomCodeDigits+1 || !createdBody.Data.RequiresApproval {
		t.Fatalf("created room = %+v, want a 7-digit code and a lobby", createdBody.Data)
	}

	listed := serveCall(router, http.MethodGet, "/api/v1/doctors/me/call-rooms", "doctor-token", "")
	if strings.Contains(listed.Body.String(), createdBody.Data.Code) {
		t.Fatalf("list response %s shows the room code again", listed.Body.String())
	}
}

func openCallToken(t *testing.T, router http.Handler, token string) call.CallJoinResponse {
	t.Helper()

	response := serveCall(router, http.MethodPost, "/api/v1/appointments/5/call", token, "")
	if response.Code != http.StatusOK {
		t.Fatalf("open call status = %d; body %s", response.Code, response.Body.String())
	}
	var body struct {
		Data call.CallJoinResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode open call response: %v", err)
	}

	return body.Data
}

func dialCall(t *testing.T, server *httptest.Server, token string) *websocket.Conn {
	t.Helper()

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/calls/ws?token=" + token
	conn, response, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("dial call socket returned error: %v (status %d)", err, status)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return conn
}

func readSignal(t *testing.T, conn *websocket.Conn) call.CallSignalMessage {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var message call.CallSignalMessage
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("read signal returned error: %v", err)
	}

	return message
}

func TestCallHandlerSocket(t *testing.T) {
	router := newCallRouter(t, newCallHandlerFixture())
	server := httptest.NewServer(router)
	defer server.Close()

	doctorJoin := openCallToken(t, router, "doctor-token")
	patientJoin := openCallToken(t, router, "patient-token")

	doctor := dialCall(t, server, doctorJoin.WSToken)
	if welcome := readSignal(t, doctor); welcome.Type != "welcome" || welcome.PeerID != doctorJoin.PeerID || len(welcome.Peers) != 0 {
		t.Fatalf("doctor welcome = %+v, want welcome with nobody in", welcome)
	}

	patient := dialCall(t, server, patientJoin.WSToken)
	welcome := readSignal(t, patient)
	if welcome.Type != "welcome" || len(welcome.Peers) != 1 || welcome.Peers[0].PeerID != doctorJoin.PeerID {
		t.Fatalf("patient welcome = %+v, want the doctor listed", welcome)
	}
	if joined := readSignal(t, doctor); joined.Type != "peer-joined" || joined.Peer == nil || joined.Peer.PeerID != patientJoin.PeerID {
		t.Fatalf("doctor got %+v, want peer-joined for the patient", joined)
	}

	if err := patient.WriteJSON(call.CallSignalMessage{Type: "offer", To: doctorJoin.PeerID, SDP: "v=0"}); err != nil {
		t.Fatalf("write offer returned error: %v", err)
	}
	if offer := readSignal(t, doctor); offer.Type != "offer" || offer.From != patientJoin.PeerID || offer.SDP != "v=0" {
		t.Fatalf("doctor got %+v, want the patient's offer", offer)
	}
	if err := doctor.WriteJSON(call.CallSignalMessage{Type: "answer", To: patientJoin.PeerID, SDP: "v=0 answer"}); err != nil {
		t.Fatalf("write answer returned error: %v", err)
	}
	if answer := readSignal(t, patient); answer.Type != "answer" || answer.From != doctorJoin.PeerID {
		t.Fatalf("patient got %+v, want the doctor's answer", answer)
	}

	if err := patient.WriteMessage(websocket.TextMessage, []byte("not json")); err != nil {
		t.Fatalf("write bad message returned error: %v", err)
	}
	if rejected := readSignal(t, patient); rejected.Type != "error" {
		t.Fatalf("patient got %+v, want an error for a bad message", rejected)
	}

	if err := patient.WriteJSON(call.CallSignalMessage{Type: "leave"}); err != nil {
		t.Fatalf("write leave returned error: %v", err)
	}
	if left := readSignal(t, doctor); left.Type != "peer-left" || left.PeerID != patientJoin.PeerID {
		t.Fatalf("doctor got %+v, want peer-left for the patient", left)
	}

	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/calls/ws?token=bad"
	if _, response, err := websocket.DefaultDialer.Dial(url, nil); err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("dial with a bad token = %v, want a 401 before the upgrade", err)
	}
}
