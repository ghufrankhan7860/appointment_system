package services_test

import (
	"context"
	"errors"
	"testing"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/models"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/apps/appointment/utils"
)

func TestAdminUserServiceCreateFirst(t *testing.T) {
	tests := []struct {
		name     string
		repo     *fakeAdminUserRepo
		user     *models.User
		password string
		wantErr  error
	}{
		{name: "first admin is created", repo: &fakeAdminUserRepo{}, user: &models.User{Name: "Ops", Email: " Ops@Example.com ", MobileNo: "999"}, password: "a-long-password"},
		{name: "refuses when an admin exists", repo: &fakeAdminUserRepo{adminExists: true}, user: &models.User{Name: "Ops", Email: "ops@example.com", MobileNo: "999"}, password: "a-long-password", wantErr: apperrors.ErrAdminAlreadyExists},
		{name: "short password", repo: &fakeAdminUserRepo{}, user: &models.User{Name: "Ops", Email: "ops@example.com", MobileNo: "999"}, password: "short", wantErr: apperrors.ErrInvalidAdminInput},
		{name: "missing email", repo: &fakeAdminUserRepo{}, user: &models.User{Name: "Ops", MobileNo: "999"}, password: "a-long-password", wantErr: apperrors.ErrInvalidAdminInput},
		{name: "duplicate user", repo: &fakeAdminUserRepo{createErr: apperrors.ErrUserAlreadyExists}, user: &models.User{Name: "Ops", Email: "ops@example.com", MobileNo: "999"}, password: "a-long-password", wantErr: apperrors.ErrUserAlreadyExists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, err := services.NewAdminUserService(tt.repo)
			if err != nil {
				t.Fatalf("NewAdminUserService returned error: %v", err)
			}

			created, err := service.CreateFirst(context.Background(), tt.user, tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("CreateFirst error = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}

			if created.Role != models.RoleAdmin || created.Email != "ops@example.com" {
				t.Fatalf("created admin = %+v", created)
			}
			if err := utils.CheckPassword(tt.repo.createInput.PasswordHash, tt.password); err != nil {
				t.Fatalf("stored password hash does not match: %v", err)
			}
		})
	}
}

func TestAdminUserServiceCreateForcesAdminRole(t *testing.T) {
	repo := &fakeAdminUserRepo{adminExists: true}
	service, _ := services.NewAdminUserService(repo)

	created, err := service.Create(context.Background(), &models.User{Name: "Ops", Email: "ops2@example.com", MobileNo: "999", Role: models.RolePatient}, "a-long-password")
	if err != nil {
		t.Fatalf("Create error = %v", err)
	}
	if created.Role != models.RoleAdmin {
		t.Fatalf("role = %s, want admin", created.Role)
	}
}
