package models

import "time"

type Role string

const (
	RolePatient Role = "patient"
	RoleDoctor  Role = "doctor"
	RoleAdmin   Role = "admin"
	// RoleStaff is a doctor's compounder or receptionist: books and manages
	// that one doctor's appointments, never their medical records.
	RoleStaff Role = "staff"
)

type User struct {
	ID           uint64
	Name         string
	Email        string
	PasswordHash []byte
	MobileNo     string
	Role         Role
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
