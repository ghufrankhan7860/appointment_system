package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/authz"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/admin"
	"appointment_system/middleware"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	adminController *controller.AdminController
	tokenVerifier   middleware.TokenVerifier
}

func NewAdminHandler(
	adminController *controller.AdminController,
	tokenVerifier middleware.TokenVerifier,
) (*AdminHandler, error) {
	if adminController == nil {
		return nil, apperrors.ErrNilAdminController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}
	return &AdminHandler{
		adminController: adminController,
		tokenVerifier:   tokenVerifier,
	}, nil
}

func (ah *AdminHandler) RegisterRoutes(router *gin.RouterGroup) {
	adminRoutes := router.Group(
		"/admin",
		middleware.Authenticate(ah.tokenVerifier),
		middleware.LogAdminRequest(),
		authz.RequirePermission(authz.PermAdminAccess),
	)

	adminRoutes.POST("/users", authz.RequirePermission(authz.PermAdminManage), ah.CreateAdmin)
	adminRoutes.GET("/doctors", authz.RequirePermission(authz.PermDoctorVerify), ah.ListDoctors)
	adminRoutes.GET("/doctors/:id", authz.RequirePermission(authz.PermDoctorVerify), ah.GetDoctor)
	adminRoutes.POST("/doctors/:id/verify", authz.RequirePermission(authz.PermDoctorVerify), ah.VerifyDoctor)
	adminRoutes.POST("/doctors/:id/reject", authz.RequirePermission(authz.PermDoctorVerify), ah.RejectDoctor)
	adminRoutes.POST("/doctors/:id/suspend", authz.RequirePermission(authz.PermDoctorSuspend), ah.SuspendDoctor)
	adminRoutes.POST("/doctors/:id/reinstate", authz.RequirePermission(authz.PermDoctorSuspend), ah.ReinstateDoctor)
	adminRoutes.POST("/specialities", authz.RequirePermission(authz.PermSpecialityManage), ah.CreateSpeciality)
	adminRoutes.POST("/payments/:id/refund", authz.RequirePermission(authz.PermPaymentRefund), ah.RefundPayment)
	adminRoutes.GET("/refunds", authz.RequirePermission(authz.PermPaymentRefund), ah.ListRefunds)
	adminRoutes.GET("/records/:id/access-log", authz.RequirePermission(authz.PermRecordAudit), ah.ListRecordAccessLog)
}

// ListRecordAccessLog godoc
// @Summary List who accessed a medical record
// @Description Returns every create, edit, finalize, view and download of a record and its files, newest first, with the user and the request's correlation id. It never shows what the record says. Limit defaults to 20 and is capped at 100.
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param id path int true "Medical record ID"
// @Param limit query int false "Page size"
// @Param offset query int false "Number of entries to skip"
// @Success 200 {object} dtos.SuccessResponse{data=admin.ListAccessLogResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /admin/records/{id}/access-log [get]
func (ah *AdminHandler) ListRecordAccessLog(ctx *gin.Context) {
	var listAccessLogRequest admin.ListAccessLogRequest
	if err := ctx.ShouldBindUri(&listAccessLogRequest); err != nil || listAccessLogRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid medical record id"})
		return
	}
	if err := ctx.ShouldBindQuery(&listAccessLogRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ah.adminController.ListRecordAccessLog(ctx.Request.Context(), &listAccessLogRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrMedicalRecordNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Medical record not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list access log"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Access log retrieved successfully",
	})
}

// RefundPayment godoc
// @Summary Refund a payment
// @Description Refunds a captured payment in full through its gateway. The appointment's payment status becomes refunded. Cancelled paid appointments are refunded automatically; this is for everything else. Razorpay refunds at normal speed and finishes later, so it usually returns 202 with refundPending true and the payment still captured; its refund.processed webhook then marks it refunded. A refund Razorpay refuses returns 409 and is listed as failed in GET /admin/refunds; refunding the payment again retries it.
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Payment ID"
// @Param request body admin.RefundPaymentRequest false "Optional note"
// @Success 200 {object} dtos.SuccessResponse{data=admin.RefundPaymentResponse}
// @Success 202 {object} dtos.SuccessResponse{data=admin.RefundPaymentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /admin/payments/{id}/refund [post]
func (ah *AdminHandler) RefundPayment(ctx *gin.Context) {
	var refundPaymentRequest admin.RefundPaymentRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&refundPaymentRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&refundPaymentRequest); err != nil || refundPaymentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid payment id"})
		return
	}

	response, err := ah.adminController.RefundPayment(ctx.Request.Context(), &refundPaymentRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPaymentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Payment not found"})
		case errors.Is(err, apperrors.ErrPaymentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This payment cannot be refunded",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrRefundAlreadyExists):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "This payment already has a refund"})
		case errors.Is(err, apperrors.ErrRefundFailed):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "The payment partner refused this refund"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to refund payment"})
		}
		return
	}

	if response.RefundPending {
		ctx.JSON(http.StatusAccepted, gin.H{
			"success": true,
			"data":    response,
			"message": "Refund requested; the payment partner is processing it",
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Payment refunded successfully",
	})
}

