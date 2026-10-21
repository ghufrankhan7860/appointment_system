// Package handler contains HTTP adapters for appointment operations.
//
// For each endpoint:
//  1. Bind path, query, header, and body data into an API DTO.
//  2. Perform transport-level validation and enforce input-size limits.
//  3. Call one controller method with context.Context first.
//  4. Map response DTOs and typed application errors to stable HTTP responses.
//
// Keep business rules, transactions, and database queries out of this package. Test
// handlers with httptest, including invalid input and error mapping.
package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/authz"
	"appointment_system/internal/apps/appointment/constant"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/appointment"
	"appointment_system/middleware"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AppointmentHandler struct {
	appointmentController *controller.AppointmentController
	tokenVerifier         middleware.TokenVerifier
}

func NewAppointmentHandler(
	appointmentController *controller.AppointmentController,
	tokenVerifier middleware.TokenVerifier,
) (*AppointmentHandler, error) {
	if appointmentController == nil {
		return nil, apperrors.ErrNilAppointmentController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}
	return &AppointmentHandler{
		appointmentController: appointmentController,
		tokenVerifier:         tokenVerifier,
	}, nil
}

func (ah *AppointmentHandler) RegisterRoutes(router *gin.RouterGroup) {
	authenticate := middleware.Authenticate(ah.tokenVerifier)

	book := authz.RequirePermission(authz.PermAppointmentBook)
	manage := authz.RequirePermission(authz.PermAppointmentManage)

	appointments := router.Group("/appointments", authenticate)
	appointments.POST("", book, ah.Book)
	appointments.GET("", ah.List)
	appointments.GET("/:id", ah.GetByID)
	appointments.POST("/:id/cancel", ah.Cancel)
	appointments.POST("/:id/reschedule", book, ah.Reschedule)
	appointments.GET("/:id/reschedule-proposal", ah.GetRescheduleProposal)
	appointments.POST("/:id/reschedule-proposal/accept", book, ah.AcceptReschedule)
	appointments.POST("/:id/reschedule-proposal/decline", book, ah.DeclineReschedule)

	doctorAppointments := router.Group("/doctors/me/appointments", authenticate, manage)
	doctorAppointments.GET("", ah.ListForDoctor)
	doctorAppointments.POST("/:id/complete", ah.Complete)
	doctorAppointments.POST("/:id/no-show", ah.MarkNoShow)
	doctorAppointments.POST("/:id/reschedule-proposals", ah.ProposeReschedule)
}

// Book godoc
// @Summary Book an appointment
// @Description Books a free slot of a verified doctor for a patient the caller has full or appointments_only access to. startsAt and mode must match a slot from GET /doctors/{id}/slots; the end time, fee and slot length come from the server.
// @Tags appointments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body appointment.BookAppointmentRequest true "Booking"
// @Param Idempotency-Key header string false "Client key; a retry with the same key and body returns the first booking"
// @Success 201 {object} dtos.SuccessResponse{data=appointment.AppointmentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 422 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments [post]
func (ah *AppointmentHandler) Book(ctx *gin.Context) {
	var bookAppointmentRequest appointment.BookAppointmentRequest
	if err := ctx.ShouldBindJSON(&bookAppointmentRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid appointment details",
			"details": bindErrorDetail(err),
		})
		return
	}
	bookAppointmentRequest.IdempotencyKey = ctx.GetHeader(constant.IdempotencyKeyHeader)

	response, err := ah.appointmentController.Book(ctx.Request.Context(), &bookAppointmentRequest)
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
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Your access to this patient does not allow booking"})
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

