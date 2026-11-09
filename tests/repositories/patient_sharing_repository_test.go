package repositories_test

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/repositories"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestPatientSharingRepositories(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	users, err := repositories.NewUserRepository(db)
	if err != nil {
		t.Fatalf("NewUserRepository returned error: %v", err)
	}
	patients, err := repositories.NewPatientRepository(db)
	if err != nil {
		t.Fatalf("NewPatientRepository returned error: %v", err)
	}
	links, err := repositories.NewUserPatientRepository(db)
	if err != nil {
		t.Fatalf("NewUserPatientRepository returned error: %v", err)
	}
	invites, err := repositories.NewPatientInviteRepository(db)
	if err != nil {
		t.Fatalf("NewPatientInviteRepository returned error: %v", err)
	}
	history, err := repositories.NewUserPatientHistoryRepository(db)
	if err != nil {
		t.Fatalf("NewUserPatientHistoryRepository returned error: %v", err)
	}
	attempts, err := repositories.NewPatientInviteAttemptRepository(db)
	if err != nil {
		t.Fatalf("NewPatientInviteAttemptRepository returned error: %v", err)
	}
	txManager, err := repositories.NewGORMTransactionManager(db)
	if err != nil {
		t.Fatalf("NewGORMTransactionManager returned error: %v", err)
	}

	owner, err := users.Create(ctx, newTestUser(t, db))
	if err != nil {
		t.Fatalf("Create owner returned error: %v", err)
	}
	other, err := users.Create(ctx, newTestUser(t, db))
	if err != nil {
		t.Fatalf("Create other returned error: %v", err)
	}
	patient, err := patients.Create(ctx, &models.Patient{
		Name:        "sharing test",
		Gender:      models.GenderFemale,
		DateOfBirth: time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC),
		BloodType:   models.BloodTypeOPlus,
	})
	if err != nil {
		t.Fatalf("Create patient returned error: %v", err)
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM patient_invite_attempts WHERE user_id IN (?, ?)", owner.ID, other.ID)
		db.Exec("DELETE FROM patient_invites WHERE patient_id = ?", patient.ID)
		db.Exec("DELETE FROM user_patient_history WHERE patient_id = ?", patient.ID)
		db.Exec("DELETE FROM user_patient WHERE patient_id = ?", patient.ID)
		db.Exec("DELETE FROM patients WHERE id = ?", patient.ID)
	})

	t.Run("one active self link per patient", func(t *testing.T) {
		if _, err := links.Create(ctx, &models.UserPatient{UserID: owner.ID, PatientID: patient.ID, PatientRelationship: models.RelationshipSelf, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive}); err != nil {
			t.Fatalf("Create self link returned error: %v", err)
		}
		_, err := links.Create(ctx, &models.UserPatient{UserID: other.ID, PatientID: patient.ID, PatientRelationship: models.RelationshipSelf, AccessLevel: models.AccessLevelFull, Status: models.UserPatientStatusActive})
		if !errors.Is(err, apperrors.ErrPatientSelfLinkExists) {
			t.Fatalf("Create second self link error = %v, want %v", err, apperrors.ErrPatientSelfLinkExists)
		}
		if _, err := links.Create(ctx, &models.UserPatient{UserID: other.ID, PatientID: patient.ID, PatientRelationship: models.RelationshipFather, AccessLevel: models.AccessLevelReadOnly, Status: models.UserPatientStatusActive}); err != nil {
			t.Fatalf("Create father link returned error: %v", err)
		}
	})

	t.Run("active links are listed and locked", func(t *testing.T) {
		err := txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
			locked, err := links.ListActiveByPatientForUpdate(txCtx, patient.ID)
			if err != nil {
				return err
			}
			if len(locked) != 2 {
				return fmt.Errorf("ListActiveByPatientForUpdate = %d links, want 2", len(locked))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("locked list: %v", err)
		}

		byID, err := users.GetByIDs(ctx, []uint64{owner.ID, other.ID})
		if err != nil {
			t.Fatalf("GetByIDs returned error: %v", err)
		}
		if len(byID) != 2 || byID[owner.ID].PasswordHash != nil {
			t.Fatalf("GetByIDs = %d users, want 2 without password hashes", len(byID))
		}
	})

	t.Run("history references a link", func(t *testing.T) {
		if err := history.Create(ctx, &models.UserPatientHistory{UserID: owner.ID, PatientID: patient.ID, ChangedByUserID: owner.ID, ToStatus: models.UserPatientStatusActive, ToAccessLevel: models.AccessLevelFull, Reason: models.UserPatientChangeCreated}); err != nil {
			t.Fatalf("Create history returned error: %v", err)
		}
		if err := history.Create(ctx, &models.UserPatientHistory{UserID: owner.ID + other.ID, PatientID: patient.ID, ChangedByUserID: owner.ID, ToStatus: models.UserPatientStatusActive, Reason: models.UserPatientChangeCreated}); err == nil {
			t.Fatalf("Create history for a missing link returned nil, want a foreign key error")
		}
	})

	t.Run("invite lifecycle", func(t *testing.T) {
		now := time.Now().UTC()
		newInvite := func(expiresAt time.Time) *models.PatientInvite {
			t.Helper()
			created, err := invites.Create(ctx, &models.PatientInvite{
				PatientID:           patient.ID,
				CreatedByUserID:     owner.ID,
				CodeHash:            fmt.Sprintf("hash-%d", time.Now().UnixNano()),
				PatientRelationship: models.RelationshipGuardian,
				AccessLevel:         models.AccessLevelAppointmentsOnly,
				Status:              models.PatientInviteStatusPending,
				ExpiresAt:           expiresAt,
			})
			if err != nil {
				t.Fatalf("Create invite returned error: %v", err)
			}
			return created
		}

		pending := newInvite(now.Add(time.Hour))
		newInvite(now.Add(-time.Hour))
		if count, err := invites.CountPendingByPatient(ctx, patient.ID, now); err != nil || count != 1 {
			t.Fatalf("CountPendingByPatient = %d, %v, want 1 unexpired", count, err)
		}
		if _, err := invites.Create(ctx, &models.PatientInvite{PatientID: patient.ID, CreatedByUserID: owner.ID, CodeHash: pending.CodeHash, PatientRelationship: models.RelationshipGuardian, AccessLevel: models.AccessLevelFull, Status: models.PatientInviteStatusPending, ExpiresAt: now.Add(time.Hour)}); !errors.Is(err, apperrors.ErrPatientInviteCodeExists) {
			t.Fatalf("Create with a used hash error = %v, want %v", err, apperrors.ErrPatientInviteCodeExists)
		}

		err := txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
			found, err := invites.GetByCodeHashForUpdate(txCtx, pending.CodeHash)
			if err != nil {
				return err
			}
			return invites.MarkRedeemed(txCtx, found.ID, other.ID, now)
		})
		if err != nil {
			t.Fatalf("redeem invite: %v", err)
		}
		if err := invites.Revoke(ctx, patient.ID, pending.ID); !errors.Is(err, apperrors.ErrPatientInviteNotFound) {
			t.Fatalf("Revoke a redeemed invite error = %v, want %v", err, apperrors.ErrPatientInviteNotFound)
		}
		if _, err := invites.GetByCodeHashForUpdate(ctx, "missing-hash"); !errors.Is(err, apperrors.ErrPatientInviteNotFound) {
			t.Fatalf("GetByCodeHashForUpdate missing error = %v, want %v", err, apperrors.ErrPatientInviteNotFound)
		}

		open := newInvite(now.Add(time.Hour))
		if err := invites.Revoke(ctx, patient.ID+1, open.ID); !errors.Is(err, apperrors.ErrPatientInviteNotFound) {
			t.Fatalf("Revoke through another patient error = %v, want %v", err, apperrors.ErrPatientInviteNotFound)
		}
		if err := invites.Revoke(ctx, patient.ID, open.ID); err != nil {
			t.Fatalf("Revoke returned error: %v", err)
		}
		if listed, err := invites.ListPendingByPatient(ctx, patient.ID, now); err != nil || len(listed) != 0 {
			t.Fatalf("ListPendingByPatient = %d, %v, want none left", len(listed), err)
		}
	})

	t.Run("attempts are counted within the window", func(t *testing.T) {
		now := time.Now().UTC()
		for _, at := range []time.Time{now, now.Add(-time.Minute), now.Add(-2 * time.Hour)} {
			if err := attempts.Create(ctx, other.ID, at); err != nil {
				t.Fatalf("Create attempt returned error: %v", err)
			}
		}
		if count, err := attempts.CountSince(ctx, other.ID, now.Add(-time.Hour)); err != nil || count != 2 {
			t.Fatalf("CountSince = %d, %v, want 2 in the last hour", count, err)
		}
		if err := attempts.DeleteBefore(ctx, other.ID, now.Add(-time.Hour)); err != nil {
			t.Fatalf("DeleteBefore returned error: %v", err)
		}
		if count, err := attempts.CountSince(ctx, other.ID, time.Time{}); err != nil || count != 2 {
			t.Fatalf("CountSince after cleanup = %d, %v, want 2", count, err)
		}
	})
}
