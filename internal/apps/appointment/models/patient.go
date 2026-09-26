package models

import "time"

type BloodType string

const (
	BloodTypeAPlus   BloodType = "A+"
	BloodTypeAMinus  BloodType = "A-"
	BloodTypeBPlus   BloodType = "B+"
	BloodTypeBMinus  BloodType = "B-"
	BloodTypeABPlus  BloodType = "AB+"
	BloodTypeABMinus BloodType = "AB-"
	BloodTypeOPlus   BloodType = "O+"
	BloodTypeOMinus  BloodType = "O-"
)

type Gender string

const (
	GenderMale    Gender = "Male"
	GenderFemale  Gender = "Female"
	GenderOther   Gender = "Other"
	GenderUnknown Gender = "Unknown"
)

type Patient struct {
	ID               uint64
	Name             string
	Gender           Gender
	DateOfBirth      time.Time
	BloodType        BloodType
	EmergencyContact string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
