package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/authz"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/doctor"
	"appointment_system/internal/apps/appointment/dtos/payment"
	"appointment_system/middleware"
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type PaymentHandler struct {
	paymentController *controller.PaymentController
	tokenVerifier     middleware.TokenVerifier
	logger            *slog.Logger
}

func NewPaymentHandler(
	paymentController *controller.PaymentController,
	tokenVerifier middleware.TokenVerifier,
	logger *slog.Logger,
) (*PaymentHandler, error) {
	if paymentController == nil {
		return nil, apperrors.ErrNilPaymentController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}
	return &PaymentHandler{
		paymentController: paymentController,
		tokenVerifier:     tokenVerifier,
		logger:            logger,
	}, nil
}

func (ph *PaymentHandler) RegisterRoutes(router *gin.RouterGroup) {
	authenticate := middleware.Authenticate(ph.tokenVerifier)
	pay := authz.RequirePermission(authz.PermPaymentPay)
	verify := authz.RequirePermission(authz.PermPaymentVerify)
	manageProfile := authz.RequirePermission(authz.PermDoctorProfileManage)

	router.POST("/appointments/:id/payments", authenticate, pay, ph.Create)
	router.GET("/appointments/:id/payment", authenticate, ph.GetForAppointment)
	router.POST("/payments/:id/verify", authenticate, pay, ph.Verify)
	router.GET("/appointments/:id/payment-options", authenticate, pay, ph.Options)
	router.POST("/appointments/:id/payment/clinic", authenticate, pay, ph.PayAtClinic)
	router.GET("/appointments/:id/payment/upi-qr", authenticate, pay, ph.AppointmentUPIQR)
	router.POST("/payments/:id/claim", authenticate, pay, ph.Claim)
	router.GET("/payments/:id/proof", authenticate, ph.Proof)

	doctorMe := router.Group("/doctors/me", authenticate)
	doctorMe.PUT("/upi", manageProfile, ph.UpdateUPI)
	doctorMe.GET("/upi-qr", manageProfile, ph.OwnUPIQR)
	doctorMe.PUT("/upi-qr", manageProfile, ph.UpdateUPIQR)
	doctorMe.DELETE("/upi-qr", manageProfile, ph.DeleteUPIQR)
	doctorMe.GET("/payments/claims", verify, ph.ListClaims)
	doctorMe.POST("/payments/:id/verify", verify, ph.VerifyClaim)
	doctorMe.POST("/payments/:id/reject", verify, ph.RejectClaim)
	doctorMe.POST("/appointments/:id/collected", verify, ph.MarkCollected)
	doctorMe.GET("/refunds", verify, ph.ListManualRefunds)
	doctorMe.POST("/refunds/:id/close", verify, ph.CloseRefund)
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
			ph.logger.WarnContext(ctx.Request.Context(), "payment webhook ignored", "reason", apperrors.Reason(err))
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
// @Description Opens a payment for an unpaid appointment and returns what the client needs to pay it: a gateway order for method razorpay (the default), or a direct UPI payment to the doctor for method upi, which the payer then claims with POST /payments/{id}/claim. The method must be one of GET /appointments/{id}/payment-options; an open payment by the other method is closed. Anyone with full or appointments_only access to the patient can pay. The amount is the fee stored on the appointment. Asking again while a payment is open returns the same one. The booking must be paid before paymentDueAt, 10 minutes after booking, or it is cancelled.
// @Tags payments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Param request body payment.CreatePaymentRequest false "Payment method"
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
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&createPaymentRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
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
		case errors.Is(err, apperrors.ErrPaymentMethodUnavailable):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "This payment method is not available for this booking"})
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

