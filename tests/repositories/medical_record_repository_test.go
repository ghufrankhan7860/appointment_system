package repositories_test

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestMedicalRecordRepository(t *testing.T) {
	db := openTestDB(t)
	fixture := newAppointmentFixture(t, db)
	ctx := context.Background()
	records, err := repositories.NewMedicalRecordRepository(db)
	if err != nil {
		t.Fatalf("NewMedicalRecordRepository returned error: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM medical_record_access_log WHERE patient_id = ?", fixture.patient.ID)
		db.Exec("DELETE FROM medical_attachments WHERE patient_id = ?", fixture.patient.ID)
		db.Exec("DELETE FROM medical_records WHERE patient_id = ?", fixture.patient.ID)
	})

	appointment, err := fixture.book(time.Now().Add(180*time.Hour).Truncate(time.Hour), 30)
	if err != nil {
		t.Fatalf("Create appointment returned error: %v", err)
	}
	draft := &models.MedicalRecord{AppointmentID: appointment.ID, PatientID: fixture.patient.ID, DoctorID: fixture.doctor.ID, CreatedByUserID: fixture.doctor.UserID}
	created, err := records.Create(ctx, draft)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := records.Create(ctx, draft); !errors.Is(err, apperrors.ErrMedicalRecordExists) {
		t.Fatalf("second Create error = %v, want %v", err, apperrors.ErrMedicalRecordExists)
	}

	pulse, temperature := 88, 101.1
	followUp := time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC)
	if err := records.UpdateDraft(ctx, &models.MedicalRecord{
		ID:                created.ID,
		Diagnosis:         "Viral fever",
		PrescriptionNotes: "Plenty of fluids",
		FollowUpOn:        &followUp,
		Vitals:            models.Vitals{BloodPressure: "118/76", PulsePerMinute: &pulse, TemperatureFahrenheit: &temperature},
		Medicines: []models.PrescriptionItem{
			{MedicineName: "Paracetamol", Strength: "500 mg", Dosage: "1-0-1", DurationDays: 3, Timing: models.MedicineTimingAfterFood},
			{MedicineName: "ORS", Dosage: "as needed", Timing: models.MedicineTimingAny},
		},
	}); err != nil {
		t.Fatalf("UpdateDraft returned error: %v", err)
	}
	if err := records.UpdateDraft(ctx, &models.MedicalRecord{ID: created.ID, Diagnosis: "Viral fever", Medicines: []models.PrescriptionItem{{MedicineName: "Paracetamol", Dosage: "1-0-1", Timing: models.MedicineTimingAny}}}); err != nil {
		t.Fatalf("second UpdateDraft returned error: %v", err)
	}

	saved, err := records.GetByAppointment(ctx, appointment.ID)
	if err != nil {
		t.Fatalf("GetByAppointment returned error: %v", err)
	}
	if len(saved.Medicines) != 1 || saved.Medicines[0].MedicineName != "Paracetamol" {
		t.Fatalf("GetByAppointment medicines = %+v, want the second edit to replace the first", saved.Medicines)
	}
	if saved.Vitals.PulsePerMinute != nil || saved.FollowUpOn != nil {
		t.Fatalf("GetByAppointment vitals %+v follow-up %v, want the second edit to clear them", saved.Vitals, saved.FollowUpOn)
	}

	if err := records.UpdateDraft(ctx, &models.MedicalRecord{
		ID: created.ID, Diagnosis: "Viral fever", FollowUpOn: &followUp,
		Vitals: models.Vitals{PulsePerMinute: &pulse, TemperatureFahrenheit: &temperature},
	}); err != nil {
		t.Fatalf("third UpdateDraft returned error: %v", err)
	}
	saved, err = records.GetByID(ctx, created.ID)
	if err != nil || saved.Vitals.PulsePerMinute == nil || *saved.Vitals.PulsePerMinute != pulse || *saved.Vitals.TemperatureFahrenheit != temperature {
		t.Fatalf("GetByID vitals = %+v, %v; want the pulse and temperature back", saved.Vitals, err)
	}
	if saved.FollowUpOn == nil || saved.FollowUpOn.Format(time.DateOnly) != "2026-11-02" {
		t.Fatalf("GetByID follow-up = %v, want 2026-11-02", saved.FollowUpOn)
	}

	if listed, err := records.ListFinalizedByPatient(ctx, fixture.patient.ID, 10, 0); err != nil || len(listed) != 0 {
		t.Fatalf("ListFinalizedByPatient before finalizing = %d, %v; want none", len(listed), err)
	}
	if err := records.Finalize(ctx, created.ID); err != nil {
		t.Fatalf("Finalize returned error: %v", err)
	}
	if err := records.Finalize(ctx, created.ID); !errors.Is(err, apperrors.ErrMedicalRecordFinalized) {
		t.Fatalf("Finalize twice error = %v, want %v", err, apperrors.ErrMedicalRecordFinalized)
	}
	if err := records.UpdateDraft(ctx, &models.MedicalRecord{ID: created.ID, Diagnosis: "changed"}); !errors.Is(err, apperrors.ErrMedicalRecordFinalized) {
		t.Fatalf("UpdateDraft after finalizing error = %v, want %v", err, apperrors.ErrMedicalRecordFinalized)
	}
	listed, err := records.ListFinalizedByPatient(ctx, fixture.patient.ID, 10, 0)
	if err != nil || len(listed) != 1 || listed[0].Diagnosis != "Viral fever" || listed[0].FinalizedAt == nil {
		t.Fatalf("ListFinalizedByPatient = %+v, %v; want the finalized record", listed, err)
	}

	takenOn := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	attachment, err := records.CreateAttachment(ctx, &models.MedicalAttachment{
		PatientID:        fixture.patient.ID,
		MedicalRecordID:  &created.ID,
		UploadedByUserID: fixture.doctor.UserID,
		Kind:             models.AttachmentKindLabReport,
		Title:            "CBC",
		StorageKey:       fmt.Sprintf("patients/%d/attachments/%d", fixture.patient.ID, time.Now().UnixNano()),
		ContentType:      "application/pdf",
		SizeBytes:        1024,
		TakenOn:          &takenOn,
	})
	if err != nil {
		t.Fatalf("CreateAttachment returned error: %v", err)
	}
	if found, err := records.GetAttachment(ctx, attachment.ID); err != nil || found.Title != "CBC" || found.TakenOn == nil {
		t.Fatalf("GetAttachment = %+v, %v; want the CBC report", found, err)
	}
	if byRecord, err := records.ListAttachmentsByRecord(ctx, created.ID); err != nil || len(byRecord) != 1 {
		t.Fatalf("ListAttachmentsByRecord = %d, %v; want 1", len(byRecord), err)
	}

	if err := records.LogAccess(ctx, &models.RecordAccess{PatientID: fixture.patient.ID, MedicalRecordID: &created.ID, AccessedByUserID: fixture.booker.ID, Action: models.RecordAccessView, CorrelationID: "corr-1"}); err != nil {
		t.Fatalf("LogAccess returned error: %v", err)
	}
	if err := records.LogAccess(ctx, &models.RecordAccess{PatientID: fixture.patient.ID, AttachmentID: &attachment.ID, AccessedByUserID: fixture.booker.ID, Action: models.RecordAccessDownload}); err != nil {
		t.Fatalf("LogAccess for an attachment returned error: %v", err)
	}
	entries, err := records.ListAccessLog(ctx, created.ID, 10, 0)
	if err != nil || len(entries) != 2 {
		t.Fatalf("ListAccessLog = %d entries, %v; want the view and the attachment download", len(entries), err)
	}
}
