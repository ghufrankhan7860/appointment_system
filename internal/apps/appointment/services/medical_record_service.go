package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"appointment_system/internal/appcontext"
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// MedicalRecordService keeps the clinical side of a visit: the doctor's notes,
// the prescription and uploaded reports. Only the appointment's doctor writes
// a record; the patient side needs full access to read finalized ones, and a
// doctor treating the patient reads them too. Every
// read and write is logged with the request's correlation ID in the same
// transaction, so access to clinical data is always traceable.
type MedicalRecordService struct {
	records      MedicalRecordRepository
	appointments AppointmentRepository
	userPatients UserPatientRepository
	doctors      DoctorRepository
	storage      FileStorage
	outbox       OutboxRepository
	tx           TransactionManager
	now          func() time.Time
	logger       Logger
}

func NewMedicalRecordService(
	records MedicalRecordRepository,
	appointments AppointmentRepository,
	userPatients UserPatientRepository,
	doctors DoctorRepository,
	storage FileStorage,
	outbox OutboxRepository,
	tx TransactionManager,
	now func() time.Time,
	logger Logger,
) (*MedicalRecordService, error) {
	if records == nil {
		return nil, apperrors.ErrNilMedicalRecordRepository
	}
	if appointments == nil {
		return nil, apperrors.ErrNilAppointmentRepository
	}
	if userPatients == nil {
		return nil, apperrors.ErrNilUserPatientRepository
	}
	if doctors == nil {
		return nil, apperrors.ErrNilDoctorRepository
	}
	if storage == nil {
		return nil, apperrors.ErrNilFileStorage
	}
	if outbox == nil {
		return nil, apperrors.ErrNilOutboxRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	if now == nil {
		return nil, apperrors.ErrNilClock
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}

	return &MedicalRecordService{
		records:      records,
		appointments: appointments,
		userPatients: userPatients,
		doctors:      doctors,
		storage:      storage,
		outbox:       outbox,
		tx:           tx,
		now:          now,
		logger:       logger,
	}, nil
}

// CreateDraft opens the record of an appointment for its doctor once the
// appointment has started. Asking again returns the existing record.
func (mrs *MedicalRecordService) CreateDraft(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	appointmentID uint64,
) (*models.MedicalRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create medical record context: %w", err)
	}
	if appointmentID == 0 {
		return nil, fmt.Errorf("create medical record without appointment id: %w", apperrors.ErrInvalidMedicalRecordInput)
	}
	doctor, err := mrs.actingDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}

	appointment, err := mrs.appointments.GetByID(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrAppointmentNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get appointment for medical record: %w", err)
	}
	if appointment.DoctorID != doctor.ID {
		return nil, apperrors.ErrAppointmentNotFound
	}

	if existing, err := mrs.records.GetByAppointment(ctx, appointmentID); err == nil {
		return existing, nil
	} else if !errors.Is(err, apperrors.ErrMedicalRecordNotFound) {
		return nil, fmt.Errorf("get medical record by appointment: %w", err)
	}

	if appointment.Status != models.AppointmentStatusScheduled && appointment.Status != models.AppointmentStatusCompleted {
		return nil, apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", fmt.Sprintf("appointment is %s; a record needs a scheduled or completed appointment", appointment.Status))
	}
	if mrs.now().Before(appointment.StartsAt) {
		return nil, apperrors.Invalid(apperrors.ErrAppointmentNotModifiable, "", "appointment has not started yet")
	}

	var created *models.MedicalRecord
	err = mrs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		created, err = mrs.records.Create(txCtx, &models.MedicalRecord{
			AppointmentID:   appointment.ID,
			PatientID:       appointment.PatientID,
			DoctorID:        doctor.ID,
			CreatedByUserID: actingUserID,
		})
		if err != nil {
			return err
		}

		return mrs.logAccess(txCtx, actingUserID, created.PatientID, &created.ID, nil, models.RecordAccessCreate)
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrMedicalRecordExists) {
			return mrs.records.GetByAppointment(ctx, appointmentID)
		}
		return nil, fmt.Errorf("create medical record: %w", err)
	}
	mrs.logger.InfoContext(ctx, "medical record created", "record_id", created.ID, "appointment_id", appointmentID)

	return created, nil
}

