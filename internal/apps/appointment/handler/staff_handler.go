package handler

import (
	"errors"
	"net/http"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/authz"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/staff"
	"appointment_system/middleware"

	"github.com/gin-gonic/gin"
)

// StaffHandler serves clinic staff: signing up with a doctor's invite, the
// staff account's own doctor, and the doctor's invites and staff list.
type StaffHandler struct {
	staffController *controller.StaffController
	tokenVerifier   middleware.TokenVerifier
}

func NewStaffHandler(staffController *controller.StaffController, tokenVerifier middleware.TokenVerifier) (*StaffHandler, error) {
	if staffController == nil {
		return nil, apperrors.ErrNilStaffController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}

	return &StaffHandler{staffController: staffController, tokenVerifier: tokenVerifier}, nil
}

func (sh *StaffHandler) RegisterRoutes(router *gin.RouterGroup) {
	authenticate := middleware.Authenticate(sh.tokenVerifier)
	manageProfile := authz.RequirePermission(authz.PermDoctorProfileManage)

	router.POST("/auth/register-staff", sh.Register)
	router.GET("/staff/me", authenticate, sh.Me)
	router.POST("/staff/me/join", authenticate, sh.Join)

	doctorStaff := router.Group("/doctors/me", authenticate, manageProfile)
	doctorStaff.POST("/staff/invites", sh.CreateInvite)
	doctorStaff.GET("/staff/invites", sh.ListInvites)
	doctorStaff.DELETE("/staff/invites/:id", sh.RevokeInvite)
	doctorStaff.GET("/staff", sh.List)
	doctorStaff.DELETE("/staff/:userId", sh.Remove)
}

// Register godoc
// @Summary Sign up as clinic staff
// @Description Creates a staff account (a doctor's compounder or receptionist) with the invite code the doctor gave. The account works for that doctor at once. Wrong codes are limited per IP address.
// @Tags staff
// @Accept json
// @Produce json
// @Param request body staff.RegisterStaffRequest true "Account details and invite code"
// @Success 201 {object} dtos.SuccessResponse{data=auth.RegisterResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 429 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /auth/register-staff [post]
func (sh *StaffHandler) Register(ctx *gin.Context) {
	var registerStaffRequest staff.RegisterStaffRequest
	if err := ctx.ShouldBindJSON(&registerStaffRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := sh.staffController.Register(ctx.Request.Context(), &registerStaffRequest, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidRegisterInput), errors.Is(err, apperrors.ErrInvalidStaffInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrStaffInviteInvalid):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Invite code is invalid or expired"})
		case errors.Is(err, apperrors.ErrStaffInviteAttemptsExceeded):
			ctx.JSON(http.StatusTooManyRequests, gin.H{"success": false, "error": "Too many wrong invite codes; try again later"})
		case errors.Is(err, apperrors.ErrUserAlreadyExists):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "User already exists",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to register staff"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Staff account created successfully",
	})
}

// Me godoc
// @Summary Get the doctor a staff account works for
// @Description Returns the doctor the calling staff account works for. 404 when it works for no doctor now (it was removed); join another with POST /staff/me/join.
// @Tags staff
// @Security BearerAuth
// @Produce json
// @Success 200 {object} dtos.SuccessResponse{data=staff.GetStaffMeResponse}
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /staff/me [get]
func (sh *StaffHandler) Me(ctx *gin.Context) {
	response, err := sh.staffController.Me(ctx.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrStaffNotLinked):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "This account does not work for a doctor"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get staff details"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Staff details retrieved successfully",
	})
}

