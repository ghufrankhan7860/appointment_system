package services_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"appointment_system/internal/appcontext"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

var pdfContent = []byte("%PDF-1.4\n1 0 obj << /Type /Catalog >> endobj\n")

func newTestMedicalRecordService(
	t *testing.T,
	records *fakeMedicalRecordRepo,
	appointments *fakeAppointmentRepo,
	userPatients *fakeUserPatientRepo,
	doctor *models.Doctor,
	storage *fakeFileStorage,
	outbox *fakeOutboxRepo,
	now time.Time,
) *services.MedicalRecordService {
	t.Helper()

	service, err := services.NewMedicalRecordService(records, appointments, userPatients, &fakeDoctorRepo{byUserID: doctor}, storage, outbox, &fakeTxManager{}, func() time.Time { return now }, discardLogger)
	if err != nil {
		t.Fatalf("NewMedicalRecordService returned error: %v", err)
	}

	return service
}

func requestContext() context.Context {
	return appcontext.With(context.Background(), appcontext.New(context.Background(), "corr-123"))
}

func draftRecord(status models.MedicalRecordStatus) *models.MedicalRecord {
	return &models.MedicalRecord{ID: 90, AppointmentID: 50, PatientID: 3, DoctorID: 5, Status: status, Diagnosis: "Viral fever"}
}

func noLink() *fakeUserPatientRepo {
	return &fakeUserPatientRepo{activeErr: apperrors.ErrUserPatientNotFound}
}

func TestMedicalRecordServiceCreateDraft(t *testing.T) {
	location := clinicLocation(t)
	nineAM := time.Date(2026, 10, 26, 9, 0, 0, 0, location)
	ownDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor.ID = 6

	tests := []struct {
		name     string
		role     models.Role
		doctor   *models.Doctor
		status   models.AppointmentStatus
		now      time.Time
		existing *models.MedicalRecord
		wantErr  error
	}{
		{name: "the doctor opens a record once the visit started", role: models.RoleDoctor, doctor: ownDoctor},
		{name: "a completed visit can get a record", role: models.RoleDoctor, doctor: ownDoctor, status: models.AppointmentStatusCompleted},
		{name: "asking again returns the existing record", role: models.RoleDoctor, doctor: ownDoctor, existing: draftRecord(models.MedicalRecordStatusFinalized)},
		{name: "before the visit starts", role: models.RoleDoctor, doctor: ownDoctor, now: nineAM.Add(-time.Minute), wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "a cancelled visit", role: models.RoleDoctor, doctor: ownDoctor, status: models.AppointmentStatusCancelled, wantErr: apperrors.ErrAppointmentNotModifiable},
		{name: "another doctor's appointment", role: models.RoleDoctor, doctor: otherDoctor, wantErr: apperrors.ErrAppointmentNotFound},
		{name: "a patient account", role: models.RolePatient, wantErr: apperrors.ErrDoctorRoleRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status == "" {
				tt.status = models.AppointmentStatusScheduled
			}
			if tt.now.IsZero() {
				tt.now = nineAM.Add(10 * time.Minute)
			}
			records := &fakeMedicalRecordRepo{record: tt.existing}
			appointments := &fakeAppointmentRepo{byID: scheduledAt(nineAM, tt.status)}
			service := newTestMedicalRecordService(t, records, appointments, noLink(), tt.doctor, &fakeFileStorage{}, &fakeOutboxRepo{}, tt.now)

			record, err := service.CreateDraft(requestContext(), 11, tt.role, 50)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("CreateDraft error = %v, want %v", err, tt.wantErr)
				}
				if records.created || len(records.accesses) != 0 {
					t.Fatalf("CreateDraft wrote a record or access log after an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateDraft returned error: %v", err)
			}
			if tt.existing != nil {
				if records.created || record.ID != 90 {
					t.Fatalf("CreateDraft created a second record, want the existing one returned")
				}
				return
			}
			if record.Status != models.MedicalRecordStatusDraft || record.PatientID != 3 || record.DoctorID != 5 || record.CreatedByUserID != 11 {
				t.Fatalf("CreateDraft record = %+v, want a draft for patient 3 by doctor 5", record)
			}
			access := records.accesses[0]
			if access.Action != models.RecordAccessCreate || access.CorrelationID != "corr-123" || access.AccessedByUserID != 11 || *access.MedicalRecordID != 90 {
				t.Fatalf("CreateDraft access log = %+v, want a create entry with the request's correlation id", access)
			}
		})
	}
}

