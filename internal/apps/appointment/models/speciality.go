package models

import "time"

// Speciality is a controlled vocabulary entry. IsPrimary is only meaningful when
// the speciality is loaded as part of a doctor profile.
type Speciality struct {
	ID        uint64
	Slug      string
	Name      string
	IsPrimary bool
	CreatedAt time.Time
}
