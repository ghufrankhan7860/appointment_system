package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/authz"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/payment"
	"appointment_system/middleware"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type PaymentHandler struct {
	paymentController *controller.PaymentController
	tokenVerifier     middleware.TokenVerifier
}

func NewPaymentHandler(
	paymentController *controller.PaymentController,
	tokenVerifier middleware.TokenVerifier,
) (*PaymentHandler, error) {
	if paymentController == nil {
		return nil, apperrors.ErrNilPaymentController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}
	return &PaymentHandler{
		paymentController: paymentController,
		tokenVerifier:     tokenVerifier,
	}, nil
}

func (ph *PaymentHandler) RegisterRoutes(router *gin.RouterGroup) {
	authenticate := middleware.Authenticate(ph.tokenVerifier)
	pay := authz.RequirePermission(authz.PermPaymentPay)

	router.POST("/appointments/:id/payments", authenticate, pay, ph.Create)
	router.GET("/appointments/:id/payment", authenticate, ph.GetForAppointment)
	router.POST("/payments/:id/verify", authenticate, pay, ph.Verify)
}

// Create godoc
// @Summary Pay for an appointment
// @Description Opens a gateway order for an unpaid appointment and returns what the client needs to pay it. Anyone with full or appointments_only access to the patient can pay. The amount is the fee stored on the appointment. Asking again while a payment is open returns the same one. The booking must be paid before paymentDueAt, 10 minutes after booking, or it is cancelled.
// @Tags payments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 201 {object} dtos.SuccessResponse{data=payment.CreatePaymentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/payments [post]
func (ph *PaymentHandler) Create(ctx *gin.Context) {
	var createPaymentRequest payment.CreatePaymentRequest
	if err := ctx.ShouldBindUri(&createPaymentRequest); err != nil || createPaymentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ph.paymentController.Create(ctx.Request.Context(), &createPaymentRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Your access to this patient does not allow paying"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrPaymentNotRequired):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Nothing to pay for this appointment",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPaymentWindowClosed):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Payment window has closed",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create payment"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Payment created successfully",
	})
}

// GetForAppointment godoc
// @Summary Get an appointment's payment
// @Description Returns the appointment's most recent payment. The patient side with any active link, the appointment's doctor and admins can see it.
// @Tags payments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 200 {object} dtos.SuccessResponse{data=payment.PaymentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/payment [get]
func (ph *PaymentHandler) GetForAppointment(ctx *gin.Context) {
	var getAppointmentPaymentRequest payment.GetAppointmentPaymentRequest
	if err := ctx.ShouldBindUri(&getAppointmentPaymentRequest); err != nil || getAppointmentPaymentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ph.paymentController.GetForAppointment(ctx.Request.Context(), &getAppointmentPaymentRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrPaymentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "No payment for this appointment"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get payment"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Payment retrieved successfully",
	})
}

// Verify godoc
// @Summary Confirm a payment
// @Description Confirms with the gateway that the order was paid, captures the payment and marks the appointment paid. Verifying a captured payment again with the same providerPaymentId returns it. If the booking expired first, the money is refunded and 409 is returned.
// @Tags payments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Payment ID"
// @Param request body payment.VerifyPaymentRequest true "Gateway proof of payment"
// @Success 200 {object} dtos.SuccessResponse{data=payment.PaymentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 422 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /payments/{id}/verify [post]
func (ph *PaymentHandler) Verify(ctx *gin.Context) {
	var verifyPaymentRequest payment.VerifyPaymentRequest
	if err := ctx.ShouldBindJSON(&verifyPaymentRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&verifyPaymentRequest); err != nil || verifyPaymentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid payment id"})
		return
	}

	response, err := ph.paymentController.Verify(ctx.Request.Context(), &verifyPaymentRequest)
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
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Your access to this patient does not allow paying"})
		case errors.Is(err, apperrors.ErrPaymentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Payment not found"})
		case errors.Is(err, apperrors.ErrPaymentVerificationFailed):
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"error":   "Payment could not be verified",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPaymentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This payment can no longer be confirmed",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPaymentWindowClosed):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Payment window has closed",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to verify payment"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Payment confirmed successfully",
	})
}
