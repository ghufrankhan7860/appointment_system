package utils_test

import (
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
	"testing"
	"time"
)

const testJWTSecret = "0123456789abcdef0123456789abcdef"

func TestJWTManagerVerify(t *testing.T) {
	manager, err := utils.NewJWTManager(testJWTSecret, "issuer", time.Minute)
	if err != nil {
		t.Fatalf("NewJWTManager returned error: %v", err)
	}
	otherIssuer, err := utils.NewJWTManager(testJWTSecret, "other-issuer", time.Minute)
	if err != nil {
		t.Fatalf("NewJWTManager returned error: %v", err)
	}
	expiring, err := utils.NewJWTManager(testJWTSecret, "issuer", time.Nanosecond)
	if err != nil {
		t.Fatalf("NewJWTManager returned error: %v", err)
	}

	user := models.User{ID: 42, Role: models.RolePatient}
	validToken, err := manager.Generate(user)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	otherIssuerToken, err := otherIssuer.Generate(user)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	expiredToken, err := expiring.Generate(user)
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	tests := []struct {
		name     string
		token    string
		wantID   uint64
		wantRole string
		wantErr  bool
	}{
		{name: "valid token", token: validToken, wantID: 42, wantRole: "patient"},
		{name: "tampered token", token: validToken + "x", wantErr: true},
		{name: "other issuer token", token: otherIssuerToken, wantErr: true},
		{name: "expired token", token: expiredToken, wantErr: true},
		{name: "empty token", token: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID, role, err := manager.Verify(tt.token)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Verify error = %v, wantErr %v", err, tt.wantErr)
			}
			if userID != tt.wantID {
				t.Fatalf("Verify user id = %d, want %d", userID, tt.wantID)
			}
			if role != tt.wantRole {
				t.Fatalf("Verify role = %q, want %q", role, tt.wantRole)
			}
		})
	}
}
