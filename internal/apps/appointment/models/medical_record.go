package models

import "time"

type MedicalRecordStatus string

const (
	MedicalRecordStatusDraft     MedicalRecordStatus = "draft"
	MedicalRecordStatusFinalized MedicalRecordStatus = "finalized"
)

type MedicineTiming string

const (
	MedicineTimingBeforeFood MedicineTiming = "before_food"
	MedicineTimingAfterFood  MedicineTiming = "after_food"
	MedicineTimingWithFood   MedicineTiming = "with_food"
	MedicineTimingAny        MedicineTiming = "any"
)

type AttachmentKind string

const (
	AttachmentKindLabReport        AttachmentKind = "lab_report"
	AttachmentKindImaging          AttachmentKind = "imaging"
	AttachmentKindPrescriptionScan AttachmentKind = "prescription_scan"
	AttachmentKindReferral         AttachmentKind = "referral"
	AttachmentKindOther            AttachmentKind = "other"
)

type RecordAccessAction string

const (
	RecordAccessCreate   RecordAccessAction = "create"
	RecordAccessUpdate   RecordAccessAction = "update"
	RecordAccessFinalize RecordAccessAction = "finalize"
	RecordAccessView     RecordAccessAction = "view"
	RecordAccessList     RecordAccessAction = "list"
	RecordAccessUpload   RecordAccessAction = "upload"
	RecordAccessDownload RecordAccessAction = "download"
)

// Vitals are the measurements taken at a visit. A nil value was not measured.
type Vitals struct {
	BloodPressure         string
	PulsePerMinute        *int
	TemperatureFahrenheit *float64
	WeightKg              *float64
	SpO2Percent           *int
}

// PrescriptionItem is one medicine on a visit's prescription.
type PrescriptionItem struct {
	MedicineName string
	Strength     string
	Form         string
	Dosage       string
	Frequency    string
	DurationDays int
	Timing       MedicineTiming
	Instructions string
}

// MedicalRecord is the doctor's clinical note for one appointment, with the
// visit's prescription. A draft is edited by the appointment's doctor; a
// finalized record never changes and is what the patient side can read.
type MedicalRecord struct {
	ID                uint64
	AppointmentID     uint64
	PatientID         uint64
	DoctorID          uint64
	CreatedByUserID   uint64
	ChiefComplaint    string
	Symptoms          string
	Diagnosis         string
	ExaminationNotes  string
	Advice            string
	PrescriptionNotes string
	FollowUpOn        *time.Time
	Vitals            Vitals
	Medicines         []PrescriptionItem
	Status            MedicalRecordStatus
	FinalizedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// MedicalAttachment is an uploaded report or scan. The file lives in file
// storage under StorageKey; the database never holds the file or a public URL.
// MedicalRecordID is nil for an upload made outside a visit.
type MedicalAttachment struct {
	ID               uint64
	PatientID        uint64
	MedicalRecordID  *uint64
	UploadedByUserID uint64
	Kind             AttachmentKind
	Title            string
	StorageKey       string
	ContentType      string
	SizeBytes        int64
	TakenOn          *time.Time
	CreatedAt        time.Time
}

// RecordAccess is one row of the medical record access log: who touched which
// patient's clinical data, how, and in which request.
type RecordAccess struct {
	ID               uint64
	PatientID        uint64
	MedicalRecordID  *uint64
	AttachmentID     *uint64
	AccessedByUserID uint64
	Action           RecordAccessAction
	CorrelationID    string
	CreatedAt        time.Time
}
