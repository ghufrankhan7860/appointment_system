package services

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// normalizeMediaMode defaults an empty mode to audio, the cheaper start.
func normalizeMediaMode(mode models.CallMediaMode) (models.CallMediaMode, error) {
	switch mode {
	case "":
		return models.CallMediaAudio, nil
	case models.CallMediaAudio, models.CallMediaVideo:
		return mode, nil
	default:
		return "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "mediaMode", "must be audio or video")
	}
}

func normalizeDisplayName(name string, required bool) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" && required {
		return "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "displayName", "is required")
	}
	if utf8.RuneCountInString(name) > constant.MaxCallDisplayNameLength {
		return "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "displayName", fmt.Sprintf("must be at most %d characters", constant.MaxCallDisplayNameLength))
	}

	return name, nil
}

// normalizeJoinCode drops the spaces and dashes a code is written with and
// checks its shape and check digit, so a typo is answered without counting as
// a wrong code.
func normalizeJoinCode(code string) (string, error) {
	code = strings.NewReplacer(" ", "", "-", "").Replace(code)
	if code == "" {
		return "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "code", "is required")
	}
	if len(code) != constant.CodeRoomCodeDigits+1 {
		return "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "code", fmt.Sprintf("must be %d digits", constant.CodeRoomCodeDigits+1))
	}
	check, err := utils.DammCheckDigit(code[:constant.CodeRoomCodeDigits])
	if err != nil {
		return "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "code", fmt.Sprintf("must be %d digits", constant.CodeRoomCodeDigits+1))
	}
	if check != code[constant.CodeRoomCodeDigits] {
		return "", apperrors.Invalid(apperrors.ErrInvalidCallInput, "code", "has a typo; check the digits")
	}

	return code, nil
}

func validConnectionState(state models.CallConnectionState) bool {
	switch state {
	case models.CallConnectionConnected, models.CallConnectionRelay, models.CallConnectionFailed:
		return true
	default:
		return false
	}
}
