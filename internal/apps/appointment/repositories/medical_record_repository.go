package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type MedicalRecordRepository struct {
	db *gorm.DB
}

func NewMedicalRecordRepository(db *gorm.DB) (*MedicalRecordRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &MedicalRecordRepository{db: db}, nil
}

type medicalRecordRecord struct {
	ID                uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	AppointmentID     uint64     `gorm:"column:appointment_id"`
	PatientID         uint64     `gorm:"column:patient_id"`
	DoctorID          uint64     `gorm:"column:doctor_id"`
	CreatedByUserID   uint64     `gorm:"column:created_by_user_id"`
	ChiefComplaint    string     `gorm:"column:chief_complaint"`
	Symptoms          string     `gorm:"column:symptoms"`
	Diagnosis         string     `gorm:"column:diagnosis"`
	ExaminationNotes  string     `gorm:"column:examination_notes"`
	Advice            string     `gorm:"column:advice"`
	PrescriptionNotes string     `gorm:"column:prescription_notes"`
	FollowUpOn        *time.Time `gorm:"column:follow_up_on"`
	Vitals            string     `gorm:"column:vitals"`
	Status            string     `gorm:"column:status"`
	FinalizedAt       *time.Time `gorm:"column:finalized_at"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at"`
}

func (medicalRecordRecord) TableName() string {
	return "medical_records"
}

// vitalsRecord is the JSON shape of medical_records.vitals.
type vitalsRecord struct {
	BloodPressure      string   `json:"bloodPressure,omitempty"`
	PulsePerMinute     *int     `json:"pulsePerMinute,omitempty"`
	TemperatureCelsius *float64 `json:"temperatureCelsius,omitempty"`
	WeightKg           *float64 `json:"weightKg,omitempty"`
	SpO2Percent        *int     `json:"spo2Percent,omitempty"`
}

type prescriptionItemRecord struct {
	ID              uint64 `gorm:"primaryKey;autoIncrement;column:id"`
	MedicalRecordID uint64 `gorm:"column:medical_record_id"`
	Position        int    `gorm:"column:position"`
	MedicineName    string `gorm:"column:medicine_name"`
	Strength        string `gorm:"column:strength"`
	Form            string `gorm:"column:form"`
	Dosage          string `gorm:"column:dosage"`
	Frequency       string `gorm:"column:frequency"`
	DurationDays    int    `gorm:"column:duration_days"`
	Timing          string `gorm:"column:timing"`
	Instructions    string `gorm:"column:instructions"`
}

func (prescriptionItemRecord) TableName() string {
	return "prescription_items"
}

