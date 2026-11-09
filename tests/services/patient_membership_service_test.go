package services_test

import (
	"context"
	"errors"
	"testing"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
)

func newTestMembershipService(
	t *testing.T,
	links *fakeUserPatientRepo,
	users *fakeUserRepo,
	history *fakeUserPatientHistoryRepo,
	outbox *fakeOutboxRepo,
) *services.PatientMembershipService {
	t.Helper()

	service, err := services.NewPatientMembershipService(links, users, history, outbox, &fakeTxManager{}, discardLogger)
	if err != nil {
		t.Fatalf("NewPatientMembershipService returned error: %v", err)
	}

	return service
}

func memberLink(userID uint64, relationship models.Relationship, accessLevel models.AccessLevel) *models.UserPatient {
	return &models.UserPatient{UserID: userID, PatientID: 10, PatientRelationship: relationship, AccessLevel: accessLevel, Status: models.UserPatientStatusActive}
}

func TestNewPatientMembershipServiceValidation(t *testing.T) {
	if _, err := services.NewPatientMembershipService(&fakeUserPatientRepo{}, nil, &fakeUserPatientHistoryRepo{}, &fakeOutboxRepo{}, &fakeTxManager{}, discardLogger); !errors.Is(err, apperrors.ErrNilUserRepository) {
		t.Fatalf("nil users error = %v, want %v", err, apperrors.ErrNilUserRepository)
	}
	if _, err := services.NewPatientMembershipService(&fakeUserPatientRepo{}, &fakeUserRepo{}, nil, &fakeOutboxRepo{}, &fakeTxManager{}, discardLogger); !errors.Is(err, apperrors.ErrNilUserPatientHistoryRepository) {
		t.Fatalf("nil history error = %v, want %v", err, apperrors.ErrNilUserPatientHistoryRepository)
	}
}

func TestPatientMembershipServiceList(t *testing.T) {
	tests := []struct {
		name    string
		links   []*models.UserPatient
		wantErr error
	}{
		{name: "full holder sees everyone", links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelFull), memberLink(8, models.RelationshipFather, models.AccessLevelReadOnly)}},
		{name: "read only holder is forbidden", links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelReadOnly)}, wantErr: apperrors.ErrPatientForbidden},
		{name: "not linked", links: []*models.UserPatient{memberLink(8, models.RelationshipFather, models.AccessLevelFull)}, wantErr: apperrors.ErrPatientNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			users := &fakeUserRepo{users: map[uint64]*models.User{7: {ID: 7, Name: "Meera", MobileNo: "9876543210"}, 8: {ID: 8, Name: "Ravi"}}}
			service := newTestMembershipService(t, &fakeUserPatientRepo{patientLinks: tt.links}, users, &fakeUserPatientHistoryRepo{}, &fakeOutboxRepo{})

			members, err := service.List(context.Background(), 7, 10)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("List error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if len(members) != 2 || members[0].Name != "Meera" || members[0].MobileNo != "9876543210" || members[1].Link.UserID != 8 {
				t.Fatalf("List = %+v, want both members with their names", members)
			}
		})
	}
}

func TestPatientMembershipServiceUpdate(t *testing.T) {
	tests := []struct {
		name         string
		links        []*models.UserPatient
		memberUserID uint64
		accessLevel  models.AccessLevel
		wantErr      error
	}{
		{name: "full holder downgrades another member", links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelFull), memberLink(8, models.RelationshipFather, models.AccessLevelFull)}, memberUserID: 8, accessLevel: models.AccessLevelReadOnly},
		{name: "own access", links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelFull)}, memberUserID: 7, accessLevel: models.AccessLevelReadOnly, wantErr: apperrors.ErrInvalidPatientInput},
		{name: "self link is protected", links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelFull), memberLink(8, models.RelationshipSelf, models.AccessLevelFull)}, memberUserID: 8, accessLevel: models.AccessLevelReadOnly, wantErr: apperrors.ErrPatientForbidden},
		{name: "appointments only holder cannot manage", links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelAppointmentsOnly), memberLink(8, models.RelationshipFather, models.AccessLevelFull)}, memberUserID: 8, accessLevel: models.AccessLevelReadOnly, wantErr: apperrors.ErrPatientForbidden},
		{name: "unknown member", links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelFull)}, memberUserID: 9, accessLevel: models.AccessLevelReadOnly, wantErr: apperrors.ErrUserPatientNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := &fakeUserPatientRepo{patientLinks: tt.links}
			history := &fakeUserPatientHistoryRepo{}
			outbox := &fakeOutboxRepo{}
			service := newTestMembershipService(t, links, &fakeUserRepo{}, history, outbox)

			updated, err := service.Update(context.Background(), 7, 10, tt.memberUserID, tt.accessLevel)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Update error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if links.updateCalled {
					t.Fatalf("Update changed a link on a refused request")
				}
				return
			}
			if updated.AccessLevel != tt.accessLevel || links.updateInput.UserID != tt.memberUserID {
				t.Fatalf("Update = %+v, want user %d at %s", updated, tt.memberUserID, tt.accessLevel)
			}
			if len(history.entries) != 1 || history.entries[0].Reason != models.UserPatientChangeAccessChanged || history.entries[0].FromAccessLevel != models.AccessLevelFull {
				t.Fatalf("Update history = %+v, want one access_changed entry from full", history.entries)
			}
			if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventPatientAccessChanged {
				t.Fatalf("Update outbox = %+v, want one %s event", outbox.events, models.OutboxEventPatientAccessChanged)
			}
		})
	}
}

