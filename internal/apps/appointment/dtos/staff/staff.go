package staff

import (
	"time"

	"appointment_system/internal/apps/appointment/dtos/doctor"
)

// StaffMemberResponse is a staff member working for the doctor.
type StaffMemberResponse struct {
	UserID   uint64    `json:"userId"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	MobileNo string    `json:"mobileNo"`
	JoinedAt time.Time `json:"joinedAt"`
}

type ListStaffResponse struct {
	Staff []StaffMemberResponse `json:"staff"`
}

type RemoveStaffRequest struct {
	UserID uint64 `uri:"userId" binding:"required"`
}

// GetStaffMeResponse is the doctor the staff account works for.
type GetStaffMeResponse struct {
	JoinedAt time.Time             `json:"joinedAt"`
	Doctor   doctor.DoctorResponse `json:"doctor"`
}

// JoinStaffRequest links a staff account that works for no doctor now to the
// invite's doctor.
type JoinStaffRequest struct {
	InviteCode string `json:"inviteCode" binding:"required" example:"K7Q2-M9XD"`
}