// Options godoc
// @Summary How an appointment can be paid
// @Description Lists the payment methods the booking offers, as PAYMENT_METHODS and the doctor allow: razorpay, upi (when the doctor set a UPI ID, with their details) and clinic. When no online method can take the money, clinic is the only one.
// @Tags payments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 200 {object} dtos.SuccessResponse{data=payment.PaymentOptionsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/payment-options [get]
func (ph *PaymentHandler) Options(ctx *gin.Context) {
	var getPaymentOptionsRequest payment.GetPaymentOptionsRequest
	if err := ctx.ShouldBindUri(&getPaymentOptionsRequest); err != nil || getPaymentOptionsRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ph.paymentController.Options(ctx.Request.Context(), &getPaymentOptionsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get payment options"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Payment options retrieved successfully",
	})
}

// PayAtClinic godoc
// @Summary Pay for an appointment at the clinic
// @Description Confirms an unpaid booking now, with its fee due at the clinic; the doctor marks it collected at the visit. Only when clinic is one of the booking's payment options. An open online payment is closed.
// @Tags payments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/payment/clinic [post]
func (ph *PaymentHandler) PayAtClinic(ctx *gin.Context) {
	var payAtClinicRequest payment.PayAtClinicRequest
	if err := ctx.ShouldBindUri(&payAtClinicRequest); err != nil || payAtClinicRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	if err := ph.paymentController.PayAtClinic(ctx.Request.Context(), &payAtClinicRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Your access to this patient does not allow paying"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrPaymentMethodUnavailable):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Paying at the clinic is not available for this booking"})
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
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to choose pay at clinic"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Booking confirmed; pay at the clinic",
	})
}

// Claim godoc
// @Summary Say a direct UPI payment was made
// @Description Records the UPI reference (UTR) of a direct UPI payment to the doctor, with an optional JPEG or PNG screenshot (proof, up to 5 MB), before the booking's paymentDueAt. The booking then waits for the doctor's check until the appointment starts, and is cancelled if nobody checks it by then.
// @Tags payments
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param id path int true "Payment ID"
// @Param reference formData string true "UPI reference (UTR), 6 to 35 letters or digits"
// @Param proof formData file false "Screenshot of the payment, JPEG or PNG"
// @Success 200 {object} dtos.SuccessResponse{data=payment.PaymentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 413 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /payments/{id}/claim [post]
func (ph *PaymentHandler) Claim(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, constant.MaxPaymentImageBytes+constant.MultipartOverheadBytes)

	var claimPaymentRequest payment.ClaimPaymentRequest
	if err := ctx.ShouldBind(&claimPaymentRequest); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "error": "Screenshot is too large"})
			return
		}
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid payment claim",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&claimPaymentRequest); err != nil || claimPaymentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid payment id"})
		return
	}

	var proof io.Reader
	if fileHeader, err := ctx.FormFile(constant.PaymentProofFormField); err == nil {
		file, err := fileHeader.Open()
		if err != nil {
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to read upload"})
			return
		}
		defer file.Close()
		proof = file
	}

	response, err := ph.paymentController.Claim(ctx.Request.Context(), &claimPaymentRequest, proof)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid payment claim",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPaymentNotFound),
			errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Payment not found"})
		case errors.Is(err, apperrors.ErrPaymentNotModifiable),
			errors.Is(err, apperrors.ErrPaymentNotRequired),
			errors.Is(err, apperrors.ErrPaymentWindowClosed):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Payment can no longer be claimed",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to claim payment"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Payment sent to the doctor for a check",
	})
}

