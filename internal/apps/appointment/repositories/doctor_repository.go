package repositories

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type DoctorRepository struct {
	db *gorm.DB
}

func NewDoctorRepository(db *gorm.DB) (*DoctorRepository, error) {
	if db == nil {
		return nil, apperrors.ErrNilDB
	}
	return &DoctorRepository{db: db}, nil
}

type doctorRecord struct {
	ID                  uint64     `gorm:"primaryKey;autoIncrement;column:id"`
	UserID              uint64     `gorm:"column:user_id"`
	FullName            string     `gorm:"column:full_name"`
	RegistrationNumber  string     `gorm:"column:registration_number"`
	RegistrationCouncil string     `gorm:"column:registration_council"`
	RegistrationYear    *int       `gorm:"column:registration_year"`
	Qualification       string     `gorm:"column:qualification"`
	YearsOfExperience   int        `gorm:"column:years_of_experience"`
	Bio                 string     `gorm:"column:bio"`
	ConsultationFee     int        `gorm:"column:consultation_fee"`
	SlotDurationMin     int        `gorm:"column:slot_duration_min"`
	ClinicName          string     `gorm:"column:clinic_name"`
	ClinicAddress       string     `gorm:"column:clinic_address"`
	City                string     `gorm:"column:city"`
	Pincode             string     `gorm:"column:pincode"`
	VerificationStatus  string     `gorm:"column:verification_status"`
	VerifiedAt          *time.Time `gorm:"column:verified_at"`
	IsAcceptingPatients bool       `gorm:"column:is_accepting_patients"`
	UPIID               string     `gorm:"column:upi_id"`
	UPIQRStorageKey     *string    `gorm:"column:upi_qr_storage_key"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
}

func (doctorRecord) TableName() string {
	return "doctors"
}

type doctorSpecialityRecord struct {
	DoctorID     uint64 `gorm:"primaryKey;column:doctor_id"`
	SpecialityID uint64 `gorm:"primaryKey;column:speciality_id"`
	IsPrimary    bool   `gorm:"column:is_primary"`
}

func (doctorSpecialityRecord) TableName() string {
	return "doctor_specialities"
}

type doctorTagRecord struct {
	DoctorID uint64 `gorm:"primaryKey;column:doctor_id"`
	Tag      string `gorm:"primaryKey;column:tag"`
	TagType  string `gorm:"column:tag_type"`
}

func (doctorTagRecord) TableName() string {
	return "doctor_tags"
}

type doctorConsultationModeRecord struct {
	DoctorID uint64 `gorm:"primaryKey;column:doctor_id"`
	Mode     string `gorm:"primaryKey;column:mode"`
}

func (doctorConsultationModeRecord) TableName() string {
	return "doctor_consultation_modes"
}

type doctorSpecialityRow struct {
	DoctorID  uint64    `gorm:"column:doctor_id"`
	IsPrimary bool      `gorm:"column:is_primary"`
	ID        uint64    `gorm:"column:id"`
	Slug      string    `gorm:"column:slug"`
	Name      string    `gorm:"column:name"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// Create inserts the doctor and its speciality, tag and mode links. Callers run
// it inside a transaction so a failed link leaves no partial profile.
func (repo *DoctorRepository) Create(ctx context.Context, doctor *models.Doctor) (*models.Doctor, error) {
	if doctor == nil {
		return nil, apperrors.ErrNilDoctorRequest
	}

	now := time.Now().UTC()
	record := toDoctorRecord(doctor)
	record.CreatedAt = now
	record.UpdatedAt = now

	if err := dbFromContext(ctx, repo.db).Create(&record).Error; err != nil {
		if doctorProfileUnique.ViolatedBy(err) {
			return nil, apperrors.ErrDoctorAlreadyExists
		}
		if registrationUnique.ViolatedBy(err) {
			return nil, apperrors.ErrDoctorRegistrationExists
		}
		return nil, fmt.Errorf("create doctor record: %w", err)
	}

	if err := repo.replaceLinks(ctx, record.ID, doctor); err != nil {
		return nil, err
	}

	return repo.GetByID(ctx, record.ID)
}

func (repo *DoctorRepository) GetByID(ctx context.Context, id uint64) (*models.Doctor, error) {
	var record doctorRecord

	if err := dbFromContext(ctx, repo.db).
		Where("id = ?", id).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrDoctorNotFound
		}
		return nil, fmt.Errorf("get doctor by id: %w", err)
	}

	return repo.withLinks(ctx, record)
}