// List godoc
// @Summary List appointments
// @Description Lists the appointments of every patient the caller has active access to, earliest first. from is inclusive and to exclusive. Limit defaults to 20 and is capped at 100.
// @Tags appointments
// @Security BearerAuth
// @Produce json
// @Param patientId query int false "Only this patient"
// @Param status query string false "Only this status" Enums(scheduled, completed, cancelled, no_show, reschedule_proposed)
// @Param from query string false "Starts at or after, like 2026-10-26T00:00:00+05:30"
// @Param to query string false "Starts before, like 2026-10-27T00:00:00+05:30"
// @Param limit query int false "Page size"
// @Param offset query int false "Number of appointments to skip"
// @Success 200 {object} dtos.SuccessResponse{data=appointment.ListAppointmentsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments [get]
func (ah *AppointmentHandler) List(ctx *gin.Context) {
	var listAppointmentsRequest appointment.ListAppointmentsRequest
	if err := ctx.ShouldBindQuery(&listAppointmentsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ah.appointmentController.List(ctx.Request.Context(), &listAppointmentsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidAppointmentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid query parameters",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list appointments"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Appointments retrieved successfully",
	})
}

// GetByID godoc
// @Summary Get an appointment
// @Description Returns an appointment of a patient the caller has active access to, or one where the caller is the doctor. Other appointments are reported as not found.
// @Tags appointments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 200 {object} dtos.SuccessResponse{data=appointment.AppointmentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id} [get]
func (ah *AppointmentHandler) GetByID(ctx *gin.Context) {
	var getAppointmentRequest appointment.GetAppointmentRequest
	if err := ctx.ShouldBindUri(&getAppointmentRequest); err != nil || getAppointmentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ah.appointmentController.GetByID(ctx.Request.Context(), &getAppointmentRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get appointment"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Appointment retrieved successfully",
	})
}

// Cancel godoc
// @Summary Cancel an appointment
// @Description Cancels a scheduled appointment that has not started. The patient side needs full or appointments_only access and must cancel at least 2 hours before the start; the doctor can cancel until the start.
// @Tags appointments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Appointment ID"
// @Param request body appointment.CancelAppointmentRequest false "Optional reason"
// @Success 200 {object} dtos.SuccessResponse{data=appointment.AppointmentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 422 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/cancel [post]
func (ah *AppointmentHandler) Cancel(ctx *gin.Context) {
	var cancelAppointmentRequest appointment.CancelAppointmentRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&cancelAppointmentRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&cancelAppointmentRequest); err != nil || cancelAppointmentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ah.appointmentController.Cancel(ctx.Request.Context(), &cancelAppointmentRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidAppointmentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPatientForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Your access to this patient does not allow cancelling"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrAppointmentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This appointment can no longer be cancelled",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrCancellationWindowPassed):
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"error":   "Too late to cancel",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to cancel appointment"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Appointment cancelled successfully",
	})
}

// Reschedule godoc
// @Summary Reschedule an appointment
// @Description Moves a scheduled appointment to another free slot of the same doctor. Only the patient side with full or appointments_only access can do it, at least 2 hours before the start. The old appointment is cancelled with reason rescheduled and the new one, returned here, has rescheduledFromId set and keeps the original fee. mode defaults to the current one.
// @Tags appointments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Appointment ID"
// @Param request body appointment.RescheduleAppointmentRequest true "New time"
// @Param Idempotency-Key header string false "Client key; a retry with the same key and body returns the first result"
// @Success 201 {object} dtos.SuccessResponse{data=appointment.AppointmentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 422 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/reschedule [post]
func (ah *AppointmentHandler) Reschedule(ctx *gin.Context) {
	var rescheduleAppointmentRequest appointment.RescheduleAppointmentRequest
	if err := ctx.ShouldBindJSON(&rescheduleAppointmentRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&rescheduleAppointmentRequest); err != nil || rescheduleAppointmentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}
	rescheduleAppointmentRequest.IdempotencyKey = ctx.GetHeader(constant.IdempotencyKeyHeader)

	response, err := ah.appointmentController.Reschedule(ctx.Request.Context(), &rescheduleAppointmentRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidAppointmentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrRescheduleForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Your access to this patient does not allow rescheduling"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor not found"})
		case errors.Is(err, apperrors.ErrAppointmentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This appointment can no longer be rescheduled",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrSlotTaken):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Selected time is not available",
				"details": "that time was just booked by someone else",
			})
		case errors.Is(err, apperrors.ErrSlotNotAvailable):
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"error":   "Selected time is not available",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrCancellationWindowPassed):
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"error":   "Too late to reschedule",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrIdempotencyKeyReused):
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"error":   "Idempotency-Key already used",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to reschedule appointment"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Appointment rescheduled successfully",
	})
}