// Proof godoc
// @Summary Download a direct UPI payment's screenshot
// @Description Streams the screenshot the payer sent with their claim, to the appointment's doctor, an admin or the patient side. Anyone else gets 404.
// @Tags payments
// @Security BearerAuth
// @Produce image/jpeg
// @Produce image/png
// @Param id path int true "Payment ID"
// @Success 200 {file} file
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /payments/{id}/proof [get]
func (ph *PaymentHandler) Proof(ctx *gin.Context) {
	var getPaymentImageRequest payment.GetPaymentImageRequest
	if err := ctx.ShouldBindUri(&getPaymentImageRequest); err != nil || getPaymentImageRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid payment id"})
		return
	}

	file, err := ph.paymentController.OpenProof(ctx.Request.Context(), &getPaymentImageRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have a UPI QR"})
		case errors.Is(err, apperrors.ErrPaymentNotFound),
			errors.Is(err, apperrors.ErrAppointmentNotFound),
			errors.Is(err, apperrors.ErrDoctorNotFound),
			errors.Is(err, apperrors.ErrPaymentImageNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Image not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to download image"})
		}
		return
	}
	defer file.Close()

	head := make([]byte, constant.ContentSniffBytes)
	read, readErr := io.ReadFull(file, head)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		_ = ctx.Error(readErr)
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to download image"})
		return
	}
	ctx.DataFromReader(http.StatusOK, -1, http.DetectContentType(head[:read]), io.MultiReader(bytes.NewReader(head[:read]), file), map[string]string{
		"Cache-Control":          "private, no-store",
		"X-Content-Type-Options": "nosniff",
	})
}

// AppointmentUPIQR godoc
// @Summary Download the doctor's UPI QR for an appointment
// @Description Streams the QR image the appointment's doctor uploaded, to its patient side. 404 when the doctor has none.
// @Tags payments
// @Security BearerAuth
// @Produce image/jpeg
// @Produce image/png
// @Param id path int true "Appointment ID"
// @Success 200 {file} file
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/payment/upi-qr [get]
func (ph *PaymentHandler) AppointmentUPIQR(ctx *gin.Context) {
	var getPaymentImageRequest payment.GetPaymentImageRequest
	if err := ctx.ShouldBindUri(&getPaymentImageRequest); err != nil || getPaymentImageRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	file, err := ph.paymentController.OpenUPIQR(ctx.Request.Context(), &getPaymentImageRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have a UPI QR"})
		case errors.Is(err, apperrors.ErrPaymentNotFound),
			errors.Is(err, apperrors.ErrAppointmentNotFound),
			errors.Is(err, apperrors.ErrDoctorNotFound),
			errors.Is(err, apperrors.ErrPaymentImageNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Image not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to download image"})
		}
		return
	}
	defer file.Close()

	head := make([]byte, constant.ContentSniffBytes)
	read, readErr := io.ReadFull(file, head)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		_ = ctx.Error(readErr)
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to download image"})
		return
	}
	ctx.DataFromReader(http.StatusOK, -1, http.DetectContentType(head[:read]), io.MultiReader(bytes.NewReader(head[:read]), file), map[string]string{
		"Cache-Control":          "private, no-store",
		"X-Content-Type-Options": "nosniff",
	})
}

// OwnUPIQR godoc
// @Summary Download my UPI QR
// @Description The QR image the calling doctor uploaded. 404 when there is none.
// @Tags doctors
// @Security BearerAuth
// @Produce image/jpeg
// @Produce image/png
// @Success 200 {file} file
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/upi-qr [get]
func (ph *PaymentHandler) OwnUPIQR(ctx *gin.Context) {
	file, err := ph.paymentController.OpenUPIQR(ctx.Request.Context(), nil)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have a UPI QR"})
		case errors.Is(err, apperrors.ErrPaymentNotFound),
			errors.Is(err, apperrors.ErrAppointmentNotFound),
			errors.Is(err, apperrors.ErrDoctorNotFound),
			errors.Is(err, apperrors.ErrPaymentImageNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Image not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to download image"})
		}
		return
	}
	defer file.Close()

	head := make([]byte, constant.ContentSniffBytes)
	read, readErr := io.ReadFull(file, head)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		_ = ctx.Error(readErr)
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to download image"})
		return
	}
	ctx.DataFromReader(http.StatusOK, -1, http.DetectContentType(head[:read]), io.MultiReader(bytes.NewReader(head[:read]), file), map[string]string{
		"Cache-Control":          "private, no-store",
		"X-Content-Type-Options": "nosniff",
	})
}

