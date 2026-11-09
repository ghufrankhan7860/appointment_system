package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/patient"
	"appointment_system/middleware"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type PatientHandler struct {
	patientController *controller.PatientController
	tokenVerifier     middleware.TokenVerifier
}

func NewPatientHandler(
	patientController *controller.PatientController,
	tokenVerifier middleware.TokenVerifier,
) (*PatientHandler, error) {
	if patientController == nil {
		return nil, apperrors.ErrNilPatientController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}
	return &PatientHandler{
		patientController: patientController,
		tokenVerifier:     tokenVerifier,
	}, nil
}

func (ph *PatientHandler) RegisterRoutes(router *gin.RouterGroup) {
	patients := router.Group("/patients", middleware.Authenticate(ph.tokenVerifier))
	patients.POST("", ph.Create)
	patients.GET("", ph.List)
	patients.GET("/:id", ph.GetByID)
	patients.PUT("/:id", ph.Update)
	patients.POST("/claim", ph.Claim)
	patients.POST("/:id/invites", ph.CreateInvite)
	patients.GET("/:id/invites", ph.ListInvites)
	patients.DELETE("/:id/invites/:inviteId", ph.RevokeInvite)
	patients.GET("/:id/members", ph.ListMembers)
	patients.PATCH("/:id/members/:userId", ph.UpdateMember)
	patients.DELETE("/:id/members/:userId", ph.RevokeMember)
	patients.DELETE("/:id/membership", ph.Leave)
}

// Create godoc
// @Summary Create a patient
// @Description Creates a patient and links it to the caller with full access.
// @Tags patients
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body patient.CreatePatientRequest true "Patient details"
// @Success 201 {object} dtos.SuccessResponse{data=patient.CreatePatientResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients [post]
func (ph *PatientHandler) Create(ctx *gin.Context) {
	var createPatientRequest patient.CreatePatientRequest
	if err := ctx.ShouldBindJSON(&createPatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ph.patientController.Create(ctx.Request.Context(), &createPatientRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput),
			errors.Is(err, apperrors.ErrNilPatientRequest):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid patient details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrUserPatientAlreadyExists):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Patient already linked to user"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create patient"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Patient created successfully",
	})
}

// GetByID godoc
// @Summary Get a patient
// @Description Returns a patient the caller has active access to. Other patients are reported as not found.
// @Tags patients
// @Security BearerAuth
// @Produce json
// @Param id path int true "Patient ID"
// @Success 200 {object} dtos.SuccessResponse{data=patient.GetPatientResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id} [get]
func (ph *PatientHandler) GetByID(ctx *gin.Context) {
	var getPatientRequest patient.GetPatientRequest
	if err := ctx.ShouldBindUri(&getPatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}

	response, err := ph.patientController.GetByID(ctx.Request.Context(), &getPatientRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get patient"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Patient retrieved successfully",
	})
}

// List godoc
// @Summary List patients
// @Description Lists the patients the caller has active access to. Limit defaults to 20 and is capped at 100.
// @Tags patients
// @Security BearerAuth
// @Produce json
// @Param limit query int false "Page size"
// @Param offset query int false "Number of patients to skip"
// @Success 200 {object} dtos.SuccessResponse{data=patient.ListPatientsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients [get]
func (ph *PatientHandler) List(ctx *gin.Context) {
	var listPatientsRequest patient.ListPatientsRequest
	if err := ctx.ShouldBindQuery(&listPatientsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ph.patientController.List(ctx.Request.Context(), &listPatientsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid query parameters"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list patients"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Patients retrieved successfully",
	})
}

// Update godoc
// @Summary Update a patient
// @Description Updates a patient. Requires full access to the patient.
// @Tags patients
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Patient ID"
// @Param request body patient.UpdatePatientRequest true "Patient details"
// @Success 200 {object} dtos.SuccessResponse{data=patient.UpdatePatientResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id} [put]
func (ph *PatientHandler) Update(ctx *gin.Context) {
	var updatePatientRequest patient.UpdatePatientRequest
	if err := ctx.ShouldBindJSON(&updatePatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&updatePatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}

	response, err := ph.patientController.Update(ctx.Request.Context(), &updatePatientRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput),
			errors.Is(err, apperrors.ErrNilPatientRequest):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid patient details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Patient access forbidden"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to update patient"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Patient updated successfully",
	})
}

