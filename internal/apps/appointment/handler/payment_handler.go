package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/authz"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/payment"
	"appointment_system/middleware"
	"errors"
	"io"
	"log"
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
	router.POST("/webhooks/payment", ph.HandleWebhook)
}

// HandleWebhook godoc
// @Summary Receive a payment gateway callback
// @Description Called by Razorpay, never by clients, so it takes no token. Subscribe it to payment.captured, order.paid, refund.processed and refund.failed. The X-Razorpay-Signature HMAC must match the raw body, and a reported payment is confirmed with Razorpay's API again before any money is recorded. Repeats, other events and orders this service did not open get 200 and change nothing; money for a booking that was already cancelled is recorded and refunded. Only a bad signature gets 401; a 5xx makes Razorpay retry. The body is limited to 64 KB.
// @Tags payments
// @Accept json
// @Produce json
// @Param X-Razorpay-Signature header string true "hex HMAC-SHA256 of the raw body, keyed with the webhook secret"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 413 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /webhooks/payment [post]
func (ph *PaymentHandler) HandleWebhook(ctx *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, constant.MaxWebhookBodyBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "error": "Webhook body is too large"})
			return
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid webhook body"})
		return
	}

	if err := ph.paymentController.HandleWebhook(ctx.Request.Context(), body, ctx.Request.Header); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPaymentVerificationFailed):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Webhook could not be verified"})
		case errors.Is(err, apperrors.ErrWebhookIgnored):
			// Answered 200 so the gateway does not retry or disable the
			// webhook. The reason names ids only; the body is never logged.
			log.Printf("payment webhook ignored: %s", apperrors.Reason(err))
			ctx.JSON(http.StatusOK, gin.H{"success": true, "received": true, "message": "Webhook ignored"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to handle webhook"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success":  true,
		"received": true,
		"message":  "Webhook received",
	})
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
// @Description Confirms with the gateway that the order was paid, captures the payment and marks the appointment paid. With Razorpay, send Checkout's razorpay_payment_id as providerPaymentId and razorpay_signature as signature; the server checks the signature against the stored order and fetches the payment from Razorpay. With none, send any providerPaymentId and the signature from clientPayload. Verifying a captured payment again with the same providerPaymentId returns it, so a webhook that captured it first is fine. If the booking expired first, the money is refunded and 409 is returned.
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
