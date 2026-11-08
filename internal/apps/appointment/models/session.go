package models

import "time"

type DevicePlatform string

const (
	DevicePlatformWeb     DevicePlatform = "web"
	DevicePlatformAndroid DevicePlatform = "android"
	DevicePlatformIOS     DevicePlatform = "ios"
)

type SessionStatus string

const (
	SessionStatusActive  SessionStatus = "active"
	SessionStatusRevoked SessionStatus = "revoked"
	SessionStatusExpired SessionStatus = "expired"
)

// Session is one login on one device. Refreshing rotates TokenHash in place, so
// the row lives until logout, revocation or expiry. IPAddress and DeviceID are
// recorded at login and never change; the LastSeen fields follow each refresh.
type Session struct {
	ID             uint64
	UserID         uint64
	TokenHash      string
	ExpiresAt      time.Time
	RevokedAt      *time.Time
	IPAddress      string
	LastSeenIP     string
	LastSeenAt     time.Time
	UserAgent      string
	Browser        UserAgentDetails
	DeviceID       string
	DeviceName     string
	DevicePlatform DevicePlatform
	AppVersion     string
	Screen         string
	Timezone       string
	Language       string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UserAgentDetails is what the server reads out of a User-Agent header.
type UserAgentDetails struct {
	BrowserName    string
	BrowserVersion string
	OSName         string
	OSVersion      string
	DeviceBrand    string
	DeviceModel    string
}

// SessionClient is what one login or refresh request says about the client:
// the IP and the User-Agent and Accept-Language headers the server reads, and the
// optional device headers the app sends.
type SessionClient struct {
	IPAddress      string
	UserAgent      string
	AcceptLanguage string
	DeviceID       string
	DeviceName     string
	DevicePlatform DevicePlatform
	AppVersion     string
	Screen         string
	Timezone       string
}
