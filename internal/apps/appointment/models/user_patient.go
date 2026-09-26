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