func TestMedicalRecordServiceUpdateDraft(t *testing.T) {
	ownDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor.ID = 6
	pulse := func(value int) *int { return &value }

	tests := []struct {
		name    string
		doctor  *models.Doctor
		status  models.MedicalRecordStatus
		edit    func(*models.MedicalRecord)
		wantErr error
		reason  string
	}{
		{name: "a draft takes notes, vitals and medicines", doctor: ownDoctor},
		{name: "a finalized record never changes", doctor: ownDoctor, status: models.MedicalRecordStatusFinalized, wantErr: apperrors.ErrMedicalRecordFinalized},
		{name: "another doctor's record", doctor: otherDoctor, wantErr: apperrors.ErrMedicalRecordNotFound},
		{name: "an impossible pulse", doctor: ownDoctor, edit: func(record *models.MedicalRecord) { record.Vitals.PulsePerMinute = pulse(500) }, wantErr: apperrors.ErrInvalidMedicalRecordInput, reason: "vitals.pulsePerMinute must be between 20 and 300"},
		{name: "a medicine without a dosage", doctor: ownDoctor, edit: func(record *models.MedicalRecord) { record.Medicines[0].Dosage = " " }, wantErr: apperrors.ErrInvalidMedicalRecordInput, reason: "medicines.dosage is required"},
		{name: "an unknown timing", doctor: ownDoctor, edit: func(record *models.MedicalRecord) { record.Medicines[0].Timing = "at_night" }, wantErr: apperrors.ErrInvalidMedicalRecordInput},
		{name: "too many medicines", doctor: ownDoctor, edit: func(record *models.MedicalRecord) {
			record.Medicines = make([]models.PrescriptionItem, constant.MaxPrescriptionItems+1)
		}, wantErr: apperrors.ErrInvalidMedicalRecordInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status == "" {
				tt.status = models.MedicalRecordStatusDraft
			}
			records := &fakeMedicalRecordRepo{record: draftRecord(tt.status)}
			service := newTestMedicalRecordService(t, records, &fakeAppointmentRepo{}, noLink(), tt.doctor, &fakeFileStorage{}, &fakeOutboxRepo{}, time.Now())

			edit := &models.MedicalRecord{
				ID:        90,
				Diagnosis: "  Viral fever  ",
				Vitals:    models.Vitals{BloodPressure: "120/80", PulsePerMinute: pulse(82)},
				Medicines: []models.PrescriptionItem{{MedicineName: " Paracetamol ", Strength: "500 mg", Dosage: "1-0-1", DurationDays: 3}},
			}
			if tt.edit != nil {
				tt.edit(edit)
			}

			updated, err := service.UpdateDraft(requestContext(), 11, models.RoleDoctor, edit)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("UpdateDraft error = %v, want %v", err, tt.wantErr)
				}
				if tt.reason != "" && apperrors.Reason(err) != tt.reason {
					t.Fatalf("UpdateDraft reason = %q, want %q", apperrors.Reason(err), tt.reason)
				}
				if records.updated != nil {
					t.Fatalf("UpdateDraft changed the record after an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateDraft returned error: %v", err)
			}
			medicine := updated.Medicines[0]
			if updated.Diagnosis != "Viral fever" || medicine.MedicineName != "Paracetamol" || medicine.Timing != models.MedicineTimingAny {
				t.Fatalf("UpdateDraft saved %+v, want trimmed text and timing defaulted to any", updated)
			}
			if len(records.accesses) != 1 || records.accesses[0].Action != models.RecordAccessUpdate {
				t.Fatalf("UpdateDraft access log = %+v, want one update entry", records.accesses)
			}
		})
	}
}

