package controller

import (
	"appointment_system/internal/apps/appointment/dtos/auth"
	"appointment_system/internal/apps/appointment/models"
)

func toSessionResponse(session *models.Session, currentSessionID uint64) auth.SessionResponse {
	return auth.SessionResponse{
		ID:             session.ID,
		Current:        currentSessionID != 0 && session.ID == currentSessionID,
		DeviceName:     session.DeviceName,
		DevicePlatform: string(session.DevicePlatform),
		AppVersion:     session.AppVersion,
		Browser:        session.Browser.BrowserName,
		BrowserVersion: session.Browser.BrowserVersion,
		OS:             session.Browser.OSName,
		OSVersion:      session.Browser.OSVersion,
		DeviceBrand:    session.Browser.DeviceBrand,
		DeviceModel:    session.Browser.DeviceModel,
		IPAddress:      session.IPAddress,
		LastSeenIP:     session.LastSeenIP,
		Language:       session.Language,
		Timezone:       session.Timezone,
		Screen:         session.Screen,
		CreatedAt:      session.CreatedAt,
		LastSeenAt:     session.LastSeenAt,
		ExpiresAt:      session.ExpiresAt,
	}
}

func toSessionClient(clientHeaders *auth.ClientHeaders, clientIP string) models.SessionClient {
	return models.SessionClient{
		IPAddress:      clientIP,
		UserAgent:      clientHeaders.UserAgent,
		AcceptLanguage: clientHeaders.AcceptLanguage,
		DeviceID:       clientHeaders.DeviceID,
		DeviceName:     clientHeaders.DeviceName,
		DevicePlatform: models.DevicePlatform(clientHeaders.DevicePlatform),
		AppVersion:     clientHeaders.AppVersion,
		Screen:         clientHeaders.Screen,
		Timezone:       clientHeaders.Timezone,
	}
}
