package handler

import (
	"errors"
	"net/http"

	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/authz"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/desk"
	"appointment_system/middleware"

	"github.com/gin-gonic/gin"
)

// DeskHandler serves the clinic desk: the pass a patient shows there, and the
// doctor or their staff finding the patient, adding a walk-in and booking.
type DeskHandler struct {
	deskController *controller.DeskController
	tokenVerifier  middleware.TokenVerifier
}

func NewDeskHandler(deskController *controller.DeskController, tokenVerifier middleware.TokenVerifier) (*DeskHandler, error) {
	if deskController == nil {
		return nil, apperrors.ErrNilDeskController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}

	return &DeskHandler{deskController: deskController, tokenVerifier: tokenVerifier}, nil
}

func (dh *DeskHandler) RegisterRoutes(router *gin.RouterGroup) {
	authenticate := middleware.Authenticate(dh.tokenVerifier)
	book := authz.RequirePermission(authz.PermAppointmentBook)
	deskOnly := authz.RequirePermission(authz.PermAppointmentDesk)

	router.POST("/patients/:id/pass", authenticate, book, dh.IssuePass)

	deskRoutes := router.Group("/doctors/me/desk", authenticate, deskOnly)
	deskRoutes.POST("/lookup", dh.LookupPass)
	deskRoutes.POST("/patients", dh.CreateWalkIn)
	deskRoutes.POST("/patients/:id/claim-code", dh.CreateClaimCode)
	deskRoutes.POST("/appointments", dh.Book)
}

// IssuePass godoc
// @Summary Get a patient's clinic pass
// @Description Returns a new pass for a patient the caller can book for: a token for the QR the clinic desk scans and a short code it can type instead. It works for 10 minutes and replaces the patient's previous pass. Requires full or appointments_only access.
// @Tags desk
// @Security BearerAuth
// @Produce json
// @Param id path int true "Patient ID"
// @Success 201 {object} dtos.SuccessResponse{data=desk.PatientPassResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /patients/{id}/pass [post]
func (dh *DeskHandler) IssuePass(ctx *gin.Context) {
	var issuePassRequest desk.IssuePassRequest
	if err := ctx.ShouldBindUri(&issuePassRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}

	response, err := dh.deskController.IssuePass(ctx.Request.Context(), &issuePassRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Your access to this patient does not allow booking"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create clinic pass"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Clinic pass created successfully",
	})
}

// LookupPass godoc
// @Summary Find a patient by their clinic pass
// @Description For a doctor or their staff: finds the patient behind a scanned pass token or a typed pass code, and lets the desk book for them from now on. Wrong codes are limited per user.
// @Tags desk
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body desk.LookupPassRequest true "Pass token or code"
// @Success 200 {object} dtos.SuccessResponse{data=desk.DeskPatientResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 429 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/desk/lookup [post]
func (dh *DeskHandler) LookupPass(ctx *gin.Context) {
	var lookupPassRequest desk.LookupPassRequest
	if err := ctx.ShouldBindJSON(&lookupPassRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := dh.deskController.LookupPass(ctx.Request.Context(), &lookupPassRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidDeskInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only a doctor or their staff can use the clinic desk"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		case errors.Is(err, apperrors.ErrPatientPassInvalid):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient code is invalid or expired"})
		case errors.Is(err, apperrors.ErrPatientPassAttemptsExceeded):
			ctx.JSON(http.StatusTooManyRequests, gin.H{"success": false, "error": "Too many wrong patient codes; try again later"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to find patient"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Patient found",
	})
}

// CreateWalkIn godoc
// @Summary Add a walk-in patient
// @Description For a doctor or their staff: adds a patient who has no app, so the desk can book for them. Nobody holds the patient until someone redeems a claim code from POST /doctors/me/desk/patients/{id}/claim-code.
// @Tags desk
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body desk.CreateWalkInRequest true "Patient details"
// @Success 201 {object} dtos.SuccessResponse{data=desk.DeskPatientResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/desk/patients [post]
func (dh *DeskHandler) CreateWalkIn(ctx *gin.Context) {
	var createWalkInRequest desk.CreateWalkInRequest
	if err := ctx.ShouldBindJSON(&createWalkInRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid patient details",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := dh.deskController.CreateWalkIn(ctx.Request.Context(), &createWalkInRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid patient details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only a doctor or their staff can use the clinic desk"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to add walk-in patient"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Walk-in patient added successfully",
	})
}

// CreateClaimCode godoc
// @Summary Create a claim code for a walk-in
// @Description For a doctor or their staff: a one-time code with which the walk-in, or whoever looks after them, takes the patient over in the app (at /claim). Only for a walk-in this desk added that nobody holds yet. Works for 48 hours.
// @Tags desk
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Patient ID"
// @Param request body desk.CreateClaimCodeRequest true "Redeemer's relationship to the patient"
// @Success 201 {object} dtos.SuccessResponse{data=desk.ClaimCodeResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/desk/patients/{id}/claim-code [post]
func (dh *DeskHandler) CreateClaimCode(ctx *gin.Context) {
	var createClaimCodeRequest desk.CreateClaimCodeRequest
	if err := ctx.ShouldBindJSON(&createClaimCodeRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&createClaimCodeRequest); err != nil || createClaimCodeRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid patient id"})
		return
	}

	response, err := dh.deskController.CreateClaimCode(ctx.Request.Context(), &createClaimCodeRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPatientInviteInput), errors.Is(err, apperrors.ErrInvalidDeskInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only a doctor or their staff can use the clinic desk"})
		case errors.Is(err, apperrors.ErrDoctorNotFound), errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		case errors.Is(err, apperrors.ErrPatientAlreadyClaimed),
			errors.Is(err, apperrors.ErrPatientInviteLimitReached),
			errors.Is(err, apperrors.ErrPatientSelfLinkExists):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Cannot create a claim code",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create claim code"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Claim code created successfully",
	})
}

// Book godoc
// @Summary Book at the clinic desk
// @Description For a doctor or their staff: books a free slot of the desk's doctor for a patient the desk has seen (pass shown, walk-in added, or already in the doctor's patient list). The fee is paid at the clinic, so the booking is confirmed at once (paymentStatus due_at_clinic, or waived without a fee) and bookedVia is desk. Send an Idempotency-Key to retry safely.
// @Tags desk
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param Idempotency-Key header string false "Client key; a retry with the same key and body returns the first booking"
// @Param request body desk.BookAtDeskRequest true "Booking details"
// @Success 201 {object} dtos.SuccessResponse{data=appointment.AppointmentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 422 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/desk/appointments [post]
func (dh *DeskHandler) Book(ctx *gin.Context) {
	var bookAtDeskRequest desk.BookAtDeskRequest
	if err := ctx.ShouldBindJSON(&bookAtDeskRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid appointment details",
			"details": bindErrorDetail(err),
		})
		return
	}
	bookAtDeskRequest.IdempotencyKey = ctx.GetHeader(constant.IdempotencyKeyHeader)

	response, err := dh.deskController.Book(ctx.Request.Context(), &bookAtDeskRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidAppointmentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid appointment details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only a doctor or their staff can use the clinic desk"})
		case errors.Is(err, apperrors.ErrPatientNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Patient not found"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor not found"})
		case errors.Is(err, apperrors.ErrSlotNotAvailable):
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"error":   "Selected time is not available",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrSlotTaken):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Selected time is not available",
				"details": "that time was just booked by someone else",
			})
		case errors.Is(err, apperrors.ErrIdempotencyKeyReused):
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"error":   "Idempotency-Key already used",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to book appointment"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Appointment booked successfully",
	})
}
