package models

import "time"

type Role string

const (
	RoleUser   Role = "patient"
	RoleDoctor Role = "doctor"
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
