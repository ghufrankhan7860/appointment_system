package services

import (
	"appointment_system/internal/apps/appointment/models"
	"context"
)

type MedicalRecordRepository interface {
	Create(ctx context.Context, record *models.MedicalRecord) (*models.MedicalRecord, error)
	GetByID(ctx context.Context, id uint64) (*models.MedicalRecord, error)
	GetByAppointment(ctx context.Context, appointmentID uint64) (*models.MedicalRecord, error)
	UpdateDraft(ctx context.Context, record *models.MedicalRecord) error
	Finalize(ctx context.Context, id uint64) error
	ListFinalizedByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.MedicalRecord, error)
	ListForDoctorView(ctx context.Context, patientID uint64, doctorID uint64, limit int, offset int) ([]*models.MedicalRecord, error)
	CreateAttachment(ctx context.Context, attachment *models.MedicalAttachment) (*models.MedicalAttachment, error)
	GetAttachment(ctx context.Context, id uint64) (*models.MedicalAttachment, error)
	ListAttachmentsByPatient(ctx context.Context, patientID uint64, limit int, offset int) ([]*models.MedicalAttachment, error)
	ListAttachmentsByRecord(ctx context.Context, recordID uint64) ([]*models.MedicalAttachment, error)
	LogAccess(ctx context.Context, access *models.RecordAccess) error
	ListAccessLog(ctx context.Context, recordID uint64, limit int, offset int) ([]*models.RecordAccess, error)
}