func TestMedicalRecordServiceFinalize(t *testing.T) {
	ownDoctor := clinicDoctor(models.VerificationStatusVerified)

	t.Run("a draft with a diagnosis is locked and announced", func(t *testing.T) {
		records := &fakeMedicalRecordRepo{record: draftRecord(models.MedicalRecordStatusDraft)}
		outbox := &fakeOutboxRepo{}
		service := newTestMedicalRecordService(t, records, &fakeAppointmentRepo{}, noLink(), ownDoctor, &fakeFileStorage{}, outbox, time.Now())

		finalized, err := service.Finalize(requestContext(), 11, models.RoleDoctor, 90)
		if err != nil {
			t.Fatalf("Finalize returned error: %v", err)
		}
		if finalized.Status != models.MedicalRecordStatusFinalized {
			t.Fatalf("Finalize status = %q, want finalized", finalized.Status)
		}
		if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventMedicalRecordFinalized {
			t.Fatalf("Finalize outbox = %+v, want one finalized event", outbox.events)
		}
		if _, ok := outbox.events[0].Payload["diagnosis"]; ok {
			t.Fatalf("Finalize put clinical content in the outbox: %+v", outbox.events[0].Payload)
		}
		if records.accesses[0].Action != models.RecordAccessFinalize {
			t.Fatalf("Finalize access log = %+v, want a finalize entry", records.accesses)
		}
	})

	t.Run("a draft without a diagnosis", func(t *testing.T) {
		record := draftRecord(models.MedicalRecordStatusDraft)
		record.Diagnosis = ""
		service := newTestMedicalRecordService(t, &fakeMedicalRecordRepo{record: record}, &fakeAppointmentRepo{}, noLink(), ownDoctor, &fakeFileStorage{}, &fakeOutboxRepo{}, time.Now())

		if _, err := service.Finalize(requestContext(), 11, models.RoleDoctor, 90); !errors.Is(err, apperrors.ErrInvalidMedicalRecordInput) || apperrors.Reason(err) != "diagnosis is required to finalize" {
			t.Fatalf("Finalize error = %v, want the diagnosis reason", err)
		}
	})

	t.Run("a record finalized twice", func(t *testing.T) {
		service := newTestMedicalRecordService(t, &fakeMedicalRecordRepo{record: draftRecord(models.MedicalRecordStatusFinalized)}, &fakeAppointmentRepo{}, noLink(), ownDoctor, &fakeFileStorage{}, &fakeOutboxRepo{}, time.Now())

		if _, err := service.Finalize(requestContext(), 11, models.RoleDoctor, 90); !errors.Is(err, apperrors.ErrMedicalRecordFinalized) {
			t.Fatalf("Finalize error = %v, want %v", err, apperrors.ErrMedicalRecordFinalized)
		}
	})
}