// UpdateUPI godoc
// @Summary Set where patients pay me by UPI
// @Description Sets the calling doctor's UPI ID (name@bank), used for direct UPI payments when PAYMENT_METHODS includes upi. An empty upiId stops them and removes the QR.
// @Tags doctors
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body doctor.UpdateUPIRequest true "UPI ID"
// @Success 200 {object} dtos.SuccessResponse{data=doctor.DoctorResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/upi [put]
func (ph *PaymentHandler) UpdateUPI(ctx *gin.Context) {
	var updateUPIRequest doctor.UpdateUPIRequest
	if err := ctx.ShouldBindJSON(&updateUPIRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ph.paymentController.UpdateUPI(ctx.Request.Context(), &updateUPIRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidDoctorInput),
			errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid UPI details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts take UPI payments"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to update UPI"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "UPI updated successfully",
	})
}

// UpdateUPIQR godoc
// @Summary Upload my UPI QR
// @Description Stores the calling doctor's UPI QR image (file, JPEG or PNG, up to 5 MB), replacing the old one. The UPI ID must be set first.
// @Tags doctors
// @Security BearerAuth
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "QR image, JPEG or PNG"
// @Success 200 {object} dtos.SuccessResponse{data=doctor.DoctorResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 413 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/upi-qr [put]
func (ph *PaymentHandler) UpdateUPIQR(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, constant.MaxPaymentImageBytes+constant.MultipartOverheadBytes)

	fileHeader, err := ctx.FormFile(constant.UPIQRFormField)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "error": "QR image is too large"})
			return
		}
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid upload",
			"details": "file is required",
		})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		_ = ctx.Error(err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to read upload"})
		return
	}
	defer file.Close()

	response, err := ph.paymentController.SetUPIQR(ctx.Request.Context(), file)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidDoctorInput),
			errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid UPI details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts take UPI payments"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to upload UPI QR"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "UPI QR uploaded successfully",
	})
}

// DeleteUPIQR godoc
// @Summary Remove my UPI QR
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Success 200 {object} dtos.SuccessResponse{data=doctor.DoctorResponse}
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/upi-qr [delete]
func (ph *PaymentHandler) DeleteUPIQR(ctx *gin.Context) {
	response, err := ph.paymentController.DeleteUPIQR(ctx.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidDoctorInput),
			errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid UPI details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts take UPI payments"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to remove UPI QR"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "UPI QR removed successfully",
	})
}

// ListClaims godoc
// @Summary Direct UPI payments waiting for my check
// @Description The calling doctor's claimed direct UPI payments, oldest first, each with its booking. Limit defaults to 20 and is capped at 100.
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Param limit query int false "Page size"
// @Param offset query int false "Page offset"
// @Success 200 {object} dtos.SuccessResponse{data=payment.ListClaimsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/payments/claims [get]
func (ph *PaymentHandler) ListClaims(ctx *gin.Context) {
	var listClaimsRequest payment.ListClaimsRequest
	if err := ctx.ShouldBindQuery(&listClaimsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ph.paymentController.ListClaims(ctx.Request.Context(), &listClaimsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have payments to check"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list payment claims"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Payment claims retrieved successfully",
	})
}

// VerifyClaim godoc
// @Summary Confirm a direct UPI payment arrived
// @Description The appointment's doctor confirms a claimed direct UPI payment: it is captured and the booking paid. A payment no longer waiting for a check gets 409.
// @Tags doctors
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Payment ID"
// @Param request body payment.VerifyClaimRequest false "Optional note"
// @Success 200 {object} dtos.SuccessResponse{data=payment.PaymentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/payments/{id}/verify [post]
func (ph *PaymentHandler) VerifyClaim(ctx *gin.Context) {
	var verifyClaimRequest payment.VerifyClaimRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&verifyClaimRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&verifyClaimRequest); err != nil || verifyClaimRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid payment id"})
		return
	}

	response, err := ph.paymentController.VerifyClaim(ctx.Request.Context(), &verifyClaimRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPaymentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Payment not found"})
		case errors.Is(err, apperrors.ErrPaymentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Payment is not waiting for a check",
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
		"message": "Payment verified",
	})
}