func (repo *DoctorRepository) GetByUserID(ctx context.Context, userID uint64) (*models.Doctor, error) {
	var record doctorRecord

	if err := dbFromContext(ctx, repo.db).
		Where("user_id = ?", userID).
		First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrDoctorNotFound
		}
		return nil, fmt.Errorf("get doctor by user id: %w", err)
	}

	return repo.withLinks(ctx, record)
}

// Update replaces the profile fields, status and links. Callers run it inside a
// transaction.
func (repo *DoctorRepository) Update(ctx context.Context, doctor *models.Doctor) (*models.Doctor, error) {
	if doctor == nil {
		return nil, apperrors.ErrNilDoctorRequest
	}
	if doctor.ID == 0 {
		return nil, fmt.Errorf("update doctor without id: %w", apperrors.ErrInvalidDoctorInput)
	}

	record := toDoctorRecord(doctor)
	updates := map[string]interface{}{
		"full_name":             record.FullName,
		"registration_number":   record.RegistrationNumber,
		"registration_council":  record.RegistrationCouncil,
		"registration_year":     record.RegistrationYear,
		"qualification":         record.Qualification,
		"years_of_experience":   record.YearsOfExperience,
		"bio":                   record.Bio,
		"consultation_fee":      record.ConsultationFee,
		"slot_duration_min":     record.SlotDurationMin,
		"clinic_name":           record.ClinicName,
		"clinic_address":        record.ClinicAddress,
		"city":                  record.City,
		"pincode":               record.Pincode,
		"verification_status":   record.VerificationStatus,
		"verified_at":           record.VerifiedAt,
		"is_accepting_patients": record.IsAcceptingPatients,
		"updated_at":            time.Now().UTC(),
	}

	result := dbFromContext(ctx, repo.db).
		Model(&doctorRecord{}).
		Where("id = ?", doctor.ID).
		Updates(updates)
	if result.Error != nil {
		if registrationUnique.ViolatedBy(result.Error) {
			return nil, apperrors.ErrDoctorRegistrationExists
		}
		return nil, fmt.Errorf("update doctor record: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, apperrors.ErrDoctorNotFound
	}

	if err := repo.replaceLinks(ctx, doctor.ID, doctor); err != nil {
		return nil, err
	}

	return repo.GetByID(ctx, doctor.ID)
}

// UpdateUPI sets where patients pay the doctor directly: the UPI ID, empty to
// stop, and the storage key of their QR image, nil for none. Profile updates
// never touch these.
func (repo *DoctorRepository) UpdateUPI(ctx context.Context, id uint64, upiID string, qrStorageKey *string) error {
	result := dbFromContext(ctx, repo.db).
		Model(&doctorRecord{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"upi_id":             upiID,
			"upi_qr_storage_key": qrStorageKey,
			"updated_at":         time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update doctor upi: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrDoctorNotFound
	}

	return nil
}

// UpdateStatus moves a doctor from one verification status to another. It only
// matches a row still in fromStatus, so two concurrent reviews cannot both win.
func (repo *DoctorRepository) UpdateStatus(
	ctx context.Context,
	id uint64,
	fromStatus models.VerificationStatus,
	toStatus models.VerificationStatus,
	verifiedAt *time.Time,
) error {
	result := dbFromContext(ctx, repo.db).
		Model(&doctorRecord{}).
		Where("id = ? AND verification_status = ?", id, string(fromStatus)).
		Updates(map[string]interface{}{
			"verification_status": string(toStatus),
			"verified_at":         verifiedAt,
			"updated_at":          time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update doctor verification status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrInvalidVerificationTransition
	}

	return nil
}

func (repo *DoctorRepository) ListByStatus(
	ctx context.Context,
	status models.VerificationStatus,
	limit int,
	offset int,
) ([]*models.Doctor, error) {
	var records []doctorRecord

	query := dbFromContext(ctx, repo.db).Model(&doctorRecord{})
	if status != "" {
		query = query.Where("verification_status = ?", string(status))
	}

	if err := query.
		Order("created_at ASC, id ASC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list doctors by status: %w", err)
	}

	return repo.withLinksMany(ctx, records)
}

// Search returns verified doctors who accept patients and match every given
// filter. Speciality and tag filters match any of the given values.
func (repo *DoctorRepository) Search(ctx context.Context, search models.DoctorSearch) ([]*models.Doctor, error) {
	var records []doctorRecord

	query := dbFromContext(ctx, repo.db).
		Model(&doctorRecord{}).
		Where("verification_status = ? AND is_accepting_patients = ?", string(models.VerificationStatusVerified), true)

	if search.Query != "" {
		pattern := "%" + utils.EscapeLike(search.Query) + "%"
		query = query.Where(
			"(full_name ILIKE ? OR qualification ILIKE ? OR bio ILIKE ? OR EXISTS (SELECT 1 FROM doctor_tags dt WHERE dt.doctor_id = doctors.id AND dt.tag ILIKE ?))",
			pattern, pattern, pattern, pattern,
		)
	}
	if len(search.Specialities) > 0 {
		query = query.Where(
			"EXISTS (SELECT 1 FROM doctor_specialities ds JOIN specialities s ON s.id = ds.speciality_id WHERE ds.doctor_id = doctors.id AND s.slug IN ?)",
			search.Specialities,
		)
	}
	if len(search.Tags) > 0 {
		query = query.Where(
			"EXISTS (SELECT 1 FROM doctor_tags dt WHERE dt.doctor_id = doctors.id AND dt.tag IN ?)",
			search.Tags,
		)
	}
	if search.City != "" {
		query = query.Where("LOWER(city) = LOWER(?)", search.City)
	}
	if search.Mode != "" {
		query = query.Where(
			"EXISTS (SELECT 1 FROM doctor_consultation_modes dm WHERE dm.doctor_id = doctors.id AND dm.mode = ?)",
			string(search.Mode),
		)
	}
	if search.FeeMax > 0 {
		query = query.Where("consultation_fee <= ?", search.FeeMax)
	}

	if err := query.
		Order("id ASC").
		Limit(search.Limit).
		Offset(search.Offset).
		Find(&records).Error; err != nil {
		return nil, fmt.Errorf("search doctors: %w", err)
	}

	return repo.withLinksMany(ctx, records)
}

func (repo *DoctorRepository) replaceLinks(ctx context.Context, doctorID uint64, doctor *models.Doctor) error {
	db := dbFromContext(ctx, repo.db)

	if err := db.Where("doctor_id = ?", doctorID).Delete(&doctorSpecialityRecord{}).Error; err != nil {
		return fmt.Errorf("delete doctor specialities: %w", err)
	}
	if err := db.Where("doctor_id = ?", doctorID).Delete(&doctorTagRecord{}).Error; err != nil {
		return fmt.Errorf("delete doctor tags: %w", err)
	}
	if err := db.Where("doctor_id = ?", doctorID).Delete(&doctorConsultationModeRecord{}).Error; err != nil {
		return fmt.Errorf("delete doctor consultation modes: %w", err)
	}

	if len(doctor.Specialities) > 0 {
		specialityRecords := make([]doctorSpecialityRecord, 0, len(doctor.Specialities))
		for _, speciality := range doctor.Specialities {
			specialityRecords = append(specialityRecords, doctorSpecialityRecord{
				DoctorID:     doctorID,
				SpecialityID: speciality.ID,
				IsPrimary:    speciality.IsPrimary,
			})
		}
		if err := db.Create(&specialityRecords).Error; err != nil {
			return fmt.Errorf("create doctor specialities: %w", err)
		}
	}

	if len(doctor.Tags) > 0 {
		tagRecords := make([]doctorTagRecord, 0, len(doctor.Tags))
		for _, tag := range doctor.Tags {
			tagRecords = append(tagRecords, doctorTagRecord{
				DoctorID: doctorID,
				Tag:      tag.Tag,
				TagType:  string(tag.TagType),
			})
		}
		if err := db.Create(&tagRecords).Error; err != nil {
			return fmt.Errorf("create doctor tags: %w", err)
		}
	}

	if len(doctor.ConsultationModes) > 0 {
		modeRecords := make([]doctorConsultationModeRecord, 0, len(doctor.ConsultationModes))
		for _, mode := range doctor.ConsultationModes {
			modeRecords = append(modeRecords, doctorConsultationModeRecord{
				DoctorID: doctorID,
				Mode:     string(mode),
			})
		}
		if err := db.Create(&modeRecords).Error; err != nil {
			return fmt.Errorf("create doctor consultation modes: %w", err)
		}
	}

	return nil
}

func (repo *DoctorRepository) withLinks(ctx context.Context, record doctorRecord) (*models.Doctor, error) {
	doctors, err := repo.withLinksMany(ctx, []doctorRecord{record})
	if err != nil {
		return nil, err
	}

	return doctors[0], nil
}

func (repo *DoctorRepository) withLinksMany(ctx context.Context, records []doctorRecord) ([]*models.Doctor, error) {
	result := make([]*models.Doctor, 0, len(records))
	if len(records) == 0 {
		return result, nil
	}

	ids := make([]uint64, 0, len(records))
	byID := make(map[uint64]*models.Doctor, len(records))
	for _, record := range records {
		doctor := toDoctorModel(record)
		ids = append(ids, record.ID)
		byID[record.ID] = doctor
		result = append(result, doctor)
	}

	db := dbFromContext(ctx, repo.db)

	var specialityRows []doctorSpecialityRow
	if err := db.
		Table("doctor_specialities ds").
		Select("ds.doctor_id, ds.is_primary, s.id, s.slug, s.name, s.created_at").
		Joins("JOIN specialities s ON s.id = ds.speciality_id").
		Where("ds.doctor_id IN ?", ids).
		Order("ds.is_primary DESC, s.name ASC").
		Scan(&specialityRows).Error; err != nil {
		return nil, fmt.Errorf("list doctor specialities: %w", err)
	}
	for _, row := range specialityRows {
		doctor := byID[row.DoctorID]
		doctor.Specialities = append(doctor.Specialities, models.Speciality{
			ID:        row.ID,
			Slug:      row.Slug,
			Name:      row.Name,
			IsPrimary: row.IsPrimary,
			CreatedAt: row.CreatedAt,
		})
	}

	var tagRecords []doctorTagRecord
	if err := db.
		Where("doctor_id IN ?", ids).
		Order("tag ASC").
		Find(&tagRecords).Error; err != nil {
		return nil, fmt.Errorf("list doctor tags: %w", err)
	}
	for _, record := range tagRecords {
		doctor := byID[record.DoctorID]
		doctor.Tags = append(doctor.Tags, models.DoctorTag{
			Tag:     record.Tag,
			TagType: models.TagType(record.TagType),
		})
	}

	var modeRecords []doctorConsultationModeRecord
	if err := db.
		Where("doctor_id IN ?", ids).
		Order("mode ASC").
		Find(&modeRecords).Error; err != nil {
		return nil, fmt.Errorf("list doctor consultation modes: %w", err)
	}
	for _, record := range modeRecords {
		doctor := byID[record.DoctorID]
		doctor.ConsultationModes = append(doctor.ConsultationModes, models.ConsultationMode(record.Mode))
	}

	return result, nil
}

func toDoctorRecord(doctor *models.Doctor) doctorRecord {
	var registrationYear *int
	if doctor.RegistrationYear != 0 {
		year := doctor.RegistrationYear
		registrationYear = &year
	}

	return doctorRecord{
		ID:                  doctor.ID,
		UserID:              doctor.UserID,
		FullName:            doctor.FullName,
		RegistrationNumber:  doctor.RegistrationNumber,
		RegistrationCouncil: doctor.RegistrationCouncil,
		RegistrationYear:    registrationYear,
		Qualification:       doctor.Qualification,
		YearsOfExperience:   doctor.YearsOfExperience,
		Bio:                 doctor.Bio,
		ConsultationFee:     doctor.ConsultationFee,
		SlotDurationMin:     doctor.SlotDurationMin,
		ClinicName:          doctor.ClinicName,
		ClinicAddress:       doctor.ClinicAddress,
		City:                doctor.City,
		Pincode:             doctor.Pincode,
		VerificationStatus:  string(doctor.VerificationStatus),
		VerifiedAt:          doctor.VerifiedAt,
		IsAcceptingPatients: doctor.IsAcceptingPatients,
	}
}

func toDoctorModel(record doctorRecord) *models.Doctor {
	registrationYear := 0
	if record.RegistrationYear != nil {
		registrationYear = *record.RegistrationYear
	}

	return &models.Doctor{
		ID:                  record.ID,
		UserID:              record.UserID,
		FullName:            record.FullName,
		RegistrationNumber:  record.RegistrationNumber,
		RegistrationCouncil: record.RegistrationCouncil,
		RegistrationYear:    registrationYear,
		Qualification:       record.Qualification,
		YearsOfExperience:   record.YearsOfExperience,
		Bio:                 record.Bio,
		ConsultationFee:     record.ConsultationFee,
		SlotDurationMin:     record.SlotDurationMin,
		ClinicName:          record.ClinicName,
		ClinicAddress:       record.ClinicAddress,
		City:                record.City,
		Pincode:             record.Pincode,
		VerificationStatus:  models.VerificationStatus(record.VerificationStatus),
		VerifiedAt:          record.VerifiedAt,
		IsAcceptingPatients: record.IsAcceptingPatients,
		UPIID:               record.UPIID,
		UPIQRStorageKey:     record.UPIQRStorageKey,
		Specialities:        []models.Speciality{},
		Tags:                []models.DoctorTag{},
		ConsultationModes:   []models.ConsultationMode{},
		CreatedAt:           record.CreatedAt,
		UpdatedAt:           record.UpdatedAt,
	}
}