func TestPatientMembershipServiceRevoke(t *testing.T) {
	tests := []struct {
		name         string
		actingUserID uint64
		links        []*models.UserPatient
		memberUserID uint64
		wantErr      error
	}{
		{name: "full holder revokes a caregiver", actingUserID: 7, links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelFull), memberLink(8, models.RelationshipCaregiver, models.AccessLevelReadOnly)}, memberUserID: 8},
		{name: "self link revokes the creator", actingUserID: 8, links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelFull), memberLink(8, models.RelationshipSelf, models.AccessLevelFull)}, memberUserID: 7},
		{name: "creator cannot revoke the self link", actingUserID: 7, links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelFull), memberLink(8, models.RelationshipSelf, models.AccessLevelFull)}, memberUserID: 8, wantErr: apperrors.ErrPatientForbidden},
		{name: "own access", actingUserID: 7, links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelFull)}, memberUserID: 7, wantErr: apperrors.ErrInvalidPatientInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := &fakeUserPatientRepo{patientLinks: tt.links}
			history := &fakeUserPatientHistoryRepo{}
			outbox := &fakeOutboxRepo{}
			service := newTestMembershipService(t, links, &fakeUserRepo{}, history, outbox)

			err := service.Revoke(context.Background(), tt.actingUserID, 10, tt.memberUserID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Revoke error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if links.revokedUserID != 0 {
					t.Fatalf("Revoke revoked user %d on a refused request", links.revokedUserID)
				}
				return
			}
			if links.revokedUserID != tt.memberUserID {
				t.Fatalf("Revoke revoked user %d, want %d", links.revokedUserID, tt.memberUserID)
			}
			if len(history.entries) != 1 || history.entries[0].Reason != models.UserPatientChangeRevokedByHolder || history.entries[0].ChangedByUserID != tt.actingUserID {
				t.Fatalf("Revoke history = %+v, want one revoked_by_holder entry by %d", history.entries, tt.actingUserID)
			}
			if len(outbox.events) != 1 || outbox.events[0].EventType != models.OutboxEventPatientAccessRevoked {
				t.Fatalf("Revoke outbox = %+v, want one %s event", outbox.events, models.OutboxEventPatientAccessRevoked)
			}
		})
	}
}

func TestPatientMembershipServiceLeave(t *testing.T) {
	tests := []struct {
		name    string
		links   []*models.UserPatient
		wantErr error
	}{
		{name: "leaves while another full holder stays", links: []*models.UserPatient{memberLink(7, models.RelationshipFather, models.AccessLevelFull), memberLink(8, models.RelationshipMother, models.AccessLevelFull)}},
		{name: "read only member leaves", links: []*models.UserPatient{memberLink(7, models.RelationshipCaregiver, models.AccessLevelReadOnly), memberLink(8, models.RelationshipMother, models.AccessLevelFull)}},
		{name: "last full holder cannot leave", links: []*models.UserPatient{memberLink(7, models.RelationshipMother, models.AccessLevelFull), memberLink(8, models.RelationshipFather, models.AccessLevelReadOnly)}, wantErr: apperrors.ErrPatientLastFullHolder},
		{name: "not linked", links: []*models.UserPatient{memberLink(8, models.RelationshipMother, models.AccessLevelFull)}, wantErr: apperrors.ErrPatientNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := &fakeUserPatientRepo{patientLinks: tt.links}
			history := &fakeUserPatientHistoryRepo{}
			service := newTestMembershipService(t, links, &fakeUserRepo{}, history, &fakeOutboxRepo{})

			err := service.Leave(context.Background(), 7, 10)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Leave error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if links.revokedUserID != 7 || len(history.entries) != 1 || history.entries[0].Reason != models.UserPatientChangeLeft {
				t.Fatalf("Leave revoked user %d with history %+v, want user 7 with one left entry", links.revokedUserID, history.entries)
			}
		})
	}
}
