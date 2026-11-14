package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/utils"
)

// PatientInviteService lets a full holder share a patient through a one-time
// code. Redeeming it adds a user-patient link for the redeemer and never
// creates a patient, so the patient's appointments and records follow the new
// link. Only the code's hash is stored, and wrong codes are capped per user.
type PatientInviteService struct {
	invites      PatientInviteRepository
	attempts     PatientInviteAttemptRepository
	userPatients UserPatientRepository
	history      UserPatientHistoryRepository
	patients     PatientRepository
	outbox       OutboxRepository
	tx           TransactionManager
	now          func() time.Time
	logger       Logger
}

func NewPatientInviteService(
	invites PatientInviteRepository,
	attempts PatientInviteAttemptRepository,
	userPatients UserPatientRepository,
	history UserPatientHistoryRepository,
	patients PatientRepository,
	outbox OutboxRepository,
	tx TransactionManager,
	now func() time.Time,
	logger Logger,
) (*PatientInviteService, error) {
	if invites == nil {
		return nil, apperrors.ErrNilPatientInviteRepository
	}
	if attempts == nil {
		return nil, apperrors.ErrNilPatientInviteAttemptRepository
	}
	if userPatients == nil {
		return nil, apperrors.ErrNilUserPatientRepository
	}
	if history == nil {
		return nil, apperrors.ErrNilUserPatientHistoryRepository
	}
	if patients == nil {
		return nil, apperrors.ErrNilPatientRepository
	}
	if outbox == nil {
		return nil, apperrors.ErrNilOutboxRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	if now == nil {
		return nil, apperrors.ErrNilClock
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}

	return &PatientInviteService{
		invites:      invites,
		attempts:     attempts,
		userPatients: userPatients,
		history:      history,
		patients:     patients,
		outbox:       outbox,
		tx:           tx,
		now:          now,
		logger:       logger,
	}, nil
}

// Create issues an invite for a patient the acting user holds with full
// access and returns it with its code, which is shown only this once. A zero
// expiresIn means constant.PatientInviteTTL. A self invite must grant full
// access and is refused while the patient already has a self link.
func (pis *PatientInviteService) Create(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
	relationship models.Relationship,
	accessLevel models.AccessLevel,
	expiresIn time.Duration,
) (*models.PatientInvite, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", fmt.Errorf("create patient invite context: %w", err)
	}
	if actingUserID == 0 || patientID == 0 {
		return nil, "", fmt.Errorf("create patient invite with missing ids: %w", apperrors.ErrInvalidPatientInviteInput)
	}
	if !validRelationship(relationship) {
		return nil, "", apperrors.Invalid(apperrors.ErrInvalidPatientInviteInput, "relationship", "must be one of self, father, mother, spouse, child, sibling, guardian, caregiver, other")
	}
	if !validAccessLevel(accessLevel) {
		return nil, "", apperrors.Invalid(apperrors.ErrInvalidPatientInviteInput, "accessLevel", "must be one of full, appointments_only, read_only")
	}
	if relationship == models.RelationshipSelf && accessLevel != models.AccessLevelFull {
		return nil, "", apperrors.Invalid(apperrors.ErrInvalidPatientInviteInput, "accessLevel", "must be full for relationship self")
	}
	if expiresIn == 0 {
		expiresIn = constant.PatientInviteTTL
	}
	if expiresIn < constant.PatientInviteMinTTL || expiresIn > constant.PatientInviteMaxTTL {
		return nil, "", apperrors.Invalid(apperrors.ErrInvalidPatientInviteInput, "expiresInHours", "must be between 1 and 168")
	}

	for try := 1; ; try++ {
		created, code, err := pis.create(ctx, actingUserID, patientID, relationship, accessLevel, expiresIn, func(links []*models.UserPatient) error {
			actor := findLink(links, actingUserID)
			if actor == nil {
				return apperrors.ErrPatientNotFound
			}
			if actor.AccessLevel != models.AccessLevelFull {
				return apperrors.ErrPatientForbidden
			}
			return nil
		})
		if errors.Is(err, apperrors.ErrPatientInviteCodeExists) && try < constant.MaxPatientInviteCodeTries {
			continue
		}
		if err != nil {
			return nil, "", err
		}

		pis.logger.InfoContext(ctx, "patient invite created", "patient_id", patientID, "invite_id", created.ID, "access_level", accessLevel)

		return created, code, nil
	}
}

