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

// StaffService lets a verified doctor add clinic staff (a compounder or
// receptionist) through a one-time invite code, and remove them. A staff
// account works for one doctor at a time; only the code's hash is stored, and
// wrong codes are capped per client IP because staff sign up with them.
type StaffService struct {
	users    UserRepository
	doctors  DoctorRepository
	staff    StaffRepository
	invites  StaffInviteRepository
	attempts StaffInviteAttemptRepository
	sessions SessionRepository
	tx       TransactionManager
	now      func() time.Time
	logger   Logger
}

func NewStaffService(
	users UserRepository,
	doctors DoctorRepository,
	staff StaffRepository,
	invites StaffInviteRepository,
	attempts StaffInviteAttemptRepository,
	sessions SessionRepository,
	tx TransactionManager,
	now func() time.Time,
	logger Logger,
) (*StaffService, error) {
	if users == nil {
		return nil, apperrors.ErrNilUserRepository
	}
	if doctors == nil {
		return nil, apperrors.ErrNilDoctorRepository
	}
	if staff == nil {
		return nil, apperrors.ErrNilStaffRepository
	}
	if invites == nil {
		return nil, apperrors.ErrNilStaffInviteRepository
	}
	if attempts == nil {
		return nil, apperrors.ErrNilStaffInviteAttemptRepository
	}
	if sessions == nil {
		return nil, apperrors.ErrNilSessionRepository
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

	return &StaffService{
		users:    users,
		doctors:  doctors,
		staff:    staff,
		invites:  invites,
		attempts: attempts,
		sessions: sessions,
		tx:       tx,
		now:      now,
		logger:   logger,
	}, nil
}

// CreateInvite issues a staff invite for the acting doctor, who must be
// verified, and returns it with its code, which is shown only this once.
func (ss *StaffService) CreateInvite(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
) (*models.StaffInvite, string, error) {
	doctor, err := ss.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, "", err
	}
	if doctor.VerificationStatus != models.VerificationStatusVerified {
		return nil, "", apperrors.Invalid(apperrors.ErrDoctorNotVerified, "", "your profile must be verified before you add staff")
	}

	now := ss.now()
	pending, err := ss.invites.CountPendingByDoctor(ctx, doctor.ID, now)
	if err != nil {
		return nil, "", fmt.Errorf("count pending staff invites: %w", err)
	}
	if pending >= constant.MaxPendingStaffInvites {
		return nil, "", apperrors.Invalid(apperrors.ErrStaffInviteLimitReached, "", fmt.Sprintf("at most %d staff invites can be open at once; revoke one first", constant.MaxPendingStaffInvites))
	}

	for try := 1; ; try++ {
		code, err := utils.NewCode(constant.PatientInviteCodeAlphabet, constant.PatientInviteCodeLength)
		if err != nil {
			return nil, "", fmt.Errorf("create staff invite code: %w", err)
		}
		invite, err := ss.invites.Create(ctx, &models.StaffInvite{
			DoctorID:        doctor.ID,
			CreatedByUserID: actingUserID,
			CodeHash:        utils.HashToken(code),
			Status:          models.StaffInviteStatusPending,
			ExpiresAt:       now.Add(constant.StaffInviteTTL),
		})
		if errors.Is(err, apperrors.ErrStaffInviteCodeExists) && try < constant.MaxStaffInviteCodeTries {
			continue
		}
		if err != nil {
			return nil, "", fmt.Errorf("create staff invite: %w", err)
		}
		ss.logger.InfoContext(ctx, "staff invite created", "invite_id", invite.ID, "doctor_id", doctor.ID)

		return invite, code, nil
	}
}

// ListInvites returns the acting doctor's pending staff invites, without codes.
func (ss *StaffService) ListInvites(ctx context.Context, actingUserID uint64, actingRole models.Role) ([]*models.StaffInvite, error) {
	doctor, err := ss.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}

	invites, err := ss.invites.ListPendingByDoctor(ctx, doctor.ID, ss.now())
	if err != nil {
		return nil, fmt.Errorf("list staff invites: %w", err)
	}

	return invites, nil
}

// RevokeInvite cancels one of the acting doctor's pending staff invites.
func (ss *StaffService) RevokeInvite(ctx context.Context, actingUserID uint64, actingRole models.Role, inviteID uint64) error {
	if inviteID == 0 {
		return fmt.Errorf("revoke staff invite without id: %w", apperrors.ErrInvalidStaffInput)
	}
	doctor, err := ss.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return err
	}

	if err := ss.invites.Revoke(ctx, doctor.ID, inviteID); err != nil {
		if errors.Is(err, apperrors.ErrStaffInviteNotFound) {
			return err
		}
		return fmt.Errorf("revoke staff invite: %w", err)
	}
	ss.logger.InfoContext(ctx, "staff invite revoked", "invite_id", inviteID, "doctor_id", doctor.ID)

	return nil
}