// RejectClaim godoc
// @Summary Say a direct UPI payment never arrived
// @Description The appointment's doctor rejects a claimed direct UPI payment: the booking is cancelled with reason payment_rejected and its slot freed. The reason is shown to the patient. A payment no longer waiting for a check gets 409.
// @Tags doctors
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Payment ID"
// @Param request body payment.RejectClaimRequest true "Reason"
// @Success 200 {object} dtos.SuccessResponse{data=payment.PaymentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/payments/{id}/reject [post]
func (ph *PaymentHandler) RejectClaim(ctx *gin.Context) {
	var rejectClaimRequest payment.RejectClaimRequest
	if err := ctx.ShouldBindJSON(&rejectClaimRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&rejectClaimRequest); err != nil || rejectClaimRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid payment id"})
		return
	}

	response, err := ph.paymentController.RejectClaim(ctx.Request.Context(), &rejectClaimRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPaymentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Payment not found"})
		case errors.Is(err, apperrors.ErrPaymentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Payment is not waiting for a check",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to reject payment"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Payment rejected and booking cancelled",
	})
}

// MarkCollected godoc
// @Summary Mark a clinic fee collected
// @Description The appointment's doctor records that the fee of a booking due at the clinic was collected (cash or UPI at the desk): it is paid. Works on a scheduled or completed appointment.
// @Tags doctors
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Appointment ID"
// @Param request body payment.MarkCollectedRequest false "Optional note"
// @Success 200 {object} dtos.SuccessResponse{data=payment.PaymentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/appointments/{id}/collected [post]
func (ph *PaymentHandler) MarkCollected(ctx *gin.Context) {
	var markCollectedRequest payment.MarkCollectedRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&markCollectedRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&markCollectedRequest); err != nil || markCollectedRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ph.paymentController.MarkCollected(ctx.Request.Context(), &markCollectedRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrPaymentNotRequired),
			errors.Is(err, apperrors.ErrAppointmentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Nothing to collect for this appointment",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to mark fee collected"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Fee marked collected",
	})
}

// ListManualRefunds godoc
// @Summary Direct UPI refunds I owe
// @Description Refunds of direct UPI payments to the calling doctor still to be made, oldest first, each with its payment and booking. Limit defaults to 20 and is capped at 100.
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Param limit query int false "Page size"
// @Param offset query int false "Page offset"
// @Success 200 {object} dtos.SuccessResponse{data=payment.ListManualRefundsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/refunds [get]
func (ph *PaymentHandler) ListManualRefunds(ctx *gin.Context) {
	var listManualRefundsRequest payment.ListManualRefundsRequest
	if err := ctx.ShouldBindQuery(&listManualRefundsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ph.paymentController.ListManualRefunds(ctx.Request.Context(), &listManualRefundsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts owe refunds"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
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

// CloseRefund godoc
// @Summary Close a direct UPI refund
// @Description The appointment's doctor settles a refund of a direct UPI payment by hand: refunded when they sent the money back (the payment and booking are refunded), not_received when a payment that was never verified never arrived (nothing to return).
// @Tags doctors
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Refund ID"
// @Param request body payment.CloseRefundRequest true "Outcome"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/refunds/{id}/close [post]
func (ph *PaymentHandler) CloseRefund(ctx *gin.Context) {
	var closeRefundRequest payment.CloseRefundRequest
	if err := ctx.ShouldBindJSON(&closeRefundRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&closeRefundRequest); err != nil || closeRefundRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid refund id"})
		return
	}

	if err := ph.paymentController.CloseRefund(ctx.Request.Context(), &closeRefundRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidPaymentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrRefundNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Refund not found"})
		case errors.Is(err, apperrors.ErrPaymentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Refund cannot be closed this way",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to close refund"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Refund closed",
	})
}