// CreateUnclaimed issues a full-access invite for a patient nobody holds yet,
// a walk-in a clinic desk added, so the person (or whoever looks after them)
// can take the patient over in the app. relationship is the redeemer's. The
// caller has checked the desk may do it; a patient someone holds already is
// ErrPatientAlreadyClaimed.
func (pis *PatientInviteService) CreateUnclaimed(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
	relationship models.Relationship,
) (*models.PatientInvite, string, error) {
	if actingUserID == 0 || patientID == 0 {
		return nil, "", fmt.Errorf("create unclaimed invite with missing ids: %w", apperrors.ErrInvalidPatientInviteInput)
	}
	if !validRelationship(relationship) {
		return nil, "", apperrors.Invalid(apperrors.ErrInvalidPatientInviteInput, "relationship", "must be one of self, father, mother, spouse, child, sibling, guardian, caregiver, other")
	}

	for try := 1; ; try++ {
		created, code, err := pis.create(ctx, actingUserID, patientID, relationship, models.AccessLevelFull, constant.PatientInviteTTL, func(links []*models.UserPatient) error {
			if len(links) > 0 {
				return apperrors.ErrPatientAlreadyClaimed
			}
			return nil
		})
		if errors.Is(err, apperrors.ErrPatientInviteCodeExists) && try < constant.MaxPatientInviteCodeTries {
			continue
		}
		if err != nil {
			return nil, "", err
		}

		pis.logger.InfoContext(ctx, "unclaimed patient invite created", "patient_id", patientID, "invite_id", created.ID)

		return created, code, nil
	}
}

// List returns the patient's pending invites that have not expired, without
// their codes.
func (pis *PatientInviteService) List(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
) ([]*models.PatientInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list patient invites context: %w", err)
	}
	if actingUserID == 0 || patientID == 0 {
		return nil, fmt.Errorf("list patient invites with missing ids: %w", apperrors.ErrInvalidPatientInviteInput)
	}

	if err := pis.authorizeFull(ctx, actingUserID, patientID); err != nil {
		return nil, err
	}

	invites, err := pis.invites.ListPendingByPatient(ctx, patientID, pis.now())
	if err != nil {
		return nil, fmt.Errorf("list pending patient invites: %w", err)
	}

	return invites, nil
}

func (pis *PatientInviteService) Revoke(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
	inviteID uint64,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("revoke patient invite context: %w", err)
	}
	if actingUserID == 0 || patientID == 0 || inviteID == 0 {
		return fmt.Errorf("revoke patient invite with missing ids: %w", apperrors.ErrInvalidPatientInviteInput)
	}

	if err := pis.authorizeFull(ctx, actingUserID, patientID); err != nil {
		return err
	}

	if err := pis.invites.Revoke(ctx, patientID, inviteID); err != nil {
		if errors.Is(err, apperrors.ErrPatientInviteNotFound) {
			return err
		}
		return fmt.Errorf("revoke patient invite: %w", err)
	}

	pis.logger.InfoContext(ctx, "patient invite revoked", "patient_id", patientID, "invite_id", inviteID)

	return nil
}

