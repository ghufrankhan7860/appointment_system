package admin

import (
	"appointment_system/internal/apps/appointment/dtos/doctor"
	"time"
)

type GetDoctorRequest struct {
	ID uint64 `uri:"id" binding:"required"`
}

type VerificationResponse struct {
	ReviewedByUserID uint64    `json:"reviewedByUserId"`
	FromStatus       string    `json:"fromStatus"`
	ToStatus         string    `json:"toStatus"`
	Reason           string    `json:"reason"`
	CreatedAt        time.Time `json:"createdAt"`
}

type GetDoctorResponse struct {
	doctor.DoctorResponse
	UserID        uint64                 `json:"userId"`
	Verifications []VerificationResponse `json:"verifications,omitempty"`
}