// CreateInvite godoc
// @Summary Invite someone to a patient
// @Description Creates a one-time invite code for a patient the caller holds with full access. Whoever redeems it gets the given relationship and access level. The code is returned only in this response. A self invite must grant full access and is refused while the patient has a self link. At most 5 invites can be pending; expiresInHours defaults to 48.
// @Tags patients
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Patient ID"
// @Param request body patient.CreatePatientInviteRequest true "Invite details"
// @Success 201 {object} dtos.SuccessResponse{data=patient.PatientInviteResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id}/invites [post]
func (ph *PatientHandler) CreateInvite(ctx *gin.Context) {
	var createPatientInviteRequest patient.CreatePatientInviteRequest
	if err := ctx.ShouldBindJSON(&createPatientInviteRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&createPatientInviteRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}

	response, err := ph.patientController.CreateInvite(ctx.Request.Context(), &createPatientInviteRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInviteInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid invite details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Patient access forbidden"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		case errors.Is(err, apperrors.ErrPatientSelfLinkExists):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Patient already has a self link"})
		case errors.Is(err, apperrors.ErrPatientInviteLimitReached):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Patient has too many pending invites"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create invite"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Invite created successfully",
	})
}

// ListInvites godoc
// @Summary List a patient's pending invites
// @Description Lists the patient's invites that are pending and not expired, without their codes. Requires full access to the patient.
// @Tags patients
// @Security BearerAuth
// @Produce json
// @Param id path int true "Patient ID"
// @Success 200 {object} dtos.SuccessResponse{data=patient.ListPatientInvitesResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id}/invites [get]
func (ph *PatientHandler) ListInvites(ctx *gin.Context) {
	var listPatientInvitesRequest patient.ListPatientInvitesRequest
	if err := ctx.ShouldBindUri(&listPatientInvitesRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}

	response, err := ph.patientController.ListInvites(ctx.Request.Context(), &listPatientInvitesRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Patient access forbidden"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list invites"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Invites retrieved successfully",
	})
}

// RevokeInvite godoc
// @Summary Revoke a pending invite
// @Description Revokes a pending invite so its code stops working. Requires full access to the patient. An invite that is not pending is reported as not found.
// @Tags patients
// @Security BearerAuth
// @Produce json
// @Param id path int true "Patient ID"
// @Param inviteId path int true "Invite ID"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id}/invites/{inviteId} [delete]
func (ph *PatientHandler) RevokeInvite(ctx *gin.Context) {
	var revokePatientInviteRequest patient.RevokePatientInviteRequest
	if err := ctx.ShouldBindUri(&revokePatientInviteRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid invite id"})
		return
	}

	if err := ph.patientController.RevokeInvite(ctx.Request.Context(), &revokePatientInviteRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Patient access forbidden"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		case errors.Is(err, apperrors.ErrPatientInviteNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Invite not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to revoke invite"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Invite revoked successfully",
	})
}

// Claim godoc
// @Summary Claim a patient with an invite code
// @Description Redeems an invite code and links the caller to its patient with the invite's relationship and access level; the patient's appointments and records follow. Codes are case-insensitive and may contain spaces or dashes. An unknown, used, revoked or expired code is one 404. After 5 wrong codes in an hour the caller gets 429 until the hour passes.
// @Tags patients
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body patient.ClaimPatientRequest true "Invite code"
// @Success 200 {object} dtos.SuccessResponse{data=patient.GetPatientResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 429 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/claim [post]
func (ph *PatientHandler) Claim(ctx *gin.Context) {
	var claimPatientRequest patient.ClaimPatientRequest
	if err := ctx.ShouldBindJSON(&claimPatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ph.patientController.Claim(ctx.Request.Context(), &claimPatientRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInviteInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid invite code",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPatientInviteInvalid):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Invite code is invalid or expired"})
		case errors.Is(err, apperrors.ErrUserPatientAlreadyExists):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Patient already linked to user"})
		case errors.Is(err, apperrors.ErrPatientSelfLinkExists):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Patient already has a self link"})
		case errors.Is(err, apperrors.ErrPatientInviteAttemptsExceeded):
			ctx.JSON(http.StatusTooManyRequests, gin.H{"success": false, "error": "Too many wrong invite codes, try again later"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to claim patient"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Patient claimed successfully",
	})
}

