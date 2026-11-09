package models

import "time"

type Relationship string

const (
	RelationshipSelf      Relationship = "self"
	RelationshipFather    Relationship = "father"
	RelationshipMother    Relationship = "mother"
	RelationshipSpouse    Relationship = "spouse"
	RelationshipChild     Relationship = "child"
	RelationshipSibling   Relationship = "sibling"
	RelationshipGuardian  Relationship = "guardian"
	RelationshipCaregiver Relationship = "caregiver"
	RelationshipOther     Relationship = "other"
)

type AccessLevel string

const (
	AccessLevelFull             AccessLevel = "full"
	AccessLevelAppointmentsOnly AccessLevel = "appointments_only"
	AccessLevelReadOnly         AccessLevel = "read_only"
)

type UserPatientStatus string

const (
	UserPatientStatusPending UserPatientStatus = "pending"
	UserPatientStatusActive  UserPatientStatus = "active"
	UserPatientStatusRevoked UserPatientStatus = "revoked"
)

type UserPatient struct {
	UserID              uint64
	PatientID           uint64
	PatientRelationship Relationship
	AccessLevel         AccessLevel
	Status              UserPatientStatus
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type UserPatientChangeReason string

const (
	UserPatientChangeCreated         UserPatientChangeReason = "created"
	UserPatientChangeInviteRedeemed  UserPatientChangeReason = "invite_redeemed"
	UserPatientChangeAccessChanged   UserPatientChangeReason = "access_changed"
	UserPatientChangeRevokedByHolder UserPatientChangeReason = "revoked_by_holder"
	UserPatientChangeLeft            UserPatientChangeReason = "left"
)

// UserPatientHistory is one change to a user-patient link. From fields are
// empty when the link did not exist before.
type UserPatientHistory struct {
	ID              uint64
	UserID          uint64
	PatientID       uint64
	ChangedByUserID uint64
	FromStatus      UserPatientStatus
	ToStatus        UserPatientStatus
	FromAccessLevel AccessLevel
	ToAccessLevel   AccessLevel
	Reason          UserPatientChangeReason
	CreatedAt       time.Time
}

// LinkedPatient is a patient together with the acting user's link to it.
type LinkedPatient struct {
	Patient Patient
	Link    UserPatient
}

type PatientMember struct {
	Link     UserPatient
	Name     string
	MobileNo string
}
