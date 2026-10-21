package authz_test

import (
	"appointment_system/internal/appcontext"
	"appointment_system/internal/apps/appointment/authz"
	"appointment_system/internal/apps/appointment/models"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name       string
		role       models.Role
		permission authz.Permission
		want       bool
	}{
		{name: "admin can verify doctors", role: models.RoleAdmin, permission: authz.PermDoctorVerify, want: true},
		{name: "admin can manage admins", role: models.RoleAdmin, permission: authz.PermAdminManage, want: true},
		{name: "admin cannot manage a doctor profile", role: models.RoleAdmin, permission: authz.PermDoctorProfileManage, want: false},
		{name: "doctor can manage own profile", role: models.RoleDoctor, permission: authz.PermDoctorProfileManage, want: true},
		{name: "doctor cannot verify doctors", role: models.RoleDoctor, permission: authz.PermDoctorVerify, want: false},
		{name: "doctor has no admin access", role: models.RoleDoctor, permission: authz.PermAdminAccess, want: false},
		{name: "patient has no admin access", role: models.RolePatient, permission: authz.PermAdminAccess, want: false},
		{name: "patient can book appointments", role: models.RolePatient, permission: authz.PermAppointmentBook, want: true},
		{name: "doctor cannot book appointments", role: models.RoleDoctor, permission: authz.PermAppointmentBook, want: false},
		{name: "doctor manages own appointments", role: models.RoleDoctor, permission: authz.PermAppointmentManage, want: true},
		{name: "patient cannot manage doctor appointments", role: models.RolePatient, permission: authz.PermAppointmentManage, want: false},
		{name: "patient can pay", role: models.RolePatient, permission: authz.PermPaymentPay, want: true},
		{name: "doctor cannot pay", role: models.RoleDoctor, permission: authz.PermPaymentPay, want: false},
		{name: "admin can refund", role: models.RoleAdmin, permission: authz.PermPaymentRefund, want: true},
		{name: "patient cannot refund", role: models.RolePatient, permission: authz.PermPaymentRefund, want: false},
		{name: "doctor writes medical records", role: models.RoleDoctor, permission: authz.PermRecordWrite, want: true},
		{name: "patient cannot write medical records", role: models.RolePatient, permission: authz.PermRecordWrite, want: false},
		{name: "admin audits record access", role: models.RoleAdmin, permission: authz.PermRecordAudit, want: true},
		{name: "admin cannot write medical records", role: models.RoleAdmin, permission: authz.PermRecordWrite, want: false},
		{name: "unknown role has nothing", role: models.Role("ghost"), permission: authz.PermAdminAccess, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authz.HasPermission(tt.role, tt.permission); got != tt.want {
				t.Fatalf("HasPermission(%q, %q) = %v, want %v", tt.role, tt.permission, got, tt.want)
			}
		})
	}
}

func TestRequirePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		role       string
		withAppCtx bool
		wantStatus int
	}{
		{name: "admin is allowed", role: "admin", withAppCtx: true, wantStatus: http.StatusOK},
		{name: "patient is forbidden", role: "patient", withAppCtx: true, wantStatus: http.StatusForbidden},
		{name: "doctor is forbidden", role: "doctor", withAppCtx: true, wantStatus: http.StatusForbidden},
		{name: "missing role is unauthorized", role: "", withAppCtx: true, wantStatus: http.StatusUnauthorized},
		{name: "missing app context is unauthorized", withAppCtx: false, wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(ctx *gin.Context) {
				if tt.withAppCtx {
					applicationContext := appcontext.New(ctx.Request.Context(), "test")
					applicationContext.PrincipalRole = tt.role
					ctx.Request = ctx.Request.WithContext(appcontext.With(ctx.Request.Context(), applicationContext))
				}
				ctx.Next()
			})
			router.GET("/", authz.RequirePermission(authz.PermDoctorVerify), func(ctx *gin.Context) {
				ctx.Status(http.StatusOK)
			})

			request := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(context.Background())
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}