type medicalAttachmentRecord struct {
	ID               uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	PatientID        uint64     `gorm:"column:patient_id"`
	MedicalRecordID  *uint64    `gorm:"column:medical_record_id"`
	UploadedByUserID uint64     `gorm:"column:uploaded_by_user_id"`
	Kind             string     `gorm:"column:kind"`
	Title            string     `gorm:"column:title"`
	StorageKey       string     `gorm:"column:storage_key"`
	ContentType      string     `gorm:"column:content_type"`
	SizeBytes        int64      `gorm:"column:size_bytes"`
	TakenOn          *time.Time `gorm:"column:taken_on"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
}

func (medicalAttachmentRecord) TableName() string {
	return "medical_attachments"
}

type recordAccessRecord struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement;column:id"`
	PatientID        uint64    `gorm:"column:patient_id"`
	MedicalRecordID  *uint64   `gorm:"column:medical_record_id"`
	AttachmentID     *uint64   `gorm:"column:attachment_id"`
	AccessedByUserID uint64    `gorm:"column:accessed_by_user_id"`
	Action           string    `gorm:"column:action"`
	CorrelationID    string    `gorm:"column:correlation_id"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (recordAccessRecord) TableName() string {
	return "medical_record_access_log"
}

// Create inserts an empty draft for an appointment. An appointment has at most
// one record; a second one returns ErrMedicalRecordExists.
func (repo *MedicalRecordRepository) Create(ctx context.Context, record *models.MedicalRecord) (*models.MedicalRecord, error) {
	if record == nil {
		return nil, apperrors.ErrNilMedicalRecordRequest
	}

	now := time.Now().UTC()
	row := medicalRecordRecord{
		AppointmentID:   record.AppointmentID,
		PatientID:       record.PatientID,
		DoctorID:        record.DoctorID,
		CreatedByUserID: record.CreatedByUserID,
		Vitals:          "{}",
		Status:          string(models.MedicalRecordStatusDraft),
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := dbFromContext(ctx, repo.db).Create(&row).Error; err != nil {
		if medicalRecordAppointmentUnique.ViolatedBy(err) {
			return nil, apperrors.ErrMedicalRecordExists
		}
		return nil, fmt.Errorf("create medical record: %w", err)
	}

	return toMedicalRecordModel(row, nil)
}

func (repo *MedicalRecordRepository) GetByID(ctx context.Context, id uint64) (*models.MedicalRecord, error) {
	return repo.getWhere(ctx, "id = ?", id)
}

func (repo *MedicalRecordRepository) GetByAppointment(ctx context.Context, appointmentID uint64) (*models.MedicalRecord, error) {
	return repo.getWhere(ctx, "appointment_id = ?", appointmentID)
}

// UpdateDraft replaces a draft's notes, vitals and medicines. It only matches a
// record that is still a draft, so a finalized record never changes. Callers
// run it inside a transaction so the notes and medicines change together.
func (repo *MedicalRecordRepository) UpdateDraft(ctx context.Context, record *models.MedicalRecord) error {
	if record == nil {
		return apperrors.ErrNilMedicalRecordRequest
	}

	vitals, err := json.Marshal(toVitalsRecord(record.Vitals))
	if err != nil {
		return fmt.Errorf("encode vitals: %w", err)
	}

	db := dbFromContext(ctx, repo.db)
	var followUpOn *time.Time
	if record.FollowUpOn != nil {
		date := utils.CivilDate(*record.FollowUpOn)
		followUpOn = &date
	}
	result := db.Model(&medicalRecordRecord{}).
		Where("id = ? AND status = ?", record.ID, string(models.MedicalRecordStatusDraft)).
		Updates(map[string]interface{}{
			"chief_complaint":    record.ChiefComplaint,
			"symptoms":           record.Symptoms,
			"diagnosis":          record.Diagnosis,
			"examination_notes":  record.ExaminationNotes,
			"advice":             record.Advice,
			"prescription_notes": record.PrescriptionNotes,
			"follow_up_on":       followUpOn,
			"vitals":             string(vitals),
			"updated_at":         time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update medical record draft: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrMedicalRecordFinalized
	}

	if err := db.Where("medical_record_id = ?", record.ID).Delete(&prescriptionItemRecord{}).Error; err != nil {
		return fmt.Errorf("delete prescription items: %w", err)
	}
	if len(record.Medicines) == 0 {
		return nil
	}

	items := make([]prescriptionItemRecord, 0, len(record.Medicines))
	for position, medicine := range record.Medicines {
		items = append(items, prescriptionItemRecord{
			MedicalRecordID: record.ID,
			Position:        position,
			MedicineName:    medicine.MedicineName,
			Strength:        medicine.Strength,
			Form:            medicine.Form,
			Dosage:          medicine.Dosage,
			Frequency:       medicine.Frequency,
			DurationDays:    medicine.DurationDays,
			Timing:          string(medicine.Timing),
			Instructions:    medicine.Instructions,
		})
	}
	if err := db.Create(&items).Error; err != nil {
		return fmt.Errorf("create prescription items: %w", err)
	}

	return nil
}

// Finalize locks a draft for good. It only matches a record that is still a
// draft, so a record is finalized once.
func (repo *MedicalRecordRepository) Finalize(ctx context.Context, id uint64) error {
	now := time.Now().UTC()
	result := dbFromContext(ctx, repo.db).
		Model(&medicalRecordRecord{}).
		Where("id = ? AND status = ?", id, string(models.MedicalRecordStatusDraft)).
		Updates(map[string]interface{}{
			"status":       string(models.MedicalRecordStatusFinalized),
			"finalized_at": now,
			"updated_at":   now,
		})
	if result.Error != nil {
		return fmt.Errorf("finalize medical record: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrMedicalRecordFinalized
	}

	return nil
}

// ListFinalizedByPatient returns a patient's finalized records, newest visit
// first, with their medicines.
func (repo *MedicalRecordRepository) ListFinalizedByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.MedicalRecord, error) {
	var rows []medicalRecordRecord

	if err := dbFromContext(ctx, repo.db).
		Where("patient_id = ? AND status = ?", patientID, string(models.MedicalRecordStatusFinalized)).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list finalized medical records: %w", err)
	}

	return repo.withItems(ctx, rows)
}

// ListForDoctorView returns what a treating doctor sees of a patient: every
// finalized record from any doctor and the doctor's own drafts, newest first,
// with their medicines.
func (repo *MedicalRecordRepository) ListForDoctorView(
	ctx context.Context,
	patientID uint64,
	doctorID uint64,
	limit int,
	offset int,
) ([]*models.MedicalRecord, error) {
	var rows []medicalRecordRecord

	if err := dbFromContext(ctx, repo.db).
		Where("patient_id = ? AND (status = ? OR doctor_id = ?)", patientID, string(models.MedicalRecordStatusFinalized), doctorID).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list medical records for doctor view: %w", err)
	}

	return repo.withItems(ctx, rows)
}

func (repo *MedicalRecordRepository) CreateAttachment(ctx context.Context, attachment *models.MedicalAttachment) (*models.MedicalAttachment, error) {
	if attachment == nil {
		return nil, apperrors.ErrNilMedicalRecordRequest
	}

	row := medicalAttachmentRecord{
		PatientID:        attachment.PatientID,
		MedicalRecordID:  attachment.MedicalRecordID,
		UploadedByUserID: attachment.UploadedByUserID,
		Kind:             string(attachment.Kind),
		Title:            attachment.Title,
		StorageKey:       attachment.StorageKey,
		ContentType:      attachment.ContentType,
		SizeBytes:        attachment.SizeBytes,
		CreatedAt:        time.Now().UTC(),
	}
	if attachment.TakenOn != nil {
		takenOn := utils.CivilDate(*attachment.TakenOn)
		row.TakenOn = &takenOn
	}

	if err := dbFromContext(ctx, repo.db).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("create medical attachment: %w", err)
	}

	return toMedicalAttachmentModel(row), nil
}

func (repo *MedicalRecordRepository) GetAttachment(ctx context.Context, id uint64) (*models.MedicalAttachment, error) {
	var row medicalAttachmentRecord

	if err := dbFromContext(ctx, repo.db).Where("id = ?", id).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrAttachmentNotFound
		}
		return nil, fmt.Errorf("get medical attachment: %w", err)
	}

	return toMedicalAttachmentModel(row), nil
}

// ListAttachmentsByPatient returns every attachment of a patient, newest first.
func (repo *MedicalRecordRepository) ListAttachmentsByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.MedicalAttachment, error) {
	return repo.listAttachments(ctx, dbFromContext(ctx, repo.db).Where("patient_id = ?", patientID).Limit(limit).Offset(offset))
}

// ListAttachmentsByRecord returns the attachments added to one record.
func (repo *MedicalRecordRepository) ListAttachmentsByRecord(ctx context.Context, recordID uint64) ([]*models.MedicalAttachment, error) {
	return repo.listAttachments(ctx, dbFromContext(ctx, repo.db).Where("medical_record_id = ?", recordID))
}

func (repo *MedicalRecordRepository) LogAccess(ctx context.Context, access *models.RecordAccess) error {
	if access == nil {
		return apperrors.ErrNilMedicalRecordRequest
	}

	row := recordAccessRecord{
		PatientID:        access.PatientID,
		MedicalRecordID:  access.MedicalRecordID,
		AttachmentID:     access.AttachmentID,
		AccessedByUserID: access.AccessedByUserID,
		Action:           string(access.Action),
		CorrelationID:    access.CorrelationID,
		CreatedAt:        time.Now().UTC(),
	}

	if err := dbFromContext(ctx, repo.db).Create(&row).Error; err != nil {
		return fmt.Errorf("log medical record access: %w", err)
	}

	return nil
}

// ListAccessLog returns who touched a record or its attachments, newest first.
func (repo *MedicalRecordRepository) ListAccessLog(ctx context.Context, recordID uint64, limit int, offset int) ([]*models.RecordAccess, error) {
	var rows []recordAccessRecord

	if err := dbFromContext(ctx, repo.db).
		Where("medical_record_id = ? OR attachment_id IN (?)", recordID,
			dbFromContext(ctx, repo.db).Model(&medicalAttachmentRecord{}).Select("id").Where("medical_record_id = ?", recordID)).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list medical record access log: %w", err)
	}

	result := make([]*models.RecordAccess, 0, len(rows))
	for _, row := range rows {
		result = append(result, &models.RecordAccess{
			ID:               row.ID,
			PatientID:        row.PatientID,
			MedicalRecordID:  row.MedicalRecordID,
			AttachmentID:     row.AttachmentID,
			AccessedByUserID: row.AccessedByUserID,
			Action:           models.RecordAccessAction(row.Action),
			CorrelationID:    row.CorrelationID,
			CreatedAt:        row.CreatedAt,
		})
	}

	return result, nil
}

func (repo *MedicalRecordRepository) getWhere(ctx context.Context, condition string, value uint64) (*models.MedicalRecord, error) {
	var row medicalRecordRecord

	if err := dbFromContext(ctx, repo.db).Where(condition, value).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrMedicalRecordNotFound
		}
		return nil, fmt.Errorf("get medical record: %w", err)
	}

	itemsByRecord, err := repo.itemsByRecord(ctx, []uint64{row.ID})
	if err != nil {
		return nil, err
	}

	return toMedicalRecordModel(row, itemsByRecord[row.ID])
}

// withItems loads the medicines of listed records and maps them to models.
func (repo *MedicalRecordRepository) withItems(ctx context.Context, rows []medicalRecordRecord) ([]*models.MedicalRecord, error) {
	if len(rows) == 0 {
		return []*models.MedicalRecord{}, nil
	}

	ids := make([]uint64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	itemsByRecord, err := repo.itemsByRecord(ctx, ids)
	if err != nil {
		return nil, err
	}

	result := make([]*models.MedicalRecord, 0, len(rows))
	for _, row := range rows {
		record, err := toMedicalRecordModel(row, itemsByRecord[row.ID])
		if err != nil {
			return nil, err
		}
		result = append(result, record)
	}

	return result, nil
}

func (repo *MedicalRecordRepository) itemsByRecord(ctx context.Context, recordIDs []uint64) (map[uint64][]prescriptionItemRecord, error) {
	var items []prescriptionItemRecord

	if err := dbFromContext(ctx, repo.db).
		Where("medical_record_id IN ?", recordIDs).
		Order("medical_record_id ASC, position ASC").
		Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list prescription items: %w", err)
	}

	result := make(map[uint64][]prescriptionItemRecord, len(recordIDs))
	for _, item := range items {
		result[item.MedicalRecordID] = append(result[item.MedicalRecordID], item)
	}

	return result, nil
}

func (repo *MedicalRecordRepository) listAttachments(ctx context.Context, query *gorm.DB) ([]*models.MedicalAttachment, error) {
	var rows []medicalAttachmentRecord

	if err := query.Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list medical attachments: %w", err)
	}

	result := make([]*models.MedicalAttachment, 0, len(rows))
	for _, row := range rows {
		result = append(result, toMedicalAttachmentModel(row))
	}

	return result, nil
}

func toMedicalRecordModel(row medicalRecordRecord, items []prescriptionItemRecord) (*models.MedicalRecord, error) {
	var vitals vitalsRecord
	if err := json.Unmarshal([]byte(row.Vitals), &vitals); err != nil {
		return nil, fmt.Errorf("decode vitals of medical record %d: %w", row.ID, err)
	}

	medicines := make([]models.PrescriptionItem, 0, len(items))
	for _, item := range items {
		medicines = append(medicines, models.PrescriptionItem{
			MedicineName: item.MedicineName,
			Strength:     item.Strength,
			Form:         item.Form,
			Dosage:       item.Dosage,
			Frequency:    item.Frequency,
			DurationDays: item.DurationDays,
			Timing:       models.MedicineTiming(item.Timing),
			Instructions: item.Instructions,
		})
	}

	return &models.MedicalRecord{
		ID:                row.ID,
		AppointmentID:     row.AppointmentID,
		PatientID:         row.PatientID,
		DoctorID:          row.DoctorID,
		CreatedByUserID:   row.CreatedByUserID,
		ChiefComplaint:    row.ChiefComplaint,
		Symptoms:          row.Symptoms,
		Diagnosis:         row.Diagnosis,
		ExaminationNotes:  row.ExaminationNotes,
		Advice:            row.Advice,
		PrescriptionNotes: row.PrescriptionNotes,
		FollowUpOn:        row.FollowUpOn,
		Vitals: models.Vitals{
			BloodPressure:      vitals.BloodPressure,
			PulsePerMinute:     vitals.PulsePerMinute,
			TemperatureCelsius: vitals.TemperatureCelsius,
			WeightKg:           vitals.WeightKg,
			SpO2Percent:        vitals.SpO2Percent,
		},
		Medicines:   medicines,
		Status:      models.MedicalRecordStatus(row.Status),
		FinalizedAt: row.FinalizedAt,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}, nil
}

func toVitalsRecord(vitals models.Vitals) vitalsRecord {
	return vitalsRecord{
		BloodPressure:      vitals.BloodPressure,
		PulsePerMinute:     vitals.PulsePerMinute,
		TemperatureCelsius: vitals.TemperatureCelsius,
		WeightKg:           vitals.WeightKg,
		SpO2Percent:        vitals.SpO2Percent,
	}
}

func toMedicalAttachmentModel(row medicalAttachmentRecord) *models.MedicalAttachment {
	return &models.MedicalAttachment{
		ID:               row.ID,
		PatientID:        row.PatientID,
		MedicalRecordID:  row.MedicalRecordID,
		UploadedByUserID: row.UploadedByUserID,
		Kind:             models.AttachmentKind(row.Kind),
		Title:            row.Title,
		StorageKey:       row.StorageKey,
		ContentType:      row.ContentType,
		SizeBytes:        row.SizeBytes,
		TakenOn:          row.TakenOn,
		CreatedAt:        row.CreatedAt,
	}
}
