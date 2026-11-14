package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type PatientPassRepository interface {
	Replace(ctx context.Context, pass *models.PatientPass) (*models.PatientPass, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*models.PatientPass, error)
	GetByCodeHash(ctx context.Context, codeHash string) (*models.PatientPass, error)
}
