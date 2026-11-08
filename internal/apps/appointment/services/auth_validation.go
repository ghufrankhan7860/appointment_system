package services

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
)

var screenSizePattern = regexp.MustCompile(`^[0-9]{2,5}x[0-9]{2,5}$`)

// validateSessionClient trims what a login or refresh request says about the
// client and rejects device headers the app sent wrong. Headers the browser
// sends on its own are never refused: a long User-Agent is cut and an
// unreadable Accept-Language is dropped.
func validateSessionClient(client models.SessionClient) (models.SessionClient, error) {
	client.IPAddress = strings.TrimSpace(client.IPAddress)
	client.UserAgent = truncate(strings.TrimSpace(client.UserAgent), constant.MaxUserAgentLength)
	client.AcceptLanguage = firstLanguageTag(client.AcceptLanguage)
	client.DeviceID = strings.TrimSpace(client.DeviceID)
	client.DeviceName = strings.TrimSpace(client.DeviceName)
	client.DevicePlatform = models.DevicePlatform(strings.ToLower(strings.TrimSpace(string(client.DevicePlatform))))
	client.AppVersion = strings.TrimSpace(client.AppVersion)
	client.Screen = strings.TrimSpace(client.Screen)
	client.Timezone = strings.TrimSpace(client.Timezone)

	if utf8.RuneCountInString(client.DeviceID) > constant.MaxDeviceIDLength {
		return models.SessionClient{}, apperrors.Invalid(apperrors.ErrInvalidSessionInput, "X-Device-Id", fmt.Sprintf("must be at most %d characters", constant.MaxDeviceIDLength))
	}
	if utf8.RuneCountInString(client.DeviceName) > constant.MaxDeviceNameLength {
		return models.SessionClient{}, apperrors.Invalid(apperrors.ErrInvalidSessionInput, "X-Device-Name", fmt.Sprintf("must be at most %d characters", constant.MaxDeviceNameLength))
	}
	if client.DevicePlatform != "" && !validDevicePlatform(client.DevicePlatform) {
		return models.SessionClient{}, apperrors.Invalid(apperrors.ErrInvalidSessionInput, "X-Device-Platform", "must be one of web, android, ios")
	}
	if utf8.RuneCountInString(client.AppVersion) > constant.MaxAppVersionLength {
		return models.SessionClient{}, apperrors.Invalid(apperrors.ErrInvalidSessionInput, "X-App-Version", fmt.Sprintf("must be at most %d characters", constant.MaxAppVersionLength))
	}
	if client.Screen != "" && !screenSizePattern.MatchString(client.Screen) {
		return models.SessionClient{}, apperrors.Invalid(apperrors.ErrInvalidSessionInput, "X-Device-Screen", "must be width x height in pixels, such as 1080x2400")
	}
	if client.Timezone != "" && !validTimezone(client.Timezone) {
		return models.SessionClient{}, apperrors.Invalid(apperrors.ErrInvalidSessionInput, "X-Device-Timezone", "must be an IANA time zone, such as Asia/Kolkata")
	}

	return client, nil
}

func validDevicePlatform(platform models.DevicePlatform) bool {
	switch platform {
	case models.DevicePlatformWeb, models.DevicePlatformAndroid, models.DevicePlatformIOS:
		return true
	default:
		return false
	}
}

// validTimezone accepts IANA names only; "Local" would mean the server's zone.
func validTimezone(name string) bool {
	if name == "Local" {
		return false
	}
	_, err := time.LoadLocation(name)

	return err == nil
}

// firstLanguageTag returns the first language of an Accept-Language header
// ("en-IN,en;q=0.9" gives "en-IN"), or "" when it is a wildcard or too long.
func firstLanguageTag(acceptLanguage string) string {
	tag, _, _ := strings.Cut(acceptLanguage, ",")
	tag, _, _ = strings.Cut(tag, ";")
	tag = strings.TrimSpace(tag)
	if tag == "*" || len(tag) > constant.MaxLanguageLength {
		return ""
	}

	return tag
}

func truncate(value string, maxLength int) string {
	if utf8.RuneCountInString(value) <= maxLength {
		return value
	}

	return string([]rune(value)[:maxLength])
}