// ListRefunds godoc
// @Summary List refunds
// @Description Returns refunds in one status, oldest first. The default, requested, lists refunds Razorpay is still processing; its refund webhook or make reconcile-refunds settles them. Failed refunds can be retried with POST /admin/payments/{id}/refund. Limit defaults to 20 and is capped at 100.
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param status query string false "Refund status" Enums(requested, succeeded, failed)
// @Param limit query int false "Page size"
// @Param offset query int false "Number of refunds to skip"
// @Success 200 {object} dtos.SuccessResponse{data=admin.ListRefundsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /admin/refunds [get]
func (ah *AdminHandler) ListRefunds(ctx *gin.Context) {
	var listRefundsRequest admin.ListRefundsRequest
	if err := ctx.ShouldBindQuery(&listRefundsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ah.adminController.ListRefunds(ctx.Request.Context(), &listRefundsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid query parameters",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list refunds"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Refunds retrieved successfully",
	})
}

// CreateAdmin godoc
// @Summary Create another admin
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body admin.CreateAdminRequest true "Admin account"
// @Success 201 {object} dtos.SuccessResponse{data=admin.CreateAdminResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /admin/users [post]
func (ah *AdminHandler) CreateAdmin(ctx *gin.Context) {
	var createAdminRequest admin.CreateAdminRequest
	if err := ctx.ShouldBindJSON(&createAdminRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ah.adminController.CreateAdmin(ctx.Request.Context(), &createAdminRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidAdminInput),
			errors.Is(err, apperrors.ErrNilAdminRequest):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid admin details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrUserAlreadyExists):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "User already exists",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create admin"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Admin created successfully",
	})
}

// ListDoctors godoc
// @Summary List doctors for review
// @Description Lists doctors in any status, oldest first. Use status=pending for the review queue.
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param status query string false "Verification status" Enums(pending, verified, rejected, suspended)
// @Param limit query int false "Page size, default 20, max 100"
// @Param offset query int false "Number of doctors to skip"
// @Success 200 {object} dtos.SuccessResponse{data=admin.ListDoctorsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /admin/doctors [get]
func (ah *AdminHandler) ListDoctors(ctx *gin.Context) {
	var listDoctorsRequest admin.ListDoctorsRequest
	if err := ctx.ShouldBindQuery(&listDoctorsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ah.adminController.ListDoctors(ctx.Request.Context(), &listDoctorsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidDoctorInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid query parameters",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list doctors"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Doctors retrieved successfully",
	})
}

// GetDoctor godoc
// @Summary Get a doctor for review
// @Description Returns a doctor in any status with its verification history, newest first.
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param id path int true "Doctor ID"
// @Success 200 {object} dtos.SuccessResponse{data=admin.GetDoctorResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /admin/doctors/{id} [get]
func (ah *AdminHandler) GetDoctor(ctx *gin.Context) {
	var getDoctorRequest admin.GetDoctorRequest
	if err := ctx.ShouldBindUri(&getDoctorRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		return
	}

	response, err := ah.adminController.GetDoctor(ctx.Request.Context(), &getDoctorRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidDoctorInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get doctor"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Doctor retrieved successfully",
	})
}