// UpdateDraft replaces a draft's notes, vitals and prescription. A finalized
// record is never changed.
func (mrs *MedicalRecordService) UpdateDraft(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	record *models.MedicalRecord,
) (*models.MedicalRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("update medical record context: %w", err)
	}
	if record == nil {
		return nil, apperrors.ErrNilMedicalRecordRequest
	}
	if record.ID == 0 {
		return nil, fmt.Errorf("update medical record without id: %w", apperrors.ErrInvalidMedicalRecordInput)
	}

	normalizeMedicalRecord(record)
	if err := validateMedicalRecord(record); err != nil {
		return nil, err
	}

	current, err := mrs.ownRecord(ctx, actingUserID, actingRole, record.ID)
	if err != nil {
		return nil, err
	}
	if current.Status != models.MedicalRecordStatusDraft {
		return nil, apperrors.Invalid(apperrors.ErrMedicalRecordFinalized, "", "a finalized record cannot be changed")
	}

	err = mrs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := mrs.records.UpdateDraft(txCtx, record); err != nil {
			return err
		}

		return mrs.logAccess(txCtx, actingUserID, current.PatientID, &current.ID, nil, models.RecordAccessUpdate)
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrMedicalRecordFinalized) {
			return nil, apperrors.Invalid(apperrors.ErrMedicalRecordFinalized, "", "a finalized record cannot be changed")
		}
		return nil, fmt.Errorf("update medical record %d: %w", record.ID, err)
	}

	updated, err := mrs.records.GetByID(ctx, record.ID)
	if err != nil {
		return nil, fmt.Errorf("get updated medical record: %w", err)
	}

	mrs.logger.InfoContext(ctx, "medical record updated", "record_id", updated.ID, "appointment_id", updated.AppointmentID)

	return updated, nil
}

// Finalize locks a draft for good and makes it readable by the patient side.
// It needs a diagnosis.
func (mrs *MedicalRecordService) Finalize(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	recordID uint64,
) (*models.MedicalRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("finalize medical record context: %w", err)
	}
	if recordID == 0 {
		return nil, fmt.Errorf("finalize medical record without id: %w", apperrors.ErrInvalidMedicalRecordInput)
	}

	record, err := mrs.ownRecord(ctx, actingUserID, actingRole, recordID)
	if err != nil {
		return nil, err
	}
	if record.Status != models.MedicalRecordStatusDraft {
		return nil, apperrors.Invalid(apperrors.ErrMedicalRecordFinalized, "", "the record is already finalized")
	}
	if record.Diagnosis == "" {
		return nil, apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "diagnosis", "is required to finalize")
	}

	err = mrs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := mrs.records.Finalize(txCtx, recordID); err != nil {
			return err
		}
		if err := mrs.logAccess(txCtx, actingUserID, record.PatientID, &record.ID, nil, models.RecordAccessFinalize); err != nil {
			return err
		}

		return mrs.outbox.Create(txCtx, &models.OutboxEvent{
			AggregateType: models.OutboxAggregateMedicalRecord,
			AggregateID:   record.ID,
			EventType:     models.OutboxEventMedicalRecordFinalized,
			Payload: map[string]any{
				"medicalRecordId": record.ID,
				"appointmentId":   record.AppointmentID,
				"patientId":       record.PatientID,
				"doctorId":        record.DoctorID,
			},
		})
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrMedicalRecordFinalized) {
			return nil, apperrors.Invalid(apperrors.ErrMedicalRecordFinalized, "", "the record is already finalized")
		}
		return nil, fmt.Errorf("finalize medical record %d: %w", recordID, err)
	}

	finalized, err := mrs.records.GetByID(ctx, recordID)
	if err != nil {
		return nil, fmt.Errorf("get finalized medical record: %w", err)
	}

	mrs.logger.InfoContext(ctx, "medical record finalized", "record_id", finalized.ID, "appointment_id", finalized.AppointmentID)

	return finalized, nil
}

