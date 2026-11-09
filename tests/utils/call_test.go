package utils_test

import (
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
	"testing"
	"time"
)

func TestDammCheckDigit(t *testing.T) {
	check, err := utils.DammCheckDigit("572")
	if err != nil || check != '4' {
		t.Fatalf("DammCheckDigit(572) = %c, %v, want 4", check, err)
	}
	if _, err := utils.DammCheckDigit("57a"); err == nil {
		t.Fatalf("DammCheckDigit(57a) returned nil error, want one for a non-digit")
	}

	const digits = "482913"
	check, err = utils.DammCheckDigit(digits)
	if err != nil {
		t.Fatalf("DammCheckDigit returned error: %v", err)
	}
	for position := range len(digits) {
		for wrong := byte('0'); wrong <= '9'; wrong++ {
			if wrong == digits[position] {
				continue
			}
			typo := digits[:position] + string(wrong) + digits[position+1:]
			if got, _ := utils.DammCheckDigit(typo); got == check {
				t.Fatalf("DammCheckDigit(%s) = %c, the same as for %s", typo, got, digits)
			}
		}
	}
	for position := range len(digits) - 1 {
		if digits[position] == digits[position+1] {
			continue
		}
		swapped := digits[:position] + string(digits[position+1]) + string(digits[position]) + digits[position+2:]
		if got, _ := utils.DammCheckDigit(swapped); got == check {
			t.Fatalf("DammCheckDigit(%s) = %c, the same as for %s", swapped, got, digits)
		}
	}
}

func TestTURNCredential(t *testing.T) {
	username, credential := utils.TURNCredential("turn-secret", "peer1", time.Unix(1700000000, 0))
	if username != "1700000000:peer1" || credential != "lwzvX873IoRUMSdVQ0NJs+Inc00=" {
		t.Fatalf("TURNCredential = %q/%q, want 1700000000:peer1 with its HMAC-SHA1", username, credential)
	}
}

func TestCallTokenManager(t *testing.T) {
	calls, err := utils.NewCallTokenManager(testJWTSecret, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("NewCallTokenManager returned error: %v", err)
	}
	access, err := utils.NewJWTManager(testJWTSecret, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("NewJWTManager returned error: %v", err)
	}

	claims := models.CallTokenClaims{RoomID: 3, ParticipantID: 8, PeerID: "a1b2c3", Role: models.CallRoleGuest}
	callToken, expiresAt, err := calls.Generate(claims)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if time.Until(expiresAt) <= 0 || time.Until(expiresAt) > time.Minute {
		t.Fatalf("Generate expiry = %s, want within a minute", expiresAt)
	}
	verified, err := calls.Verify(callToken)
	if err != nil || verified != claims {
		t.Fatalf("Verify = %+v, %v, want %+v", verified, err, claims)
	}

	if _, _, _, err := access.Verify(callToken); err == nil {
		t.Fatalf("access Verify accepted a call token")
	}
	accessToken, err := access.Generate(models.User{ID: 42, Role: models.RolePatient}, 9)
	if err != nil {
		t.Fatalf("Generate access token returned error: %v", err)
	}
	if _, err := calls.Verify(accessToken); err == nil {
		t.Fatalf("call Verify accepted an access token")
	}
	if _, err := utils.NewCallTokenManager("short", "issuer", time.Minute); err == nil {
		t.Fatalf("NewCallTokenManager accepted a short secret")
	}
}