// Complete godoc
// @Summary Complete an appointment
// @Description Marks one of the caller's scheduled appointments as completed. Only the appointment's doctor can do it, only after it has started, and only once it is paid or free.
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 200 {object} dtos.SuccessResponse{data=appointment.AppointmentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Failure 422 {object} dtos.ErrorResponse
// @Router /doctors/me/appointments/{id}/complete [post]
func (ah *AppointmentHandler) Complete(ctx *gin.Context) {
	var closeAppointmentRequest appointment.CloseAppointmentRequest
	if err := ctx.ShouldBindUri(&closeAppointmentRequest); err != nil || closeAppointmentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ah.appointmentController.Complete(ctx.Request.Context(), &closeAppointmentRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts can complete appointments"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrAppointmentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This appointment cannot be completed",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrPaymentPending):
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"error":   "Payment is pending",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to complete appointment"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Appointment completed successfully",
	})
}

// MarkNoShow godoc
// @Summary Mark an appointment as a no-show
// @Description Marks one of the caller's scheduled appointments as a no-show. Only the appointment's doctor can do it, and only after it has started.
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 200 {object} dtos.SuccessResponse{data=appointment.AppointmentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/appointments/{id}/no-show [post]
func (ah *AppointmentHandler) MarkNoShow(ctx *gin.Context) {
	var closeAppointmentRequest appointment.CloseAppointmentRequest
	if err := ctx.ShouldBindUri(&closeAppointmentRequest); err != nil || closeAppointmentRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ah.appointmentController.MarkNoShow(ctx.Request.Context(), &closeAppointmentRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts can mark a no-show"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrAppointmentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This appointment cannot be marked as a no-show",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to mark no-show"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Appointment marked as a no-show",
	})
}

// ProposeReschedule godoc
// @Summary Propose a new time for an appointment
// @Description The appointment's doctor offers another free slot. Both the current and the offered time are held, as reschedule_proposed, until the patient side accepts or declines, or the offer expires after 24 hours or at the current start, whichever is first. Only a scheduled appointment that is paid or free and has not started can be moved. mode defaults to the current one.
// @Tags doctors
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Appointment ID"
// @Param request body appointment.ProposeRescheduleRequest true "Offered time and reason"
// @Success 201 {object} dtos.SuccessResponse{data=appointment.RescheduleProposalResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 422 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/appointments/{id}/reschedule-proposals [post]
func (ah *AppointmentHandler) ProposeReschedule(ctx *gin.Context) {
	var proposeRescheduleRequest appointment.ProposeRescheduleRequest
	if err := ctx.ShouldBindJSON(&proposeRescheduleRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&proposeRescheduleRequest); err != nil || proposeRescheduleRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ah.appointmentController.ProposeReschedule(ctx.Request.Context(), &proposeRescheduleRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidAppointmentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts can propose a new time"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor not found"})
		case errors.Is(err, apperrors.ErrAppointmentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This appointment cannot be moved now",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrSlotTaken):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Selected time is not available",
				"details": "that time was just booked by someone else",
			})
		case errors.Is(err, apperrors.ErrSlotNotAvailable):
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success": false,
				"error":   "Selected time is not available",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to propose a new time"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "New time proposed successfully",
	})
}