// List returns the acting doctor's current staff.
func (ss *StaffService) List(ctx context.Context, actingUserID uint64, actingRole models.Role) ([]*models.StaffMember, error) {
	doctor, err := ss.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return nil, err
	}

	staff, err := ss.staff.ListActiveByDoctor(ctx, doctor.ID)
	if err != nil {
		return nil, fmt.Errorf("list staff: %w", err)
	}

	return staff, nil
}

// Remove ends a staff member's work for the acting doctor and signs them out
// everywhere. Their account stays and can join another doctor.
func (ss *StaffService) Remove(ctx context.Context, actingUserID uint64, actingRole models.Role, staffUserID uint64) error {
	if staffUserID == 0 {
		return fmt.Errorf("remove staff without user id: %w", apperrors.ErrInvalidStaffInput)
	}
	doctor, err := ss.ownDoctor(ctx, actingUserID, actingRole)
	if err != nil {
		return err
	}

	err = ss.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ss.staff.Remove(txCtx, doctor.ID, staffUserID, actingUserID, ss.now()); err != nil {
			return err
		}
		_, err := ss.sessions.RevokeActiveByUser(txCtx, staffUserID, 0)
		return err
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrStaffNotFound) {
			return err
		}
		return fmt.Errorf("remove staff: %w", err)
	}
	ss.logger.InfoContext(ctx, "staff removed", "doctor_id", doctor.ID, "staff_user_id", staffUserID)

	return nil
}

// Register signs up a new staff account with an invite code: the account, its
// link to the invite's doctor and the redeemed invite are written together.
// user carries the name, email and mobile number; its role is always staff.
func (ss *StaffService) Register(
	ctx context.Context,
	user *models.User,
	password string,
	code string,
	clientIP string,
) (*models.User, error) {
	if user == nil {
		return nil, apperrors.ErrInvalidStaffInput
	}
	if len(password) < constant.MinPasswordLength {
		return nil, apperrors.Invalid(apperrors.ErrInvalidRegisterInput, "password", fmt.Sprintf("must be at least %d characters", constant.MinPasswordLength))
	}
	passwordHash, err := utils.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash staff password: %w", err)
	}
	user.PasswordHash = passwordHash
	user.Role = models.RoleStaff

	var created *models.User
	invite, err := ss.redeem(ctx, code, clientIP, func(txCtx context.Context, doctorID uint64) (uint64, error) {
		created, err = ss.users.Create(txCtx, user)
		if err != nil {
			return 0, duplicateUserReason(err)
		}
		_, err := ss.staff.Create(txCtx, &models.DoctorStaff{DoctorID: doctorID, UserID: created.ID})
		return created.ID, err
	})
	if err != nil {
		return nil, err
	}
	ss.logger.InfoContext(ctx, "staff registered", "user_id", created.ID, "doctor_id", invite.DoctorID)

	return created, nil
}

// Join links a staff account that works for no doctor now (removed by its
// last one) to the invite's doctor.
func (ss *StaffService) Join(
	ctx context.Context,
	actingUserID uint64,
	actingRole models.Role,
	code string,
	clientIP string,
) (*models.DoctorStaff, error) {
	if actingUserID == 0 {
		return nil, fmt.Errorf("join without acting user: %w", apperrors.ErrInvalidStaffInput)
	}
	if actingRole != models.RoleStaff {
		return nil, apperrors.ErrStaffNotLinked
	}

	var link *models.DoctorStaff
	if _, err := ss.redeem(ctx, code, clientIP, func(txCtx context.Context, doctorID uint64) (uint64, error) {
		created, err := ss.staff.Create(txCtx, &models.DoctorStaff{DoctorID: doctorID, UserID: actingUserID})
		if err != nil {
			if errors.Is(err, apperrors.ErrStaffAlreadyLinked) {
				return 0, apperrors.Invalid(apperrors.ErrStaffAlreadyLinked, "", "you already work for a doctor; ask them to remove you first")
			}
			return 0, err
		}
		link = created
		return actingUserID, nil
	}); err != nil {
		return nil, err
	}
	ss.logger.InfoContext(ctx, "staff joined doctor", "user_id", actingUserID, "doctor_id", link.DoctorID)

	return link, nil
}

