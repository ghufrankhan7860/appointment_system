package utils_test

import (
	"appointment_system/internal/apps/appointment/utils"
	"strings"
	"testing"
)

func TestNewCode(t *testing.T) {
	const alphabet = "ABC23"

	seen := map[string]bool{}
	for range 20 {
		code, err := utils.NewCode(alphabet, 12)
		if err != nil {
			t.Fatalf("NewCode returned error: %v", err)
		}
		if len(code) != 12 {
			t.Fatalf("NewCode length = %d, want 12", len(code))
		}
		for _, char := range code {
			if !strings.ContainsRune(alphabet, char) {
				t.Fatalf("NewCode = %q, want only characters from %q", code, alphabet)
			}
		}
		seen[code] = true
	}
	if len(seen) < 2 {
		t.Fatalf("NewCode returned the same code every time")
	}
}
