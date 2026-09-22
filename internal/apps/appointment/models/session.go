package models

import "time"

type Session struct {
	ID        uint64
	UserID    uint64
	TokenHash string
	Expiry    time.Time
	IsRevoked bool
	CreatedAt time.Time
	UpdatedAt time.Time
}
