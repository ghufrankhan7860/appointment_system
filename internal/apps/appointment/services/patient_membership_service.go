package services

import (
	"context"
	"errors"
	"fmt"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
)

// PatientMembershipService manages who has access to a patient. A self link
// is the patient's own account: it always keeps full access and no one else
// can change or revoke it. Every change leaves the patient at least one full
// holder, and revoking access never touches past appointments.
type PatientMembershipService struct {
	userPatients UserPatientRepository
	users        UserRepository
	history      UserPatientHistoryRepository
	outbox       OutboxRepository
	tx           TransactionManager
	logger       Logger
}

func NewPatientMembershipService(
	userPatients UserPatientRepository,
	users UserRepository,
	history UserPatientHistoryRepository,
	outbox OutboxRepository,
	tx TransactionManager,
	logger Logger,
) (*PatientMembershipService, error) {
	if userPatients == nil {
		return nil, apperrors.ErrNilUserPatientRepository
	}
	if users == nil {
		return nil, apperrors.ErrNilUserRepository
	}
	if history == nil {
		return nil, apperrors.ErrNilUserPatientHistoryRepository
	}
	if outbox == nil {
		return nil, apperrors.ErrNilOutboxRepository
	}
	if tx == nil {
		return nil, apperrors.ErrNilTransactionManager
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}

	return &PatientMembershipService{
		userPatients: userPatients,
		users:        users,
		history:      history,
		outbox:       outbox,
		tx:           tx,
		logger:       logger,
	}, nil
}

// List returns everyone with active access to the patient. Only a full holder
// may see it.
func (pms *PatientMembershipService) List(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
) ([]*models.PatientMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list patient members context: %w", err)
	}
	if actingUserID == 0 || patientID == 0 {
		return nil, fmt.Errorf("list patient members with missing ids: %w", apperrors.ErrInvalidPatientInput)
	}

	links, err := pms.userPatients.ListActiveByPatient(ctx, patientID)
	if err != nil {
		return nil, fmt.Errorf("list patient links: %w", err)
	}
	if err := authorizeMember(links, actingUserID, true); err != nil {
		return nil, err
	}

	ids := make([]uint64, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.UserID)
	}
	users, err := pms.users.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get patient member users: %w", err)
	}

	members := make([]*models.PatientMember, 0, len(links))
	for _, link := range links {
		member := &models.PatientMember{Link: *link}
		if user, ok := users[link.UserID]; ok {
			member.Name = user.Name
			member.MobileNo = user.MobileNo
		}
		members = append(members, member)
	}

	return members, nil
}

// Update changes another member's access level. A self link cannot be
// changed, and a full holder cannot be downgraded when they are the last one.
func (pms *PatientMembershipService) Update(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
	memberUserID uint64,
	accessLevel models.AccessLevel,
) (*models.UserPatient, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("update patient member context: %w", err)
	}
	if actingUserID == 0 || patientID == 0 || memberUserID == 0 {
		return nil, fmt.Errorf("update patient member with missing ids: %w", apperrors.ErrInvalidPatientInput)
	}
	if memberUserID == actingUserID {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPatientInput, "userId", "cannot be your own access")
	}
	if !validAccessLevel(accessLevel) {
		return nil, apperrors.Invalid(apperrors.ErrInvalidPatientInput, "accessLevel", "must be one of full, appointments_only, read_only")
	}

	var updated *models.UserPatient
	err := pms.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		links, err := pms.userPatients.ListActiveByPatientForUpdate(txCtx, patientID)
		if err != nil {
			return fmt.Errorf("list patient links: %w", err)
		}
		member, err := managedMember(links, actingUserID, memberUserID)
		if err != nil {
			return err
		}
		if member.AccessLevel == accessLevel {
			updated = member
			return nil
		}
		if fullHoldersAfter(links, memberUserID, accessLevel) == 0 {
			return apperrors.ErrPatientLastFullHolder
		}

		next := *member
		next.AccessLevel = accessLevel
		updated, err = pms.userPatients.Update(txCtx, &next)
		if err != nil {
			return fmt.Errorf("update user-patient link: %w", err)
		}

		return pms.recordChange(txCtx, actingUserID, member, updated, models.UserPatientChangeAccessChanged, models.OutboxEventPatientAccessChanged)
	})
	if err != nil {
		if isMembershipError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("update patient member: %w", err)
	}

	pms.logger.InfoContext(ctx, "patient member updated", "patient_id", patientID, "member_user_id", memberUserID, "access_level", accessLevel)

	return updated, nil
}