// Redeem links the acting user to the invite's patient and returns the
// patient with the new link. An unknown, used, revoked or expired code is one
// ErrPatientInviteInvalid and counts as a failed attempt; after
// constant.MaxFailedPatientInviteAttempts within
// constant.PatientInviteAttemptWindow it answers ErrPatientInviteAttemptsExceeded
// until the window passes. A user whose earlier link was revoked gets that link
// back with the invite's relationship and access level.
func (pis *PatientInviteService) Redeem(
	ctx context.Context,
	actingUserID uint64,
	code string,
) (*models.LinkedPatient, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("redeem patient invite context: %w", err)
	}
	if actingUserID == 0 {
		return nil, fmt.Errorf("redeem patient invite without acting user: %w", apperrors.ErrInvalidPatientInviteInput)
	}
	code = normalizeInviteCode(code)
	if code == "" {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPatientInviteInput, "code", "is required")
	}

	now := pis.now()
	failures, err := pis.attempts.CountSince(ctx, actingUserID, now.Add(-constant.PatientInviteAttemptWindow))
	if err != nil {
		return nil, fmt.Errorf("count patient invite attempts: %w", err)
	}
	if failures >= constant.MaxFailedPatientInviteAttempts {
		return nil, apperrors.ErrPatientInviteAttemptsExceeded
	}

	var link *models.UserPatient
	var inviteID uint64
	var expiredInviteID uint64
	err = pis.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		invite, err := pis.invites.GetByCodeHashForUpdate(txCtx, utils.HashToken(code))
		if err != nil {
			if errors.Is(err, apperrors.ErrPatientInviteNotFound) {
				return apperrors.ErrPatientInviteInvalid
			}
			return fmt.Errorf("get patient invite: %w", err)
		}
		if invite.Status != models.PatientInviteStatusPending {
			return apperrors.ErrPatientInviteInvalid
		}
		if !now.Before(invite.ExpiresAt) {
			expiredInviteID = invite.ID
			return apperrors.ErrPatientInviteInvalid
		}
		inviteID = invite.ID

		existing, err := pis.userPatients.GetByUserAndPatient(txCtx, actingUserID, invite.PatientID)
		if err != nil && !errors.Is(err, apperrors.ErrUserPatientNotFound) {
			return fmt.Errorf("get user-patient link: %w", err)
		}
		if existing != nil && existing.Status == models.UserPatientStatusActive {
			return apperrors.ErrUserPatientAlreadyExists
		}

		next := &models.UserPatient{
			UserID:              actingUserID,
			PatientID:           invite.PatientID,
			PatientRelationship: invite.PatientRelationship,
			AccessLevel:         invite.AccessLevel,
			Status:              models.UserPatientStatusActive,
		}
		history := &models.UserPatientHistory{
			UserID:          actingUserID,
			PatientID:       invite.PatientID,
			ChangedByUserID: actingUserID,
			ToStatus:        models.UserPatientStatusActive,
			ToAccessLevel:   invite.AccessLevel,
			Reason:          models.UserPatientChangeInviteRedeemed,
		}
		if existing != nil {
			history.FromStatus = existing.Status
			history.FromAccessLevel = existing.AccessLevel
			link, err = pis.userPatients.Update(txCtx, next)
		} else {
			link, err = pis.userPatients.Create(txCtx, next)
		}
		if err != nil {
			if errors.Is(err, apperrors.ErrPatientSelfLinkExists) ||
				errors.Is(err, apperrors.ErrUserPatientAlreadyExists) {
				return err
			}
			return fmt.Errorf("link user to patient: %w", err)
		}

		if err := pis.invites.MarkRedeemed(txCtx, invite.ID, actingUserID, now); err != nil {
			return fmt.Errorf("mark patient invite redeemed: %w", err)
		}
		if err := pis.history.Create(txCtx, history); err != nil {
			return fmt.Errorf("create user-patient history: %w", err)
		}

		return pis.outbox.Create(txCtx, patientAccessEvent(models.OutboxEventPatientAccessGranted, link, actingUserID, models.UserPatientChangeInviteRedeemed))
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrPatientInviteInvalid) {
			if recordErr := pis.recordFailedAttempt(ctx, actingUserID, now, expiredInviteID); recordErr != nil {
				return nil, recordErr
			}
			return nil, err
		}
		if errors.Is(err, apperrors.ErrUserPatientAlreadyExists) ||
			errors.Is(err, apperrors.ErrPatientSelfLinkExists) {
			return nil, err
		}
		return nil, fmt.Errorf("redeem patient invite: %w", err)
	}

	patient, err := pis.patients.GetByID(ctx, link.PatientID)
	if err != nil {
		return nil, fmt.Errorf("get redeemed patient: %w", err)
	}

	pis.logger.InfoContext(ctx, "patient invite redeemed", "patient_id", link.PatientID, "invite_id", inviteID, "access_level", link.AccessLevel)

	return &models.LinkedPatient{Patient: *patient, Link: *link}, nil
}

