package utils

import (
	"strings"
	"testing"
	"time"

	"appointment_system/internal/apps/appointment/models"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestParseUserID(t *testing.T) {
	manager, err := NewJWTManager(testSecret, "issuer", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.Generate(models.User{ID: 42, Role: "patient"})
	if err != nil {
		t.Fatal(err)
	}

	if id, err := manager.ParseUserID(token); err != nil || id != 42 {
		t.Fatalf("ParseUserID = %d, %v; want 42, nil", id, err)
	}
	if _, err := manager.ParseUserID(token + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}

	other, _ := NewJWTManager(testSecret, "other-issuer", time.Minute)
	if _, err := other.ParseUserID(token); err == nil {
		t.Fatal("wrong issuer accepted")
	}

	expired, _ := NewJWTManager(testSecret, "issuer", time.Nanosecond)
	old, _ := expired.Generate(models.User{ID: 42})
	time.Sleep(5 * time.Millisecond)
	if _, err := expired.ParseUserID(old); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired token: err = %v", err)
	}
}