// GetByID returns a record with its attachments to its doctor, in any status,
// or to the patient side with full access once it is finalized. Anyone else
// gets ErrMedicalRecordNotFound.
func (mrs *MedicalRecordService) GetByID(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	recordID uint64,
) (*models.MedicalRecord, []*models.MedicalAttachment, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("get medical record context: %w", err)
	}
	if actingUserID == 0 || recordID == 0 {
		return nil, nil, fmt.Errorf("get medical record with missing ids: %w", apperrors.ErrInvalidMedicalRecordInput)
	}

	record, err := mrs.records.GetByID(ctx, recordID)
	if err != nil {
		if errors.Is(err, apperrors.ErrMedicalRecordNotFound) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("get medical record: %w", err)
	}
	if !mrs.isRecordDoctor(ctx, actingUserID, actingRole, record) {
		if record.Status != models.MedicalRecordStatusFinalized {
			return nil, nil, apperrors.ErrMedicalRecordNotFound
		}
		if err := mrs.checkReadAccess(ctx, actingUserID, actingRole, record.PatientID); err != nil {
			if errors.Is(err, apperrors.ErrPatientNotFound) {
				return nil, nil, apperrors.ErrMedicalRecordNotFound
			}
			return nil, nil, err
		}
	}

	var attachments []*models.MedicalAttachment
	err = mrs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		attachments, err = mrs.records.ListAttachmentsByRecord(txCtx, recordID)
		if err != nil {
			return err
		}

		return mrs.logAccess(txCtx, actingUserID, record.PatientID, &record.ID, nil, models.RecordAccessView)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("read medical record %d: %w", recordID, err)
	}

	return record, attachments, nil
}

// ListForPatient returns a patient's finalized records, newest first, to the
// patient side with full access.
func (mrs *MedicalRecordService) ListForPatient(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
	limit int,
	offset int,
) ([]*models.MedicalRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list medical records context: %w", err)
	}
	if actingUserID == 0 || patientID == 0 {
		return nil, fmt.Errorf("list medical records with missing ids: %w", apperrors.ErrInvalidMedicalRecordInput)
	}
	if err := mrs.checkFullAccess(ctx, actingUserID, patientID); err != nil {
		return nil, err
	}

	limit, offset = clampPagination(limit, offset)

	var records []*models.MedicalRecord
	err := mrs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		records, err = mrs.records.ListFinalizedByPatient(txCtx, patientID, limit, offset)
		if err != nil {
			return err
		}

		return mrs.logAccess(txCtx, actingUserID, patientID, nil, nil, models.RecordAccessList)
	})
	if err != nil {
		return nil, fmt.Errorf("list medical records of patient %d: %w", patientID, err)
	}

	return records, nil
}

// ListForDoctor returns what a doctor treating the patient sees: every
// finalized record from any doctor and the doctor's own drafts, newest first.
// A patient without a counted booking with the doctor reads as not found.
func (mrs *MedicalRecordService) ListForDoctor(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	patientID uint64,
	limit int,
	offset int,
) ([]*models.MedicalRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list patient records for doctor context: %w", err)
	}
	if patientID == 0 {
		return nil, fmt.Errorf("list patient records for doctor without patient id: %w", apperrors.ErrInvalidMedicalRecordInput)
	}

	doctor, err := mrs.treatingDoctor(ctx, actingUserID, actingRole, patientID)
	if err != nil {
		return nil, err
	}

	limit, offset = clampPagination(limit, offset)

	var records []*models.MedicalRecord
	err = mrs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		records, err = mrs.records.ListForDoctorView(txCtx, patientID, doctor.ID, limit, offset)
		if err != nil {
			return err
		}

		return mrs.logAccess(txCtx, actingUserID, patientID, nil, nil, models.RecordAccessList)
	})
	if err != nil {
		return nil, fmt.Errorf("list medical records of patient %d for doctor: %w", patientID, err)
	}

	return records, nil
}

