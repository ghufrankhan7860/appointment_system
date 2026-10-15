package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"appointment_system/internal/apps/appointment/apperrors"
)

func TestWritePatientError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		err  error
		want int
	}{
		{fmt.Errorf("wrap: %w", apperrors.ErrInvalidPatientInput), http.StatusBadRequest},
		{apperrors.ErrNilPatientRequest, http.StatusBadRequest},
		{apperrors.ErrPatientForbidden, http.StatusForbidden},
		{apperrors.ErrPatientNotFound, http.StatusNotFound},
		{apperrors.ErrUserPatientAlreadyExists, http.StatusConflict},
		{fmt.Errorf("db down"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		writePatientError(ctx, tt.err, "failed")
		if rec.Code != tt.want {
			t.Errorf("%v: status = %d, want %d", tt.err, rec.Code, tt.want)
		}
	}
}

func TestNewPatientHandlerRejectsNil(t *testing.T) {
	if _, err := NewPatientHandler(nil, func(string) (uint64, error) { return 0, nil }); err == nil {
		t.Fatal("expected error for nil controller")
	}
}