// ListMembers godoc
// @Summary List who has access to a patient
// @Description Lists every user with active access to the patient, with a masked mobile number. Current marks the caller. Requires full access to the patient.
// @Tags patients
// @Security BearerAuth
// @Produce json
// @Param id path int true "Patient ID"
// @Success 200 {object} dtos.SuccessResponse{data=patient.ListPatientMembersResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id}/members [get]
func (ph *PatientHandler) ListMembers(ctx *gin.Context) {
	var listPatientMembersRequest patient.ListPatientMembersRequest
	if err := ctx.ShouldBindUri(&listPatientMembersRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}

	response, err := ph.patientController.ListMembers(ctx.Request.Context(), &listPatientMembersRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Patient access forbidden"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list members"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Members retrieved successfully",
	})
}

// UpdateMember godoc
// @Summary Change a member's access
// @Description Changes another member's access level. Requires full access to the patient. A self link cannot be changed, your own access cannot be changed, and the last full holder cannot be downgraded.
// @Tags patients
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Patient ID"
// @Param userId path int true "Member user ID"
// @Param request body patient.UpdatePatientMemberRequest true "New access level"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id}/members/{userId} [patch]
func (ph *PatientHandler) UpdateMember(ctx *gin.Context) {
	var updatePatientMemberRequest patient.UpdatePatientMemberRequest
	if err := ctx.ShouldBindJSON(&updatePatientMemberRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&updatePatientMemberRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid member id"})
		return
	}

	if err := ph.patientController.UpdateMember(ctx.Request.Context(), &updatePatientMemberRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid member details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Patient access forbidden"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		case errors.Is(err, apperrors.ErrUserPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Member not found"})
		case errors.Is(err, apperrors.ErrPatientLastFullHolder):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Patient must keep at least one full access holder"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to update member"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Member updated successfully",
	})
}

// RevokeMember godoc
// @Summary Remove a member's access
// @Description Revokes another member's access. Requires full access to the patient. A self link cannot be revoked by anyone else, your own access is removed with leave, and the last full holder cannot be removed. Past appointments are kept.
// @Tags patients
// @Security BearerAuth
// @Produce json
// @Param id path int true "Patient ID"
// @Param userId path int true "Member user ID"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id}/members/{userId} [delete]
func (ph *PatientHandler) RevokeMember(ctx *gin.Context) {
	var revokePatientMemberRequest patient.RevokePatientMemberRequest
	if err := ctx.ShouldBindUri(&revokePatientMemberRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid member id"})
		return
	}

	if err := ph.patientController.RevokeMember(ctx.Request.Context(), &revokePatientMemberRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid member",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Patient access forbidden"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		case errors.Is(err, apperrors.ErrUserPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Member not found"})
		case errors.Is(err, apperrors.ErrPatientLastFullHolder):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Patient must keep at least one full access holder"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to revoke member"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Member revoked successfully",
	})
}

// Leave godoc
// @Summary Leave a patient
// @Description Removes the caller's own access to the patient. The last full holder cannot leave until someone else has full access. Past appointments are kept.
// @Tags patients
// @Security BearerAuth
// @Produce json
// @Param id path int true "Patient ID"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id}/membership [delete]
func (ph *PatientHandler) Leave(ctx *gin.Context) {
	var leavePatientRequest patient.LeavePatientRequest
	if err := ctx.ShouldBindUri(&leavePatientRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}

	if err := ph.patientController.Leave(ctx.Request.Context(), &leavePatientRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		case errors.Is(err, apperrors.ErrPatientLastFullHolder):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Patient must keep at least one full access holder"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to leave patient"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Patient left successfully",
	})
}