// ListAttachmentsForDoctor returns a patient's attachments, newest first, to a
// doctor treating the patient. Anyone else reads the patient as not found.
func (mrs *MedicalRecordService) ListAttachmentsForDoctor(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	patientID uint64,
	limit int,
	offset int,
) ([]*models.MedicalAttachment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list patient attachments for doctor context: %w", err)
	}
	if patientID == 0 {
		return nil, fmt.Errorf("list patient attachments for doctor without patient id: %w", apperrors.ErrInvalidMedicalRecordInput)
	}

	if _, err := mrs.treatingDoctor(ctx, actingUserID, actingRole, patientID); err != nil {
		return nil, err
	}

	limit, offset = clampPagination(limit, offset)

	var attachments []*models.MedicalAttachment
	err := mrs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		attachments, err = mrs.records.ListAttachmentsByPatient(txCtx, patientID, limit, offset)
		if err != nil {
			return err
		}

		return mrs.logAccess(txCtx, actingUserID, patientID, nil, nil, models.RecordAccessList)
	})
	if err != nil {
		return nil, fmt.Errorf("list attachments of patient %d for doctor: %w", patientID, err)
	}

	return attachments, nil
}

// ListAccessLog returns who touched a record or its attachments. It shows no
// clinical content; the route permission decides who may see it.
func (mrs *MedicalRecordService) ListAccessLog(
	ctx context.Context,
	recordID uint64,
	limit int,
	offset int,
) ([]*models.RecordAccess, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list access log context: %w", err)
	}
	if recordID == 0 {
		return nil, fmt.Errorf("list access log without record id: %w", apperrors.ErrInvalidMedicalRecordInput)
	}

	if _, err := mrs.records.GetByID(ctx, recordID); err != nil {
		if errors.Is(err, apperrors.ErrMedicalRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get medical record: %w", err)
	}

	limit, offset = clampPagination(limit, offset)

	entries, err := mrs.records.ListAccessLog(ctx, recordID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list access log: %w", err)
	}

	return entries, nil
}

// UploadAttachment stores a report or scan. The patient side with full access
// uploads to the patient outside a visit; the appointment's doctor uploads to
// their own draft record. The file type is detected from its content and only
// PDF, JPEG and PNG are kept.
func (mrs *MedicalRecordService) UploadAttachment(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	attachment *models.MedicalAttachment,
	content io.Reader,
) (*models.MedicalAttachment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("upload attachment context: %w", err)
	}
	if attachment == nil || content == nil {
		return nil, apperrors.ErrNilMedicalRecordRequest
	}
	if actingUserID == 0 {
		return nil, fmt.Errorf("upload attachment without acting user: %w", apperrors.ErrInvalidMedicalRecordInput)
	}

	attachment.Title = strings.TrimSpace(attachment.Title)
	if err := validateAttachment(attachment); err != nil {
		return nil, err
	}

	if attachment.MedicalRecordID != nil {
		record, err := mrs.ownRecord(ctx, actingUserID, actingRole, *attachment.MedicalRecordID)
		if err != nil {
			return nil, err
		}
		if record.Status != models.MedicalRecordStatusDraft {
			return nil, apperrors.Invalid(apperrors.ErrMedicalRecordFinalized, "", "a finalized record cannot be changed")
		}
		attachment.PatientID = record.PatientID
	} else {
		if attachment.PatientID == 0 {
			return nil, apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "patientId", "is required")
		}
		if err := mrs.checkFullAccess(ctx, actingUserID, attachment.PatientID); err != nil {
			return nil, err
		}
	}

	head := make([]byte, constant.ContentSniffBytes)
	read, err := io.ReadFull(content, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read attachment: %w", err)
	}
	if read == 0 {
		return nil, apperrors.Invalid(apperrors.ErrInvalidMedicalRecordInput, "file", "is empty")
	}
	contentType := http.DetectContentType(head[:read])
	if !slices.Contains(constant.AttachmentContentTypes, contentType) {
		return nil, apperrors.Invalid(apperrors.ErrAttachmentType, "file", "must be a PDF, JPEG or PNG")
	}

	token, err := utils.NewOpaqueToken()
	if err != nil {
		return nil, fmt.Errorf("create storage key: %w", err)
	}
	key := fmt.Sprintf("patients/%d/attachments/%s", attachment.PatientID, token)

	counted := &countingReader{reader: io.LimitReader(io.MultiReader(bytes.NewReader(head[:read]), content), constant.MaxAttachmentBytes+1)}
	if err := mrs.storage.Put(ctx, key, counted, contentType); err != nil {
		return nil, fmt.Errorf("store attachment: %w", err)
	}
	if counted.read > constant.MaxAttachmentBytes {
		_ = mrs.storage.Delete(ctx, key)
		return nil, apperrors.Invalid(apperrors.ErrAttachmentTooLarge, "file", fmt.Sprintf("must be at most %d MB", constant.MaxAttachmentBytes>>20))
	}

	attachment.UploadedByUserID = actingUserID
	attachment.StorageKey = key
	attachment.ContentType = contentType
	attachment.SizeBytes = counted.read

	var created *models.MedicalAttachment
	err = mrs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		created, err = mrs.records.CreateAttachment(txCtx, attachment)
		if err != nil {
			return err
		}

		return mrs.logAccess(txCtx, actingUserID, created.PatientID, created.MedicalRecordID, &created.ID, models.RecordAccessUpload)
	})
	if err != nil {
		_ = mrs.storage.Delete(ctx, key)
		return nil, fmt.Errorf("save attachment: %w", err)
	}

	mrs.logger.InfoContext(ctx, "attachment uploaded", "attachment_id", created.ID, "record_id", created.MedicalRecordID, "size_bytes", created.SizeBytes)

	return created, nil
}

