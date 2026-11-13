package models

import "time"

type VerificationStatus string

const (
	VerificationStatusPending   VerificationStatus = "pending"
	VerificationStatusVerified  VerificationStatus = "verified"
	VerificationStatusRejected  VerificationStatus = "rejected"
	VerificationStatusSuspended VerificationStatus = "suspended"
)

type ConsultationMode string

const (
	ConsultationModeInPerson ConsultationMode = "in_person"
	ConsultationModeVideo    ConsultationMode = "video"
	ConsultationModePhone    ConsultationMode = "phone"
)

type TagType string

const (
	TagTypeKeyword   TagType = "keyword"
	TagTypeCondition TagType = "condition"
	TagTypeProcedure TagType = "procedure"
	TagTypeLanguage  TagType = "language"
)

type DoctorTag struct {
	Tag     string
	TagType TagType
}

// Doctor is the professional profile of a user with role doctor. Only verified
// doctors that accept patients are visible to search.
type Doctor struct {
	ID                  uint64
	UserID              uint64
	FullName            string
	RegistrationNumber  string
	RegistrationCouncil string
	RegistrationYear    int
	Qualification       string
	YearsOfExperience   int
	Bio                 string
	ConsultationFee     int
	SlotDurationMin     int
	ClinicName          string
	ClinicAddress       string
	City                string
	Pincode             string
	VerificationStatus  VerificationStatus
	VerifiedAt          *time.Time
	IsAcceptingPatients bool
	// UPIID is where patients pay the doctor directly, when the upi method is on;
	// UPIQRStorageKey is their optional QR image.
	UPIID           string
	UPIQRStorageKey *string
	Specialities        []Speciality
	Tags                []DoctorTag
	ConsultationModes   []ConsultationMode
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type DoctorSearch struct {
	Query        string
	Specialities []string
	Tags         []string
	City         string
	Mode         ConsultationMode
	FeeMax       int
	// Date, when set, keeps only doctors with a free slot on that clinic date,
	// in Mode when one is given.
	Date   *time.Time
	Limit  int
	Offset int
}