// GetRescheduleProposal godoc
// @Summary Get the doctor's pending reschedule proposal
// @Description Returns the proposal waiting on an appointment the caller can see: the patient side with any active link, or the appointment's doctor.
// @Tags appointments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 200 {object} dtos.SuccessResponse{data=appointment.RescheduleProposalResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/reschedule-proposal [get]
func (ah *AppointmentHandler) GetRescheduleProposal(ctx *gin.Context) {
	var getRescheduleProposalRequest appointment.GetRescheduleProposalRequest
	if err := ctx.ShouldBindUri(&getRescheduleProposalRequest); err != nil || getRescheduleProposalRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ah.appointmentController.GetRescheduleProposal(ctx.Request.Context(), &getRescheduleProposalRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrProposalNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "No reschedule proposal is waiting"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get reschedule proposal"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Reschedule proposal retrieved successfully",
	})
}

// AcceptReschedule godoc
// @Summary Accept the doctor's new time
// @Description Moves the appointment to the offered time. The current appointment is cancelled with reason rescheduled and the new one, returned here, has rescheduledFromId set and keeps the payment. Only the patient side with full or appointments_only access can answer, before the proposal expires.
// @Tags appointments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 200 {object} dtos.SuccessResponse{data=appointment.AppointmentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/reschedule-proposal/accept [post]
func (ah *AppointmentHandler) AcceptReschedule(ctx *gin.Context) {
	var answerRescheduleProposalRequest appointment.AnswerRescheduleProposalRequest
	if err := ctx.ShouldBindUri(&answerRescheduleProposalRequest); err != nil || answerRescheduleProposalRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ah.appointmentController.AcceptReschedule(ctx.Request.Context(), &answerRescheduleProposalRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrRescheduleForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Your access to this patient does not allow answering"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrProposalNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "No reschedule proposal is waiting"})
		case errors.Is(err, apperrors.ErrAppointmentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This proposal can no longer be answered",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to accept the new time"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "New time accepted",
	})
}

// DeclineReschedule godoc
// @Summary Decline the doctor's new time
// @Description Keeps the current time: the offered time is released and the appointment is scheduled again. Only the patient side with full or appointments_only access can answer, before the proposal expires.
// @Tags appointments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 200 {object} dtos.SuccessResponse{data=appointment.AppointmentResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/reschedule-proposal/decline [post]
func (ah *AppointmentHandler) DeclineReschedule(ctx *gin.Context) {
	var answerRescheduleProposalRequest appointment.AnswerRescheduleProposalRequest
	if err := ctx.ShouldBindUri(&answerRescheduleProposalRequest); err != nil || answerRescheduleProposalRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ah.appointmentController.DeclineReschedule(ctx.Request.Context(), &answerRescheduleProposalRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrRescheduleForbidden):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Your access to this patient does not allow answering"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrProposalNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "No reschedule proposal is waiting"})
		case errors.Is(err, apperrors.ErrAppointmentNotModifiable):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "This proposal can no longer be answered",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to decline the new time"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "New time declined",
	})
}

// ListForDoctor godoc
// @Summary List the caller's appointments as a doctor for a day
// @Description The caller's appointments in any status on one clinic date, earliest first. date defaults to today in the clinic timezone.
// @Tags doctors
// @Security BearerAuth
// @Produce json
// @Param date query string false "Date as YYYY-MM-DD"
// @Success 200 {object} dtos.SuccessResponse{data=appointment.ListDoctorAppointmentsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/appointments [get]
func (ah *AppointmentHandler) ListForDoctor(ctx *gin.Context) {
	var listDoctorAppointmentsRequest appointment.ListDoctorAppointmentsRequest
	if err := ctx.ShouldBindQuery(&listDoctorAppointmentsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid query parameters",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ah.appointmentController.ListForDoctor(ctx.Request.Context(), &listDoctorAppointmentsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidAppointmentInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid query parameters",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrDoctorRoleRequired):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only doctor accounts have appointments to manage"})
		case errors.Is(err, apperrors.ErrDoctorNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Doctor profile not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list appointments"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Appointments retrieved successfully",
	})
}