// ListAttachments returns a patient's attachments, newest first, to the
// patient side with full access.
func (mrs *MedicalRecordService) ListAttachments(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
	limit int,
	offset int,
) ([]*models.MedicalAttachment, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list attachments context: %w", err)
	}
	if actingUserID == 0 || patientID == 0 {
		return nil, fmt.Errorf("list attachments with missing ids: %w", apperrors.ErrInvalidMedicalRecordInput)
	}
	if err := mrs.checkFullAccess(ctx, actingUserID, patientID); err != nil {
		return nil, err
	}

	limit, offset = clampPagination(limit, offset)

	var attachments []*models.MedicalAttachment
	err := mrs.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		var err error
		attachments, err = mrs.records.ListAttachmentsByPatient(txCtx, patientID, limit, offset)
		if err != nil {
			return err
		}

		return mrs.logAccess(txCtx, actingUserID, patientID, nil, nil, models.RecordAccessList)
	})
	if err != nil {
		return nil, fmt.Errorf("list attachments of patient %d: %w", patientID, err)
	}

	return attachments, nil
}

// OpenAttachment returns an attachment's file to the patient side with full
// access, to the doctor of the record it belongs to, or to a doctor treating
// the patient. The caller closes it.
func (mrs *MedicalRecordService) OpenAttachment(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	attachmentID uint64,
) (*models.MedicalAttachment, io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("open attachment context: %w", err)
	}
	if actingUserID == 0 || attachmentID == 0 {
		return nil, nil, fmt.Errorf("open attachment with missing ids: %w", apperrors.ErrInvalidMedicalRecordInput)
	}

	attachment, err := mrs.records.GetAttachment(ctx, attachmentID)
	if err != nil {
		if errors.Is(err, apperrors.ErrAttachmentNotFound) {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("get attachment: %w", err)
	}

	allowed := false
	if attachment.MedicalRecordID != nil {
		record, err := mrs.records.GetByID(ctx, *attachment.MedicalRecordID)
		if err != nil {
			return nil, nil, fmt.Errorf("get record of attachment: %w", err)
		}
		allowed = mrs.isRecordDoctor(ctx, actingUserID, actingRole, record)
	}
	if !allowed {
		if err := mrs.checkReadAccess(ctx, actingUserID, actingRole, attachment.PatientID); err != nil {
			if errors.Is(err, apperrors.ErrPatientNotFound) {
				return nil, nil, apperrors.ErrAttachmentNotFound
			}
			return nil, nil, err
		}
	}

	if err := mrs.logAccess(ctx, actingUserID, attachment.PatientID, attachment.MedicalRecordID, &attachment.ID, models.RecordAccessDownload); err != nil {
		return nil, nil, fmt.Errorf("log attachment download: %w", err)
	}

	file, err := mrs.storage.Open(ctx, attachment.StorageKey)
	if err != nil {
		return nil, nil, fmt.Errorf("open stored attachment %d: %w", attachmentID, err)
	}

	return attachment, file, nil
}