// Revoke removes another member's access. A self link cannot be revoked by
// anyone else.
func (pms *PatientMembershipService) Revoke(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
	memberUserID uint64,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("revoke patient member context: %w", err)
	}
	if actingUserID == 0 || patientID == 0 || memberUserID == 0 {
		return fmt.Errorf("revoke patient member with missing ids: %w", apperrors.ErrInvalidPatientInput)
	}
	if memberUserID == actingUserID {
		return apperrors.Invalid(apperrors.ErrInvalidPatientInput, "userId", "cannot be your own access; leave instead")
	}

	err := pms.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		links, err := pms.userPatients.ListActiveByPatientForUpdate(txCtx, patientID)
		if err != nil {
			return fmt.Errorf("list patient links: %w", err)
		}
		member, err := managedMember(links, actingUserID, memberUserID)
		if err != nil {
			return err
		}
		if fullHoldersAfter(links, memberUserID, "") == 0 {
			return apperrors.ErrPatientLastFullHolder
		}

		revoked, err := pms.userPatients.Revoke(txCtx, memberUserID, patientID)
		if err != nil {
			return fmt.Errorf("revoke user-patient link: %w", err)
		}

		return pms.recordChange(txCtx, actingUserID, member, revoked, models.UserPatientChangeRevokedByHolder, models.OutboxEventPatientAccessRevoked)
	})
	if err != nil {
		if isMembershipError(err) {
			return err
		}
		return fmt.Errorf("revoke patient member: %w", err)
	}

	pms.logger.InfoContext(ctx, "patient member revoked", "patient_id", patientID, "member_user_id", memberUserID)

	return nil
}

// Leave removes the acting user's own access. The last full holder cannot
// leave until someone else has full access.
func (pms *PatientMembershipService) Leave(
	ctx context.Context,
	actingUserID uint64,
	patientID uint64,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("leave patient context: %w", err)
	}
	if actingUserID == 0 || patientID == 0 {
		return fmt.Errorf("leave patient with missing ids: %w", apperrors.ErrInvalidPatientInput)
	}

	err := pms.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		links, err := pms.userPatients.ListActiveByPatientForUpdate(txCtx, patientID)
		if err != nil {
			return fmt.Errorf("list patient links: %w", err)
		}
		actor := findLink(links, actingUserID)
		if actor == nil {
			return apperrors.ErrPatientNotFound
		}
		if fullHoldersAfter(links, actingUserID, "") == 0 {
			return apperrors.ErrPatientLastFullHolder
		}

		revoked, err := pms.userPatients.Revoke(txCtx, actingUserID, patientID)
		if err != nil {
			return fmt.Errorf("revoke user-patient link: %w", err)
		}

		return pms.recordChange(txCtx, actingUserID, actor, revoked, models.UserPatientChangeLeft, models.OutboxEventPatientAccessRevoked)
	})
	if err != nil {
		if isMembershipError(err) {
			return err
		}
		return fmt.Errorf("leave patient: %w", err)
	}

	pms.logger.InfoContext(ctx, "patient left", "patient_id", patientID)

	return nil
}

func (pms *PatientMembershipService) recordChange(
	ctx context.Context,
	actingUserID uint64,
	before *models.UserPatient,
	after *models.UserPatient,
	reason models.UserPatientChangeReason,
	eventType string,
) error {
	if err := pms.history.Create(ctx, &models.UserPatientHistory{
		UserID:          after.UserID,
		PatientID:       after.PatientID,
		ChangedByUserID: actingUserID,
		FromStatus:      before.Status,
		ToStatus:        after.Status,
		FromAccessLevel: before.AccessLevel,
		ToAccessLevel:   after.AccessLevel,
		Reason:          reason,
	}); err != nil {
		return fmt.Errorf("create user-patient history: %w", err)
	}

	return pms.outbox.Create(ctx, patientAccessEvent(eventType, after, actingUserID, reason))
}

// authorizeMember checks the acting user holds the patient, with full access
// when needFull.
func authorizeMember(links []*models.UserPatient, actingUserID uint64, needFull bool) error {
	actor := findLink(links, actingUserID)
	if actor == nil {
		return apperrors.ErrPatientNotFound
	}
	if needFull && actor.AccessLevel != models.AccessLevelFull {
		return apperrors.ErrPatientForbidden
	}

	return nil
}

// managedMember returns the member a full holder is changing; a self link is
// only the patient's own to change.
func managedMember(links []*models.UserPatient, actingUserID uint64, memberUserID uint64) (*models.UserPatient, error) {
	if err := authorizeMember(links, actingUserID, true); err != nil {
		return nil, err
	}
	member := findLink(links, memberUserID)
	if member == nil {
		return nil, apperrors.ErrUserPatientNotFound
	}
	if member.PatientRelationship == models.RelationshipSelf {
		return nil, apperrors.ErrPatientForbidden
	}

	return member, nil
}

func isMembershipError(err error) bool {
	return errors.Is(err, apperrors.ErrPatientNotFound) ||
		errors.Is(err, apperrors.ErrPatientForbidden) ||
		errors.Is(err, apperrors.ErrUserPatientNotFound) ||
		errors.Is(err, apperrors.ErrPatientLastFullHolder)
}