// Join godoc
// @Summary Join a doctor as staff
// @Description Links a staff account that works for no doctor now to the doctor whose invite code it gives. Wrong codes are limited per IP address.
// @Tags staff
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body staff.JoinStaffRequest true "Invite code"
// @Success 200 {object} dtos.SuccessResponse{data=staff.GetStaffMeResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 429 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /staff/me/join [post]
func (sh *StaffHandler) Join(ctx *gin.Context) {
	var joinStaffRequest staff.JoinStaffRequest
	if err := ctx.ShouldBindJSON(&joinStaffRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := sh.staffController.Join(ctx.Request.Context(), &joinStaffRequest, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidStaffInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrStaffAlreadyLinked):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Already working for a doctor",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrStaffNotLinked):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only staff accounts can join a doctor"})
		case errors.Is(err, apperrors.ErrStaffInviteInvalid):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Invite code is invalid or expired"})
		case errors.Is(err, apperrors.ErrStaffInviteAttemptsExceeded):
			ctx.JSON(http.StatusTooManyRequests, gin.H{"success": false, "error": "Too many wrong invite codes; try again later"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to join doctor"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Joined doctor successfully",
	})
}

// CreateInvite godoc
// @Summary Invite clinic staff
// @Description Creates a one-time staff invite code for the calling doctor, who must be verified. The code is returned only in this response and works for 48 hours; at most 5 can be pending.
// @Tags staff
// @Security BearerAuth
// @Produce json
// @Success 201 {object} dtos.SuccessResponse{data=staff.StaffInviteResponse}
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/staff/invites [post]
func (sh *StaffHandler) CreateInvite(ctx *gin.Context) {
	response, err := sh.staffController.CreateInvite(ctx.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts can add staff"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		case errors.Is(err, apperrors.ErrDoctorNotVerified), errors.Is(err, apperrors.ErrStaffInviteLimitReached):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Cannot create a staff invite",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create staff invite"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Staff invite created successfully",
	})
}

// ListInvites godoc
// @Summary List pending staff invites
// @Description Lists the calling doctor's staff invites that are pending and not expired, without their codes.
// @Tags staff
// @Security BearerAuth
// @Produce json
// @Success 200 {object} dtos.SuccessResponse{data=staff.ListStaffInvitesResponse}
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/staff/invites [get]
func (sh *StaffHandler) ListInvites(ctx *gin.Context) {
	response, err := sh.staffController.ListInvites(ctx.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have staff"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list staff invites"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Staff invites retrieved successfully",
	})
}

// RevokeInvite godoc
// @Summary Revoke a staff invite
// @Description Cancels one of the calling doctor's pending staff invites.
// @Tags staff
// @Security BearerAuth
// @Produce json
// @Param id path int true "Invite ID"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/staff/invites/{id} [delete]
func (sh *StaffHandler) RevokeInvite(ctx *gin.Context) {
	var revokeStaffInviteRequest staff.RevokeStaffInviteRequest
	if err := ctx.ShouldBindUri(&revokeStaffInviteRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid invite id"})
		return
	}

	if err := sh.staffController.RevokeInvite(ctx.Request.Context(), &revokeStaffInviteRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have staff"})
		case errors.Is(err, apperrors.ErrDoctorNotFound), errors.Is(err, apperrors.ErrStaffInviteNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Staff invite not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to revoke staff invite"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true, "message": "Staff invite revoked successfully"})
}

// List godoc
// @Summary List clinic staff
// @Description Lists the staff working for the calling doctor, oldest first.
// @Tags staff
// @Security BearerAuth
// @Produce json
// @Success 200 {object} dtos.SuccessResponse{data=staff.ListStaffResponse}
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/staff [get]
func (sh *StaffHandler) List(ctx *gin.Context) {
	response, err := sh.staffController.List(ctx.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have staff"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list staff"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Staff retrieved successfully",
	})
}

// Remove godoc
// @Summary Remove a staff member
// @Description Ends a staff member's work for the calling doctor and signs them out on every device. Their account stays and can join another doctor.
// @Tags staff
// @Security BearerAuth
// @Produce json
// @Param userId path int true "Staff user ID"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/staff/{userId} [delete]
func (sh *StaffHandler) Remove(ctx *gin.Context) {
	var removeStaffRequest staff.RemoveStaffRequest
	if err := ctx.ShouldBindUri(&removeStaffRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid staff user id"})
		return
	}

	if err := sh.staffController.Remove(ctx.Request.Context(), &removeStaffRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have staff"})
		case errors.Is(err, apperrors.ErrDoctorNotFound), errors.Is(err, apperrors.ErrStaffNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Staff member not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to remove staff"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true, "message": "Staff member removed successfully"})
}
