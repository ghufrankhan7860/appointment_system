package services_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/apps/appointment/utils"
)

var inviteNow = time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)

type inviteFakes struct {
	invites  *fakePatientInviteRepo
	attempts *fakeInviteAttemptRepo
	links    *fakeUserPatientRepo
	history  *fakeUserPatientHistoryRepo
	outbox   *fakeOutboxRepo
}

func newTestInviteService(t *testing.T, fakes inviteFakes) *services.PatientInviteService {
	t.Helper()

	service, err := services.NewPatientInviteService(
		fakes.invites,
		fakes.attempts,
		fakes.links,
		fakes.history,
		&fakePatientRepo{getResult: &models.Patient{ID: 10, Name: "Asha"}},
		fakes.outbox,
		&fakeTxManager{},
		func() time.Time { return inviteNow },
		discardLogger,
	)
	if err != nil {
		t.Fatalf("NewPatientInviteService returned error: %v", err)
	}

	return service
}

func fullLink(userID uint64, relationship models.Relationship) *models.UserPatient {
	return &models.UserPatient{UserID: userID, PatientID: 10, PatientRelationship: relationship, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}
}

func TestNewPatientInviteServiceValidation(t *testing.T) {
	now := func() time.Time { return inviteNow }

	tests := []struct {
		name    string
		new     func() error
		wantErr error
	}{
		{name: "nil invite repository", wantErr: apperrors.ErrNilPatientInviteRepository, new: func() error {
			_, err := services.NewPatientInviteService(nil, &fakeInviteAttemptRepo{}, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, &fakePatientRepo{}, &fakeOutboxRepo{}, &fakeTxManager{}, now, discardLogger)
			return err
		}},
		{name: "nil attempt repository", wantErr: apperrors.ErrNilPatientInviteAttemptRepository, new: func() error {
			_, err := services.NewPatientInviteService(&fakePatientInviteRepo{}, nil, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, &fakePatientRepo{}, &fakeOutboxRepo{}, &fakeTxManager{}, now, discardLogger)
			return err
		}},
		{name: "nil history repository", wantErr: apperrors.ErrNilUserPatientHistoryRepository, new: func() error {
			_, err := services.NewPatientInviteService(&fakePatientInviteRepo{}, &fakeInviteAttemptRepo{}, &fakeUserPatientRepo{}, nil, &fakePatientRepo{}, &fakeOutboxRepo{}, &fakeTxManager{}, now, discardLogger)
			return err
		}},
		{name: "nil clock", wantErr: apperrors.ErrNilClock, new: func() error {
			_, err := services.NewPatientInviteService(&fakePatientInviteRepo{}, &fakeInviteAttemptRepo{}, &fakeUserPatientRepo{}, &fakeUserPatientHistoryRepo{}, &fakePatientRepo{}, &fakeOutboxRepo{}, &fakeTxManager{}, nil, discardLogger)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.new(); !errors.Is(err, tt.wantErr) {
				t.Fatalf("NewPatientInviteService error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestPatientInviteServiceCreate(t *testing.T) {
	readOnly := &models.UserPatient{UserID: 7, PatientID: 10, PatientRelationship: models.RelationshipMother, AccessLevel: models.AccessLevelReadOnly, Status: models.UserPatientStatusActive}

	tests := []struct {
		name         string
		links        []*models.UserPatient
		pending      int64
		relationship models.Relationship
		accessLevel  models.AccessLevel
		expiresIn    time.Duration
		wantErr      error
	}{
		{name: "full holder creates an invite", links: []*models.UserPatient{fullLink(7, models.RelationshipMother)}, relationship: models.RelationshipFather, accessLevel: models.AccessLevelAppointmentsOnly},
		{name: "not linked", links: []*models.UserPatient{fullLink(8, models.RelationshipMother)}, relationship: models.RelationshipFather, accessLevel: models.AccessLevelFull, wantErr: apperrors.ErrPatientNotFound},
		{name: "not full access", links: []*models.UserPatient{readOnly}, relationship: models.RelationshipFather, accessLevel: models.AccessLevelFull, wantErr: apperrors.ErrPatientForbidden},
		{name: "self without full access", links: []*models.UserPatient{fullLink(7, models.RelationshipMother)}, relationship: models.RelationshipSelf, accessLevel: models.AccessLevelReadOnly, wantErr: apperrors.ErrInvalidPatientInviteInput},
		{name: "self while a self link exists", links: []*models.UserPatient{fullLink(7, models.RelationshipMother), fullLink(9, models.RelationshipSelf)}, relationship: models.RelationshipSelf, accessLevel: models.AccessLevelFull, wantErr: apperrors.ErrPatientSelfLinkExists},
		{name: "too many pending invites", links: []*models.UserPatient{fullLink(7, models.RelationshipMother)}, pending: constant.MaxPendingPatientInvites, relationship: models.RelationshipFather, accessLevel: models.AccessLevelFull, wantErr: apperrors.ErrPatientInviteLimitReached},
		{name: "lifetime too long", links: []*models.UserPatient{fullLink(7, models.RelationshipMother)}, relationship: models.RelationshipFather, accessLevel: models.AccessLevelFull, expiresIn: 200 * time.Hour, wantErr: apperrors.ErrInvalidPatientInviteInput},
		{name: "unknown relationship", links: []*models.UserPatient{fullLink(7, models.RelationshipMother)}, relationship: "cousin", accessLevel: models.AccessLevelFull, wantErr: apperrors.ErrInvalidPatientInviteInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invites := &fakePatientInviteRepo{pending: tt.pending}
			service := newTestInviteService(t, inviteFakes{
				invites:  invites,
				attempts: &fakeInviteAttemptRepo{},
				links:    &fakeUserPatientRepo{patientLinks: tt.links},
				history:  &fakeUserPatientHistoryRepo{},
				outbox:   &fakeOutboxRepo{},
			})

			created, code, err := service.Create(context.Background(), 7, 10, tt.relationship, tt.accessLevel, tt.expiresIn)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Create error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if !regexp.MustCompile(`^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{8}$`).MatchString(code) {
				t.Fatalf("Create code = %q, want 8 characters from the invite alphabet", code)
			}
			if created.CodeHash != utils.HashToken(code) || created.CodeHash == code {
				t.Fatalf("Create stored %q, want only the hash of the code", created.CodeHash)
			}
			if !created.ExpiresAt.Equal(inviteNow.Add(constant.PatientInviteTTL)) || created.Status != models.PatientInviteStatusPending {
				t.Fatalf("Create invite = %+v, want pending until %v", created, inviteNow.Add(constant.PatientInviteTTL))
			}
		})
	}
}

func TestPatientInviteServiceCreateRetriesCodeClash(t *testing.T) {
	invites := &fakePatientInviteRepo{createErrs: []error{apperrors.ErrPatientInviteCodeExists}}
	service := newTestInviteService(t, inviteFakes{
		invites:  invites,
		attempts: &fakeInviteAttemptRepo{},
		links:    &fakeUserPatientRepo{patientLinks: []*models.UserPatient{fullLink(7, models.RelationshipMother)}},
		history:  &fakeUserPatientHistoryRepo{},
		outbox:   &fakeOutboxRepo{},
	})

	if _, _, err := service.Create(context.Background(), 7, 10, models.RelationshipFather, models.AccessLevelFull, 0); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if invites.createCalls != 2 {
		t.Fatalf("Create tried %d codes, want 2", invites.createCalls)
	}
}

func TestPatientInviteServiceRedeem(t *testing.T) {
	pending := &models.PatientInvite{ID: 31, PatientID: 10, PatientRelationship: models.RelationshipFather, AccessLevel: models.AccessLevelAppointmentsOnly, Status: models.PatientInviteStatusPending, ExpiresAt: inviteNow.Add(time.Hour)}
	expired := *pending
	expired.ExpiresAt = inviteNow.Add(-time.Minute)
	redeemed := *pending
	redeemed.Status = models.PatientInviteStatusRedeemed
	revokedLink := &models.UserPatient{UserID: 8, PatientID: 10, PatientRelationship: models.RelationshipFather, AccessLevel: models.AccessLevelReadOnly, Status: models.UserPatientStatusRevoked}
	activeLink := fullLink(8, models.RelationshipFather)

	tests := []struct {
		name          string
		invite        *models.PatientInvite
		inviteErr     error
		existing      *models.UserPatient
		failures      int64
		wantErr       error
		wantAttempt   bool
		wantExpiredID uint64
		wantUpdate    bool
	}{
		{name: "new link", invite: pending},
		{name: "revoked link is reactivated", invite: pending, existing: revokedLink, wantUpdate: true},
		{name: "already linked", invite: pending, existing: activeLink, wantErr: apperrors.ErrUserPatientAlreadyExists},
		{name: "unknown code", inviteErr: apperrors.ErrPatientInviteNotFound, wantErr: apperrors.ErrPatientInviteInvalid, wantAttempt: true},
		{name: "used code", invite: &redeemed, wantErr: apperrors.ErrPatientInviteInvalid, wantAttempt: true},
		{name: "expired code", invite: &expired, wantErr: apperrors.ErrPatientInviteInvalid, wantAttempt: true, wantExpiredID: 31},
		{name: "too many wrong codes", invite: pending, failures: constant.MaxFailedPatientInviteAttempts, wantErr: apperrors.ErrPatientInviteAttemptsExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := &fakeUserPatientRepo{activeErr: apperrors.ErrUserPatientNotFound, createResult: &models.UserPatient{UserID: 8, PatientID: 10, PatientRelationship: models.RelationshipFather, AccessLevel: models.AccessLevelAppointmentsOnly, Status: models.UserPatientStatusActive}}
			if tt.existing != nil {
				links.activeErr = nil
				links.activeResult = tt.existing
			}
			fakes := inviteFakes{
				invites:  &fakePatientInviteRepo{invite: tt.invite, getErr: tt.inviteErr},
				attempts: &fakeInviteAttemptRepo{count: tt.failures},
				links:    links,
				history:  &fakeUserPatientHistoryRepo{},
				outbox:   &fakeOutboxRepo{},
			}
			service := newTestInviteService(t, fakes)

			claimed, err := service.Redeem(context.Background(), 8, " k7q2-m9xd ")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Redeem error = %v, want %v", err, tt.wantErr)
			}
			if fakes.invites.gotHash != "" && fakes.invites.gotHash != utils.HashToken("K7Q2M9XD") {
				t.Fatalf("Redeem looked up %q, want the hash of the normalised code", fakes.invites.gotHash)
			}
			if (fakes.attempts.created == 1) != tt.wantAttempt {
				t.Fatalf("Redeem recorded %d attempts, want attempt %v", fakes.attempts.created, tt.wantAttempt)
			}
			if fakes.invites.expiredID != tt.wantExpiredID {
				t.Fatalf("Redeem expired invite %d, want %d", fakes.invites.expiredID, tt.wantExpiredID)
			}
			if tt.wantErr != nil {
				return
			}
			if links.updateCalled != tt.wantUpdate {
				t.Fatalf("Redeem updated an existing link = %v, want %v", links.updateCalled, tt.wantUpdate)
			}
			if claimed.Patient.ID != 10 || claimed.Link.AccessLevel != models.AccessLevelAppointmentsOnly || claimed.Link.Status != models.UserPatientStatusActive {
				t.Fatalf("Redeem = %+v, want patient 10 with active appointments_only access", claimed)
			}
			if fakes.invites.redeemedID != 31 {
				t.Fatalf("Redeem marked invite %d redeemed, want 31", fakes.invites.redeemedID)
			}
			if len(fakes.history.entries) != 1 || fakes.history.entries[0].Reason != models.UserPatientChangeInviteRedeemed {
				t.Fatalf("Redeem history = %+v, want one invite_redeemed entry", fakes.history.entries)
			}
			if tt.wantUpdate && fakes.history.entries[0].FromStatus != models.UserPatientStatusRevoked {
				t.Fatalf("Redeem history from status = %q, want revoked", fakes.history.entries[0].FromStatus)
			}
			if len(fakes.outbox.events) != 1 || fakes.outbox.events[0].EventType != models.OutboxEventPatientAccessGranted {
				t.Fatalf("Redeem outbox = %+v, want one %s event", fakes.outbox.events, models.OutboxEventPatientAccessGranted)
			}
		})
	}
}
