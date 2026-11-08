package auth

import "time"

// SessionResponse is one login on one device. Current marks the session the
// request was made with.
type SessionResponse struct {
	ID             uint64    `json:"id"`
	Current        bool      `json:"current"`
	DeviceName     string    `json:"deviceName"`
	DevicePlatform string    `json:"devicePlatform" enums:"web,android,ios" example:"web"`
	AppVersion     string    `json:"appVersion"`
	Browser        string    `json:"browser" example:"Chrome Mobile"`
	BrowserVersion string    `json:"browserVersion" example:"130.0"`
	OS             string    `json:"os" example:"Android"`
	OSVersion      string    `json:"osVersion" example:"14"`
	DeviceBrand    string    `json:"deviceBrand" example:"Google"`
	DeviceModel    string    `json:"deviceModel" example:"Pixel 8"`
	IPAddress      string    `json:"ipAddress"`
	LastSeenIP     string    `json:"lastSeenIp"`
	Language       string    `json:"language" example:"en-IN"`
	Timezone       string    `json:"timezone" example:"Asia/Kolkata"`
	Screen         string    `json:"screen" example:"1080x2400"`
	CreatedAt      time.Time `json:"createdAt"`
	LastSeenAt     time.Time `json:"lastSeenAt"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type ListSessionsResponse struct {
	Sessions []SessionResponse `json:"sessions"`
}