func TestMedicalRecordServiceReadAccess(t *testing.T) {
	ownDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctor.ID = 6

	tests := []struct {
		name    string
		role    models.Role
		doctor  *models.Doctor
		link    *models.UserPatient
		status  models.MedicalRecordStatus
		wantErr error
	}{
		{name: "the record's doctor reads a draft", role: models.RoleDoctor, doctor: ownDoctor, status: models.MedicalRecordStatusDraft},
		{name: "full access reads a finalized record", role: models.RolePatient, link: patientLink(models.AccessLevelFull)},
		{name: "full access cannot see a draft", role: models.RolePatient, link: patientLink(models.AccessLevelFull), status: models.MedicalRecordStatusDraft, wantErr: apperrors.ErrMedicalRecordNotFound},
		{name: "appointments_only access cannot read records", role: models.RolePatient, link: patientLink(models.AccessLevelAppointmentsOnly), wantErr: apperrors.ErrMedicalRecordNotFound},
		{name: "read_only access cannot read records", role: models.RolePatient, link: patientLink(models.AccessLevelReadOnly), wantErr: apperrors.ErrMedicalRecordNotFound},
		{name: "no link", role: models.RolePatient, wantErr: apperrors.ErrMedicalRecordNotFound},
		{name: "another doctor", role: models.RoleDoctor, doctor: otherDoctor, wantErr: apperrors.ErrMedicalRecordNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status == "" {
				tt.status = models.MedicalRecordStatusFinalized
			}
			userPatients := &fakeUserPatientRepo{activeResult: tt.link}
			if tt.link == nil {
				userPatients.activeErr = apperrors.ErrUserPatientNotFound
			}
			records := &fakeMedicalRecordRepo{record: draftRecord(tt.status)}
			service := newTestMedicalRecordService(t, records, &fakeAppointmentRepo{}, userPatients, tt.doctor, &fakeFileStorage{}, &fakeOutboxRepo{}, time.Now())

			_, _, err := service.GetByID(requestContext(), 7, tt.role, 90)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("GetByID error = %v, want %v", err, tt.wantErr)
				}
				if len(records.accesses) != 0 {
					t.Fatalf("GetByID logged a view after refusing it")
				}
				return
			}
			if err != nil {
				t.Fatalf("GetByID returned error: %v", err)
			}
			if len(records.accesses) != 1 || records.accesses[0].Action != models.RecordAccessView || records.accesses[0].AccessedByUserID != 7 {
				t.Fatalf("GetByID access log = %+v, want one view entry by 7", records.accesses)
			}
		})
	}

	t.Run("listing needs full access", func(t *testing.T) {
		for _, link := range []*models.UserPatient{patientLink(models.AccessLevelAppointmentsOnly), patientLink(models.AccessLevelReadOnly)} {
			service := newTestMedicalRecordService(t, &fakeMedicalRecordRepo{}, &fakeAppointmentRepo{}, &fakeUserPatientRepo{activeResult: link}, nil, &fakeFileStorage{}, &fakeOutboxRepo{}, time.Now())
			if _, err := service.ListForPatient(requestContext(), 7, 3, 10, 0); !errors.Is(err, apperrors.ErrPatientNotFound) {
				t.Fatalf("ListForPatient with %s error = %v, want %v", link.AccessLevel, err, apperrors.ErrPatientNotFound)
			}
		}

		records := &fakeMedicalRecordRepo{listed: []*models.MedicalRecord{draftRecord(models.MedicalRecordStatusFinalized)}}
		service := newTestMedicalRecordService(t, records, &fakeAppointmentRepo{}, &fakeUserPatientRepo{activeResult: patientLink(models.AccessLevelFull)}, nil, &fakeFileStorage{}, &fakeOutboxRepo{}, time.Now())
		listed, err := service.ListForPatient(requestContext(), 7, 3, 10, 0)
		if err != nil || len(listed) != 1 || records.accesses[0].Action != models.RecordAccessList {
			t.Fatalf("ListForPatient = %d records, %v, log %+v; want 1 with a list entry", len(listed), err, records.accesses)
		}
	})
}