func (pis *PatientInviteService) create(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
	relationship models.Relationship,
	accessLevel models.AccessLevel,
	expiresIn time.Duration,
	authorize func(links []*models.UserPatient) error,
) (*models.PatientInvite, string, error) {
	code, err := utils.NewCode(constant.PatientInviteCodeAlphabet, constant.PatientInviteCodeLength)
	if err != nil {
		return nil, "", fmt.Errorf("generate patient invite code: %w", err)
	}

	var created *models.PatientInvite
	err = pis.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		links, err := pis.userPatients.ListActiveByPatientForUpdate(txCtx, patientID)
		if err != nil {
			return fmt.Errorf("list patient links: %w", err)
		}
		if err := authorize(links); err != nil {
			return err
		}
		if relationship == models.RelationshipSelf && hasSelfLink(links) {
			return apperrors.ErrPatientSelfLinkExists
		}

		now := pis.now()
		pending, err := pis.invites.CountPendingByPatient(txCtx, patientID, now)
		if err != nil {
			return fmt.Errorf("count pending patient invites: %w", err)
		}
		if pending >= constant.MaxPendingPatientInvites {
			return apperrors.ErrPatientInviteLimitReached
		}

		created, err = pis.invites.Create(txCtx, &models.PatientInvite{
			PatientID:           patientID,
			CreatedByUserID:     actingUserID,
			CodeHash:            utils.HashToken(code),
			PatientRelationship: relationship,
			AccessLevel:         accessLevel,
			Status:              models.PatientInviteStatusPending,
			ExpiresAt:           now.Add(expiresIn),
		})

		return err
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrPatientNotFound) ||
			errors.Is(err, apperrors.ErrPatientForbidden) ||
			errors.Is(err, apperrors.ErrPatientAlreadyClaimed) ||
			errors.Is(err, apperrors.ErrPatientSelfLinkExists) ||
			errors.Is(err, apperrors.ErrPatientInviteLimitReached) ||
			errors.Is(err, apperrors.ErrPatientInviteCodeExists) {
			return nil, "", err
		}
		return nil, "", fmt.Errorf("create patient invite: %w", err)
	}

	return created, code, nil
}

func (pis *PatientInviteService) authorizeFull(ctx context.Context, actingUserID uint64, patientID uint64) error {
	link, err := pis.userPatients.GetActiveByUserAndPatient(ctx, actingUserID, patientID)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserPatientNotFound) {
			return apperrors.ErrPatientNotFound
		}
		return fmt.Errorf("authorize patient invite: %w", err)
	}
	if link.AccessLevel != models.AccessLevelFull {
		return apperrors.ErrPatientForbidden
	}

	return nil
}

// recordFailedAttempt runs outside the redeem transaction, which has rolled
// back, so the attempt and an expired invite's status are kept.
func (pis *PatientInviteService) recordFailedAttempt(ctx context.Context, actingUserID uint64, now time.Time, expiredInviteID uint64) error {
	if err := pis.attempts.Create(ctx, actingUserID, now); err != nil {
		return fmt.Errorf("record patient invite attempt: %w", err)
	}
	if err := pis.attempts.DeleteBefore(ctx, actingUserID, now.Add(-constant.PatientInviteAttemptWindow)); err != nil {
		return fmt.Errorf("delete old patient invite attempts: %w", err)
	}
	if expiredInviteID != 0 {
		if err := pis.invites.MarkExpired(ctx, expiredInviteID); err != nil && !errors.Is(err, apperrors.ErrPatientInviteNotFound) {
			return fmt.Errorf("mark patient invite expired: %w", err)
		}
	}

	return nil
}