// actingDoctor returns the doctor profile of a doctor account.
func (mrs *MedicalRecordService) actingDoctor(ctx context.Context, actingUserID uint64, actingRole models.Role) (*models.Doctor, error) {
	if err := validateDoctorActor(actingUserID, actingRole); err != nil {
		return nil, err
	}

	doctor, err := mrs.doctors.GetByUserID(ctx, actingUserID)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get own doctor profile: %w", err)
	}

	return doctor, nil
}

// ownRecord loads a record that belongs to the acting doctor. Another doctor's
// record reads as not found.
func (mrs *MedicalRecordService) ownRecord(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	recordID uint64,
) (*models.MedicalRecord, error) {
	doctor, err := mrs.actingDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}

	record, err := mrs.records.GetByID(ctx, recordID)
	if err != nil {
		if errors.Is(err, apperrors.ErrMedicalRecordNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get medical record: %w", err)
	}
	if record.DoctorID != doctor.ID {
		return nil, apperrors.ErrMedicalRecordNotFound
	}

	return record, nil
}

func (mrs *MedicalRecordService) isRecordDoctor(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	record *models.MedicalRecord,
) bool {
	if actingRole != models.RoleDoctor {
		return false
	}

	doctor, err := mrs.doctors.GetByUserID(ctx, actingUserID)
	return err == nil && doctor.ID == record.DoctorID
}

// checkFullAccess requires an active link with full access to the patient.
// Clinical data is stricter than appointments: appointments_only and read_only
// links read as not found, the same as no link.
func (mrs *MedicalRecordService) checkFullAccess(ctx context.Context, actingUserID uint64, patientID uint64) error {
	link, err := mrs.userPatients.GetActiveByUserAndPatient(ctx, actingUserID, patientID)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserPatientNotFound) {
			return apperrors.ErrPatientNotFound
		}
		return fmt.Errorf("authorize medical record access: %w", err)
	}
	if link.AccessLevel != models.AccessLevelFull {
		return apperrors.ErrPatientNotFound
	}

	return nil
}

// treatingDoctor returns the acting doctor when the patient has a counted
// booking with them (the doctor's patient list). Anyone else, a doctor
// without such a booking included, reads the patient as not found.
func (mrs *MedicalRecordService) treatingDoctor(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	patientID uint64,
) (*models.Doctor, error) {
	doctor, err := mrs.actingDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) || errors.Is(err, apperrors.ErrDoctorRoleRequired) {
			return nil, apperrors.ErrPatientNotFound
		}
		return nil, err
	}

	treats, err := mrs.appointments.HasCountedBooking(ctx, doctor.ID, patientID)
	if err != nil {
		return nil, fmt.Errorf("check doctor's patient: %w", err)
	}
	if !treats {
		return nil, apperrors.ErrPatientNotFound
	}

	return doctor, nil
}

// checkReadAccess allows a doctor treating the patient or the patient side
// with full access to read the patient's finalized records and files.
func (mrs *MedicalRecordService) checkReadAccess(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	patientID uint64,
) error {
	if actingRole == models.RoleDoctor {
		_, err := mrs.treatingDoctor(ctx, actingUserID, actingRole, patientID)
		return err
	}

	return mrs.checkFullAccess(ctx, actingUserID, patientID)
}

func (mrs *MedicalRecordService) logAccess(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
	recordID *uint64,
	attachmentID *uint64,
	action models.RecordAccessAction,
) error {
	correlationID := ""
	if applicationContext, ok := appcontext.From(ctx); ok {
		correlationID = applicationContext.CorrelationID
	}

	return mrs.records.LogAccess(ctx, &models.RecordAccess{
		PatientID:        patientID,
		MedicalRecordID:  recordID,
		AttachmentID:     attachmentID,
		AccessedByUserID: actingUserID,
		Action:           action,
		CorrelationID:    correlationID,
	})
}

// countingReader counts the bytes read through it, so an upload's real size is
// known without trusting the client.
type countingReader struct {
	reader io.Reader
	read   int64
}

func (cr *countingReader) Read(buffer []byte) (int, error) {
	read, err := cr.reader.Read(buffer)
	cr.read += int64(read)
	return read, err
}