// VerifyDoctor godoc
// @Summary Verify a pending doctor
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Doctor ID"
// @Param request body admin.ReviewDoctorRequest false "Optional note"
// @Success 200 {object} dtos.SuccessResponse{data=admin.GetDoctorResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /admin/doctors/{id}/verify [post]
func (ah *AdminHandler) VerifyDoctor(ctx *gin.Context) {
	var reviewDoctorRequest admin.ReviewDoctorRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&reviewDoctorRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&reviewDoctorRequest); err != nil || reviewDoctorRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		return
	}

	response, err := ah.adminController.VerifyDoctor(ctx.Request.Context(), &reviewDoctorRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidDoctorInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		case errors.Is(err, apperrors.ErrVerificationReasonRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "A reason is required"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor not found"})
		case errors.Is(err, apperrors.ErrInvalidVerificationTransition):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This action is not allowed for the current verification status",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to verify doctor"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Doctor verified successfully",
	})
}

// RejectDoctor godoc
// @Summary Reject a pending doctor
// @Description A reason is required and is stored in the verification history. The doctor resubmits by updating the profile, which moves it back to pending.
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Doctor ID"
// @Param request body admin.ReviewDoctorRequest true "Reason"
// @Success 200 {object} dtos.SuccessResponse{data=admin.GetDoctorResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /admin/doctors/{id}/reject [post]
func (ah *AdminHandler) RejectDoctor(ctx *gin.Context) {
	var reviewDoctorRequest admin.ReviewDoctorRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&reviewDoctorRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&reviewDoctorRequest); err != nil || reviewDoctorRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		return
	}

	response, err := ah.adminController.RejectDoctor(ctx.Request.Context(), &reviewDoctorRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidDoctorInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		case errors.Is(err, apperrors.ErrVerificationReasonRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "A reason is required"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor not found"})
		case errors.Is(err, apperrors.ErrInvalidVerificationTransition):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This action is not allowed for the current verification status",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to reject doctor"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Doctor rejected successfully",
	})
}

// SuspendDoctor godoc
// @Summary Suspend a verified doctor
// @Description A reason is required. A suspended doctor disappears from search.
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Doctor ID"
// @Param request body admin.ReviewDoctorRequest true "Reason"
// @Success 200 {object} dtos.SuccessResponse{data=admin.GetDoctorResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /admin/doctors/{id}/suspend [post]
func (ah *AdminHandler) SuspendDoctor(ctx *gin.Context) {
	var reviewDoctorRequest admin.ReviewDoctorRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&reviewDoctorRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&reviewDoctorRequest); err != nil || reviewDoctorRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		return
	}

	response, err := ah.adminController.SuspendDoctor(ctx.Request.Context(), &reviewDoctorRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidDoctorInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		case errors.Is(err, apperrors.ErrVerificationReasonRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "A reason is required"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor not found"})
		case errors.Is(err, apperrors.ErrInvalidVerificationTransition):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This action is not allowed for the current verification status",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to suspend doctor"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Doctor suspended successfully",
	})
}

// ReinstateDoctor godoc
// @Summary Reinstate a suspended doctor
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Doctor ID"
// @Param request body admin.ReviewDoctorRequest false "Optional note"
// @Success 200 {object} dtos.SuccessResponse{data=admin.GetDoctorResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /admin/doctors/{id}/reinstate [post]
func (ah *AdminHandler) ReinstateDoctor(ctx *gin.Context) {
	var reviewDoctorRequest admin.ReviewDoctorRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&reviewDoctorRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&reviewDoctorRequest); err != nil || reviewDoctorRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		return
	}

	response, err := ah.adminController.ReinstateDoctor(ctx.Request.Context(), &reviewDoctorRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidDoctorInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid doctor id"})
		case errors.Is(err, apperrors.ErrVerificationReasonRequired):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "A reason is required"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor not found"})
		case errors.Is(err, apperrors.ErrInvalidVerificationTransition):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This action is not allowed for the current verification status",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to reinstate doctor"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Doctor reinstated successfully",
	})
}

// CreateSpeciality godoc
// @Summary Add a speciality
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body admin.CreateSpecialityRequest true "Speciality"
// @Success 201 {object} dtos.SuccessResponse{data=admin.CreateSpecialityResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /admin/specialities [post]
func (ah *AdminHandler) CreateSpeciality(ctx *gin.Context) {
	var createSpecialityRequest admin.CreateSpecialityRequest
	if err := ctx.ShouldBindJSON(&createSpecialityRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ah.adminController.CreateSpeciality(ctx.Request.Context(), &createSpecialityRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidDoctorInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid speciality details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrSpecialityAlreadyExists):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Speciality already exists"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create speciality"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Speciality created successfully",
	})
}