func TestMedicalRecordServiceAttachments(t *testing.T) {
	ownDoctor := clinicDoctor(models.VerificationStatusVerified)

	tests := []struct {
		name       string
		role       models.Role
		doctor     *models.Doctor
		link       *models.UserPatient
		recordID   *uint64
		record     *models.MedicalRecord
		content    []byte
		title      string
		storeErr   error
		saveErr    error
		wantErr    error
		wantStored bool
	}{
		{name: "full access uploads a PDF", role: models.RolePatient, link: patientLink(models.AccessLevelFull), content: pdfContent, wantStored: true},
		{name: "the doctor adds a scan to their draft", role: models.RoleDoctor, doctor: ownDoctor, recordID: ptr(uint64(90)), record: draftRecord(models.MedicalRecordStatusDraft), content: []byte("\x89PNG\r\n\x1a\n0000"), wantStored: true},
		{name: "the doctor cannot add to a finalized record", role: models.RoleDoctor, doctor: ownDoctor, recordID: ptr(uint64(90)), record: draftRecord(models.MedicalRecordStatusFinalized), content: pdfContent, wantErr: apperrors.ErrMedicalRecordFinalized},
		{name: "appointments_only access cannot upload", role: models.RolePatient, link: patientLink(models.AccessLevelAppointmentsOnly), content: pdfContent, wantErr: apperrors.ErrPatientNotFound},
		{name: "a text file is refused by its content", role: models.RolePatient, link: patientLink(models.AccessLevelFull), content: []byte("just some notes"), wantErr: apperrors.ErrAttachmentType},
		{name: "an empty file", role: models.RolePatient, link: patientLink(models.AccessLevelFull), content: []byte{}, wantErr: apperrors.ErrInvalidMedicalRecordInput},
		{name: "a file over the limit is removed again", role: models.RolePatient, link: patientLink(models.AccessLevelFull), content: append(append([]byte{}, pdfContent...), make([]byte, constant.MaxAttachmentBytes)...), wantErr: apperrors.ErrAttachmentTooLarge},
		{name: "a missing title", role: models.RolePatient, link: patientLink(models.AccessLevelFull), content: pdfContent, title: " ", wantErr: apperrors.ErrInvalidMedicalRecordInput},
		{name: "a failed save removes the stored file", role: models.RolePatient, link: patientLink(models.AccessLevelFull), content: pdfContent, saveErr: errors.New("database down")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.title == "" {
				tt.title = "Blood report"
			}
			userPatients := &fakeUserPatientRepo{activeResult: tt.link}
			if tt.link == nil {
				userPatients.activeErr = apperrors.ErrUserPatientNotFound
			}
			records := &fakeMedicalRecordRepo{record: tt.record, createAttachmentErr: tt.saveErr}
			storage := &fakeFileStorage{}
			service := newTestMedicalRecordService(t, records, &fakeAppointmentRepo{}, userPatients, tt.doctor, storage, &fakeOutboxRepo{}, time.Now())

			uploaded, err := service.UploadAttachment(requestContext(), 7, tt.role, &models.MedicalAttachment{
				PatientID:       3,
				MedicalRecordID: tt.recordID,
				Kind:            models.AttachmentKindLabReport,
				Title:           tt.title,
			}, bytes.NewReader(tt.content))
			if !tt.wantStored {
				if err == nil || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
					t.Fatalf("UploadAttachment error = %v, want %v", err, tt.wantErr)
				}
				if len(storage.files) != 0 {
					t.Fatalf("UploadAttachment left %d stored files after an error", len(storage.files))
				}
				return
			}
			if err != nil {
				t.Fatalf("UploadAttachment returned error: %v", err)
			}
			if !strings.HasPrefix(uploaded.StorageKey, "patients/3/attachments/") || uploaded.SizeBytes != int64(len(tt.content)) || uploaded.UploadedByUserID != 7 {
				t.Fatalf("UploadAttachment saved %+v, want a key under patients/3 with the real size", uploaded)
			}
			if !bytes.Equal(storage.files[uploaded.StorageKey], tt.content) {
				t.Fatalf("UploadAttachment stored different bytes")
			}
			if records.accesses[0].Action != models.RecordAccessUpload || *records.accesses[0].AttachmentID != uploaded.ID {
				t.Fatalf("UploadAttachment access log = %+v, want an upload entry", records.accesses)
			}
		})
	}

	t.Run("opening needs full access or the record's doctor", func(t *testing.T) {
		recordID := uint64(90)
		storage := &fakeFileStorage{files: map[string][]byte{"patients/3/attachments/abc": pdfContent}}
		attachment := &models.MedicalAttachment{ID: 95, PatientID: 3, MedicalRecordID: &recordID, StorageKey: "patients/3/attachments/abc", ContentType: "application/pdf"}

		for _, tc := range []struct {
			role    models.Role
			doctor  *models.Doctor
			link    *models.UserPatient
			wantErr error
		}{
			{role: models.RolePatient, link: patientLink(models.AccessLevelFull)},
			{role: models.RoleDoctor, doctor: ownDoctor},
			{role: models.RolePatient, link: patientLink(models.AccessLevelReadOnly), wantErr: apperrors.ErrAttachmentNotFound},
		} {
			userPatients := &fakeUserPatientRepo{activeResult: tc.link}
			if tc.link == nil {
				userPatients.activeErr = apperrors.ErrUserPatientNotFound
			}
			records := &fakeMedicalRecordRepo{record: draftRecord(models.MedicalRecordStatusFinalized), attachments: []*models.MedicalAttachment{attachment}}
			service := newTestMedicalRecordService(t, records, &fakeAppointmentRepo{}, userPatients, tc.doctor, storage, &fakeOutboxRepo{}, time.Now())

			_, file, err := service.OpenAttachment(requestContext(), 7, tc.role, 95)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) || len(records.accesses) != 0 {
					t.Fatalf("OpenAttachment as %s error = %v with %d log rows, want %v and none", tc.role, err, len(records.accesses), tc.wantErr)
				}
				continue
			}
			if err != nil {
				t.Fatalf("OpenAttachment as %s returned error: %v", tc.role, err)
			}
			content, _ := io.ReadAll(file)
			file.Close()
			if !bytes.Equal(content, pdfContent) || records.accesses[0].Action != models.RecordAccessDownload {
				t.Fatalf("OpenAttachment as %s returned %d bytes with log %+v, want the file and a download entry", tc.role, len(content), records.accesses)
			}
		}
	})
}

func ptr[T any](value T) *T {
	return &value
}
