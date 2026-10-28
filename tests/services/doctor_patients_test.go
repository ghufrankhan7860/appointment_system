package services_test

import (
	"errors"
	"io"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

func TestAppointmentServiceListPatients(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, clinicLocation(t))
	rows := []*models.DoctorPatient{{PatientID: 3, Name: "Ravi Kumar", Visits: 2}}

	tests := []struct {
		name    string
		role    models.Role
		doctor  *models.Doctor
		wantErr error
	}{
		{name: "the doctor lists their patients", role: models.RoleDoctor, doctor: clinicDoctor(models.VerificationStatusVerified)},
		{name: "a doctor account without a profile", role: models.RoleDoctor, wantErr: apperrors.ErrDoctorNotFound},
		{name: "a patient account", role: models.RolePatient, wantErr: apperrors.ErrDoctorRoleRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appointments := &fakeAppointmentRepo{patients: rows}
			service := newTestAppointmentService(t, &fakeDoctorRepo{byUserID: tt.doctor}, appointments, &fakeOutboxRepo{}, &fakeUserPatientRepo{}, now)

			listed, err := service.ListPatients(requestContext(), 11, tt.role, "  ravi ", 0, 0)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ListPatients error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListPatients returned error: %v", err)
			}
			if len(listed) != 1 || appointments.patientsQuery != "ravi" || appointments.patientsLimit != constant.DefaultListLimit {
				t.Fatalf("ListPatients = %d rows for %q with limit %d, want 1 for ravi with the default limit", len(listed), appointments.patientsQuery, appointments.patientsLimit)
			}
		})
	}
}

func TestMedicalRecordServiceTreatingDoctor(t *testing.T) {
	treatingDoctor := clinicDoctor(models.VerificationStatusVerified)
	otherDoctorsRecord := func(status models.MedicalRecordStatus) *models.MedicalRecord {
		record := draftRecord(status)
		record.DoctorID = 9
		return record
	}
	standalone := &models.MedicalAttachment{ID: 95, PatientID: 3, StorageKey: "patients/3/attachments/abc", ContentType: "application/pdf"}

	tests := []struct {
		name    string
		role    models.Role
		treated bool
		wantErr error
	}{
		{name: "a doctor treating the patient", role: models.RoleDoctor, treated: true},
		{name: "a doctor without a counted booking", role: models.RoleDoctor, wantErr: apperrors.ErrPatientNotFound},
		{name: "a patient account", role: models.RolePatient, wantErr: apperrors.ErrPatientNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name+" lists records and files", func(t *testing.T) {
			records := &fakeMedicalRecordRepo{listed: []*models.MedicalRecord{otherDoctorsRecord(models.MedicalRecordStatusFinalized)}, attachments: []*models.MedicalAttachment{standalone}}
			appointments := &fakeAppointmentRepo{treated: map[uint64]bool{3: tt.treated}}
			service := newTestMedicalRecordService(t, records, appointments, noLink(), treatingDoctor, &fakeFileStorage{}, &fakeOutboxRepo{}, time.Now())

			listed, err := service.ListForDoctor(requestContext(), 11, tt.role, 3, 0, 0)
			_, attachmentsErr := service.ListAttachmentsForDoctor(requestContext(), 11, tt.role, 3, 0, 0)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || !errors.Is(attachmentsErr, tt.wantErr) {
					t.Fatalf("ListForDoctor error = %v, ListAttachmentsForDoctor error = %v, want %v", err, attachmentsErr, tt.wantErr)
				}
				if len(records.accesses) != 0 {
					t.Fatalf("refused lists wrote %d access entries", len(records.accesses))
				}
				return
			}
			if err != nil || attachmentsErr != nil {
				t.Fatalf("ListForDoctor error = %v, ListAttachmentsForDoctor error = %v", err, attachmentsErr)
			}
			if len(listed) != 1 || records.viewDoctorID != treatingDoctor.ID {
				t.Fatalf("ListForDoctor = %d records for doctor %d, want 1 for doctor %d", len(listed), records.viewDoctorID, treatingDoctor.ID)
			}
			if len(records.accesses) != 2 || records.accesses[0].Action != models.RecordAccessList || records.accesses[1].Action != models.RecordAccessList {
				t.Fatalf("access log = %+v, want two list entries", records.accesses)
			}
		})
	}

	t.Run("a treating doctor opens another doctor's finalized record but not a draft", func(t *testing.T) {
		appointments := &fakeAppointmentRepo{treated: map[uint64]bool{3: true}}
		finalized := &fakeMedicalRecordRepo{record: otherDoctorsRecord(models.MedicalRecordStatusFinalized)}
		service := newTestMedicalRecordService(t, finalized, appointments, noLink(), treatingDoctor, &fakeFileStorage{}, &fakeOutboxRepo{}, time.Now())
		if _, _, err := service.GetByID(requestContext(), 11, models.RoleDoctor, 90); err != nil {
			t.Fatalf("GetByID of a finalized record returned error: %v", err)
		}

		draft := &fakeMedicalRecordRepo{record: otherDoctorsRecord(models.MedicalRecordStatusDraft)}
		service = newTestMedicalRecordService(t, draft, appointments, noLink(), treatingDoctor, &fakeFileStorage{}, &fakeOutboxRepo{}, time.Now())
		if _, _, err := service.GetByID(requestContext(), 11, models.RoleDoctor, 90); !errors.Is(err, apperrors.ErrMedicalRecordNotFound) {
			t.Fatalf("GetByID of another doctor's draft error = %v, want %v", err, apperrors.ErrMedicalRecordNotFound)
		}

		notTreating := &fakeAppointmentRepo{}
		service = newTestMedicalRecordService(t, finalized, notTreating, noLink(), treatingDoctor, &fakeFileStorage{}, &fakeOutboxRepo{}, time.Now())
		if _, _, err := service.GetByID(requestContext(), 11, models.RoleDoctor, 90); !errors.Is(err, apperrors.ErrMedicalRecordNotFound) {
			t.Fatalf("GetByID by a doctor not treating the patient error = %v, want %v", err, apperrors.ErrMedicalRecordNotFound)
		}
	})

	t.Run("a treating doctor downloads the patient's own upload", func(t *testing.T) {
		storage := &fakeFileStorage{files: map[string][]byte{standalone.StorageKey: pdfContent}}
		records := &fakeMedicalRecordRepo{attachments: []*models.MedicalAttachment{standalone}}
		service := newTestMedicalRecordService(t, records, &fakeAppointmentRepo{treated: map[uint64]bool{3: true}}, noLink(), treatingDoctor, storage, &fakeOutboxRepo{}, time.Now())

		_, file, err := service.OpenAttachment(requestContext(), 11, models.RoleDoctor, 95)
		if err != nil {
			t.Fatalf("OpenAttachment returned error: %v", err)
		}
		content, _ := io.ReadAll(file)
		file.Close()
		if string(content) != string(pdfContent) || len(records.accesses) != 1 || records.accesses[0].Action != models.RecordAccessDownload {
			t.Fatalf("OpenAttachment read %q with log %+v, want the file and one download entry", content, records.accesses)
		}

		service = newTestMedicalRecordService(t, records, &fakeAppointmentRepo{}, noLink(), treatingDoctor, storage, &fakeOutboxRepo{}, time.Now())
		if _, _, err := service.OpenAttachment(requestContext(), 11, models.RoleDoctor, 95); !errors.Is(err, apperrors.ErrAttachmentNotFound) {
			t.Fatalf("OpenAttachment by a doctor not treating the patient error = %v, want %v", err, apperrors.ErrAttachmentNotFound)
		}
	})
}