// Me returns the doctor the acting staff account works for, with its link.
func (ss *StaffService) Me(ctx context.Context, actingUserID uint64, actingRole models.Role) (*models.DoctorStaff, *models.Doctor, error) {
	if actingUserID == 0 {
		return nil, nil, fmt.Errorf("staff me without acting user: %w", apperrors.ErrInvalidStaffInput)
	}
	if actingRole != models.RoleStaff {
		return nil, nil, apperrors.ErrStaffNotLinked
	}

	link, err := ss.staff.GetActiveByUser(ctx, actingUserID)
	if err != nil {
		if errors.Is(err, apperrors.ErrStaffNotFound) {
			return nil, nil, apperrors.ErrStaffNotLinked
		}
		return nil, nil, fmt.Errorf("get staff link: %w", err)
	}
	doctor, err := ss.doctors.GetByID(ctx, link.DoctorID)
	if err != nil {
		return nil, nil, fmt.Errorf("get staff doctor: %w", err)
	}

	return link, doctor, nil
}

// redeem checks an invite code and, in one transaction, runs join for the
// invite's doctor and marks the invite redeemed by the user join returns. An
// unknown, used, revoked or expired code is one ErrStaffInviteInvalid and
// counts against clientIP; past constant.MaxFailedStaffInviteAttempts within
// constant.StaffInviteAttemptWindow it is ErrStaffInviteAttemptsExceeded.
func (ss *StaffService) redeem(
	ctx context.Context,
	code string,
	clientIP string,
	join func(txCtx context.Context, doctorID uint64) (uint64, error),
) (*models.StaffInvite, error) {
	code = normalizeInviteCode(code)
	if len(code) != constant.PatientInviteCodeLength {
		return nil, apperrors.Invalid(apperrors.ErrInvalidStaffInput, "inviteCode", fmt.Sprintf("must be the %d-character code from the doctor", constant.PatientInviteCodeLength))
	}

	now := ss.now()
	failed, err := ss.attempts.CountSince(ctx, clientIP, now.Add(-constant.StaffInviteAttemptWindow))
	if err != nil {
		return nil, fmt.Errorf("count staff invite attempts: %w", err)
	}
	if failed >= constant.MaxFailedStaffInviteAttempts {
		return nil, apperrors.ErrStaffInviteAttemptsExceeded
	}

	var invite *models.StaffInvite
	err = ss.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		found, err := ss.invites.GetByCodeHashForUpdate(txCtx, utils.HashToken(code))
		if err != nil {
			if errors.Is(err, apperrors.ErrStaffInviteNotFound) {
				return apperrors.ErrStaffInviteInvalid
			}
			return fmt.Errorf("get staff invite: %w", err)
		}
		invite = found
		if found.Status != models.StaffInviteStatusPending || !now.Before(found.ExpiresAt) {
			return apperrors.ErrStaffInviteInvalid
		}

		userID, err := join(txCtx, found.DoctorID)
		if err != nil {
			return err
		}

		return ss.invites.MarkRedeemed(txCtx, found.ID, userID, now)
	})
	if errors.Is(err, apperrors.ErrStaffInviteInvalid) {
		ss.recordFailedAttempt(ctx, clientIP, invite, now)
		return nil, err
	}
	if err != nil {
		var invalid *apperrors.ValidationError
		if errors.As(err, &invalid) || errors.Is(err, apperrors.ErrUserAlreadyExists) {
			return nil, err
		}
		return nil, fmt.Errorf("redeem staff invite: %w", err)
	}

	return invite, nil
}

// recordFailedAttempt runs outside the redeem transaction, which has rolled
// back, so the attempt and an expired invite's status are kept.
func (ss *StaffService) recordFailedAttempt(ctx context.Context, clientIP string, invite *models.StaffInvite, now time.Time) {
	if err := ss.attempts.Create(ctx, clientIP, now); err != nil {
		ss.logger.WarnContext(ctx, "record staff invite attempt failed", "error", err)
	}
	if err := ss.attempts.DeleteBefore(ctx, clientIP, now.Add(-constant.StaffInviteAttemptWindow)); err != nil {
		ss.logger.WarnContext(ctx, "prune staff invite attempts failed", "error", err)
	}
	if invite != nil && invite.Status == models.StaffInviteStatusPending && !now.Before(invite.ExpiresAt) {
		if err := ss.invites.MarkExpired(ctx, invite.ID); err != nil && !errors.Is(err, apperrors.ErrStaffInviteNotFound) {
			ss.logger.WarnContext(ctx, "expire staff invite failed", "invite_id", invite.ID, "error", err)
		}
	}
}

func (ss *StaffService) ownDoctor(ctx context.Context, actingUserID uint64, actingRole models.Role) (*models.Doctor, error) {
	if err := validateDoctorActor(actingUserID, actingRole); err != nil {
		return nil, err
	}
	doctor, err := ss.doctors.GetByUserID(ctx, actingUserID)
	if err != nil {
		if errors.Is(err, apperrors.ErrDoctorNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("get own doctor profile: %w", err)
	}

	return doctor, nil
}
