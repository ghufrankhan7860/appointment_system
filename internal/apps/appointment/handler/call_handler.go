package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/authz"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/call"
	"appointment_system/middleware"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type CallHandler struct {
	callController *controller.CallController
	tokenVerifier  middleware.TokenVerifier
	logger         *slog.Logger
}

func NewCallHandler(
	callController *controller.CallController,
	tokenVerifier middleware.TokenVerifier,
	logger *slog.Logger,
) (*CallHandler, error) {
	if callController == nil {
		return nil, apperrors.ErrNilCallController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}
	if logger == nil {
		return nil, apperrors.ErrNilLogger
	}
	return &CallHandler{
		callController: callController,
		tokenVerifier:  tokenVerifier,
		logger:         logger,
	}, nil
}

func (ch *CallHandler) RegisterRoutes(router *gin.RouterGroup) {
	authenticate := middleware.Authenticate(ch.tokenVerifier)

	manage := authz.RequirePermission(authz.PermAppointmentManage)
	host := authz.RequirePermission(authz.PermCallHost)

	appointmentCalls := router.Group("/appointments/:id/call", authenticate)
	appointmentCalls.POST("", ch.OpenAppointmentCall)
	appointmentCalls.GET("", ch.GetAppointmentCall)
	appointmentCalls.POST("/end", ch.EndAppointmentCall)
	appointmentCalls.POST("/guest-links", manage, ch.CreateGuestLink)
	appointmentCalls.DELETE("/guest-links/:linkId", manage, ch.RevokeGuestLink)

	calls := router.Group("/calls")
	calls.POST("/guest/:token", ch.JoinGuestCall)
	calls.POST("/code/:code", ch.JoinRoom)
	calls.GET("/ws", ch.Socket)

	rooms := router.Group("/doctors/me/call-rooms", authenticate, host)
	rooms.POST("", ch.CreateRoom)
	rooms.GET("", ch.ListRooms)
	rooms.POST("/:id/join", ch.JoinRoomAsHost)
	rooms.POST("/:id/end", ch.EndRoom)
	rooms.GET("/:id/participants", ch.ListRoomParticipants)
}

// OpenAppointmentCall godoc
// @Summary Join an appointment's call
// @Description Joins the caller to the appointment's audio/video call, opening the room if nobody has yet. Only the appointment's doctor and the patient side with full or appointments_only access can join, from 10 minutes before the start until 30 minutes after the end of a scheduled appointment that is paid or free. Each side has one seat. Returns the token for GET /calls/ws and the ICE servers.
// @Tags calls
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Appointment ID"
// @Param request body call.OpenCallRequest false "Media to start with; audio when empty"
// @Success 200 {object} dtos.SuccessResponse{data=call.CallJoinResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/call [post]
func (ch *CallHandler) OpenAppointmentCall(ctx *gin.Context) {
	var openCallRequest call.OpenCallRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&openCallRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&openCallRequest); err != nil || openCallRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ch.callController.OpenAppointmentCall(ctx.Request.Context(), &openCallRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidCallInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid call details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrCallNotOpen):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Call is not open",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrCallRoomFull):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Call room is full"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to open call"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Call opened successfully",
	})
}

// GetAppointmentCall godoc
// @Summary Get an appointment's call
// @Description Returns the appointment's live call room and who joined it; inRoom marks the people connected right now. Open to the appointment's doctor and the patient side with full or appointments_only access.
// @Tags calls
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 200 {object} dtos.SuccessResponse{data=call.GetCallResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/call [get]
func (ch *CallHandler) GetAppointmentCall(ctx *gin.Context) {
	var getCallRequest call.GetCallRequest
	if err := ctx.ShouldBindUri(&getCallRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ch.callController.GetAppointmentCall(ctx.Request.Context(), &getCallRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidCallInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrCallRoomNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "No call is open for this appointment"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to get call"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Call fetched successfully",
	})
}

// EndAppointmentCall godoc
// @Summary Hang up an appointment's call
// @Description Ends the appointment's live call for everyone in it.
// @Tags calls
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/call/end [post]
func (ch *CallHandler) EndAppointmentCall(ctx *gin.Context) {
	var endCallRequest call.EndCallRequest
	if err := ctx.ShouldBindUri(&endCallRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	if err := ch.callController.EndAppointmentCall(ctx.Request.Context(), &endCallRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidCallInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrCallRoomNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "No call is open for this appointment"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to end call"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Call ended successfully",
	})
}

// CreateGuestLink godoc
// @Summary Create a guest link for an appointment's call
// @Description Lets the appointment's doctor invite someone without an account to the call as the patient side. The token is shown only in this response; the frontend builds the join URL from it. The link works until 30 minutes after the appointment ends, up to maxUses joins (3 by default, at most 5).
// @Tags calls
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Appointment ID"
// @Param request body call.CreateGuestLinkRequest false "Guest link"
// @Success 201 {object} dtos.SuccessResponse{data=call.GuestLinkResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/call/guest-links [post]
func (ch *CallHandler) CreateGuestLink(ctx *gin.Context) {
	var createGuestLinkRequest call.CreateGuestLinkRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&createGuestLinkRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&createGuestLinkRequest); err != nil || createGuestLinkRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment id"})
		return
	}

	response, err := ch.callController.CreateGuestLink(ctx.Request.Context(), &createGuestLinkRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidCallInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid guest link details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrCallNotOpen):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Call is not open",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create guest link"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Guest link created successfully",
	})
}

// RevokeGuestLink godoc
// @Summary Revoke a guest link
// @Description Stops a guest link from being used again. Someone already in the call stays in it.
// @Tags calls
// @Security BearerAuth
// @Produce json
// @Param id path int true "Appointment ID"
// @Param linkId path int true "Guest link ID"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /appointments/{id}/call/guest-links/{linkId} [delete]
func (ch *CallHandler) RevokeGuestLink(ctx *gin.Context) {
	var revokeGuestLinkRequest call.RevokeGuestLinkRequest
	if err := ctx.ShouldBindUri(&revokeGuestLinkRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment or guest link id"})
		return
	}

	if err := ch.callController.RevokeGuestLink(ctx.Request.Context(), &revokeGuestLinkRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidCallInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid appointment or guest link id"})
		case errors.Is(err, apperrors.ErrAppointmentNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Appointment not found"})
		case errors.Is(err, apperrors.ErrCallGuestLinkNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Guest link not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to revoke guest link"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Guest link revoked successfully",
	})
}

// JoinGuestCall godoc
// @Summary Join an appointment's call with a guest link
// @Description Public: joins someone without an account to an appointment's call as the patient side. displayName is required unless the doctor set one on the link. Refused while the patient side is in the call. 5 wrong links or codes from one IP within 10 minutes block joins from it for the rest of the window.
// @Tags calls
// @Accept json
// @Produce json
// @Param token path string true "Guest link token"
// @Param request body call.JoinGuestCallRequest false "Guest details"
// @Success 200 {object} dtos.SuccessResponse{data=call.CallJoinResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 429 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /calls/guest/{token} [post]
func (ch *CallHandler) JoinGuestCall(ctx *gin.Context) {
	var joinGuestCallRequest call.JoinGuestCallRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&joinGuestCallRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&joinGuestCallRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid guest link"})
		return
	}

	response, err := ch.callController.JoinGuestCall(ctx.Request.Context(), &joinGuestCallRequest, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidCallInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid call details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrCallJoinInvalid):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Call link is invalid or expired"})
		case errors.Is(err, apperrors.ErrCallNotOpen):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Call is not open",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrCallRoomFull):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "Call room is full"})
		case errors.Is(err, apperrors.ErrCallJoinAttemptsExceeded):
			ctx.JSON(http.StatusTooManyRequests, gin.H{"success": false, "error": "Too many wrong call links or codes, try again later"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to join call"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Call joined successfully",
	})
}

// JoinRoom godoc
// @Summary Join a code room
// @Description Public: joins a doctor's code room with its 7-digit code (spaces and dashes are ignored; a wrong check digit is answered 400 and not counted). With a lobby, waiting is true until the host admits the joiner. 5 wrong codes or links from one IP within 10 minutes block joins from it for the rest of the window.
// @Tags calls
// @Accept json
// @Produce json
// @Param code path string true "Join code"
// @Param request body call.JoinCallRoomRequest true "Joiner details"
// @Success 200 {object} dtos.SuccessResponse{data=call.CallJoinResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 429 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /calls/code/{code} [post]
func (ch *CallHandler) JoinRoom(ctx *gin.Context) {
	var joinCallRoomRequest call.JoinCallRoomRequest
	if err := ctx.ShouldBindJSON(&joinCallRoomRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	if err := ctx.ShouldBindUri(&joinCallRoomRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid room code"})
		return
	}

	response, err := ch.callController.JoinRoom(ctx.Request.Context(), &joinCallRoomRequest, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidCallInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid room details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrCallJoinInvalid):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Room code is invalid or expired"})
		case errors.Is(err, apperrors.ErrCallNotOpen):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Room is not open yet",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrCallRoomFull):
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "This room is full"})
		case errors.Is(err, apperrors.ErrCallJoinAttemptsExceeded):
			ctx.JSON(http.StatusTooManyRequests, gin.H{"success": false, "error": "Too many wrong room codes or links, try again later"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to join room"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Room joined successfully",
	})
}

// CreateRoom godoc
// @Summary Create a code room
// @Description Opens a room outside any appointment that anyone with its code can join under a display name: up to maxParticipants people including the host (5 by default), with a lobby unless requiresApproval is false. The 7-digit code is shown only in this response. With scheduledAt the room opens 10 minutes before it and lasts until 2 hours after; without it the room lasts 24 hours. Only verified doctors can host; at most 3 live rooms and 20 a day.
// @Tags calls
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body call.CreateCallRoomRequest false "Room"
// @Success 201 {object} dtos.SuccessResponse{data=call.CallRoomResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/call-rooms [post]
func (ch *CallHandler) CreateRoom(ctx *gin.Context) {
	var createCallRoomRequest call.CreateCallRoomRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&createCallRoomRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}

	response, err := ch.callController.CreateRoom(ctx.Request.Context(), &createCallRoomRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidCallInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid room details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrCallHostNotAllowed):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only verified doctors can host a room"})
		case errors.Is(err, apperrors.ErrCallRoomLimitReached):
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "Too many rooms",
				"details": apperrors.Reason(err),
			})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to create room"})
		}
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "Room created successfully",
	})
}

// ListRooms godoc
// @Summary List my code rooms
// @Description Lists the caller's code rooms, live and ended, newest first (the last 50). Codes are never shown again.
// @Tags calls
// @Security BearerAuth
// @Produce json
// @Success 200 {object} dtos.SuccessResponse{data=call.ListCallRoomsResponse}
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/call-rooms [get]
func (ch *CallHandler) ListRooms(ctx *gin.Context) {
	response, err := ch.callController.ListRooms(ctx.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list rooms"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Rooms fetched successfully",
	})
}

// JoinRoomAsHost godoc
// @Summary Join my code room as its host
// @Description Joins the caller to their own live code room. The host's seat is always kept free. The host admits, denies, mutes and removes people over the WebSocket.
// @Tags calls
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Room ID"
// @Param request body call.JoinCallRoomAsHostRequest false "Media to start with; audio when empty"
// @Success 200 {object} dtos.SuccessResponse{data=call.CallJoinResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/call-rooms/{id}/join [post]
func (ch *CallHandler) JoinRoomAsHost(ctx *gin.Context) {
	var joinCallRoomAsHostRequest call.JoinCallRoomAsHostRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&joinCallRoomAsHostRequest); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": bindErrorDetail(err),
			})
			return
		}
	}
	if err := ctx.ShouldBindUri(&joinCallRoomAsHostRequest); err != nil || joinCallRoomAsHostRequest.ID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid room id"})
		return
	}

	response, err := ch.callController.JoinRoomAsHost(ctx.Request.Context(), &joinCallRoomAsHostRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidCallInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid call details",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrCallHostNotAllowed):
			ctx.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Only verified doctors can host a room"})
		case errors.Is(err, apperrors.ErrCallRoomNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Room not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to join room"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Room joined successfully",
	})
}

// EndRoom godoc
// @Summary End my code room
// @Description Ends the caller's live code room for everyone in it.
// @Tags calls
// @Security BearerAuth
// @Produce json
// @Param id path int true "Room ID"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/call-rooms/{id}/end [post]
func (ch *CallHandler) EndRoom(ctx *gin.Context) {
	var endCallRoomRequest call.EndCallRoomRequest
	if err := ctx.ShouldBindUri(&endCallRoomRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid room id"})
		return
	}

	if err := ch.callController.EndRoom(ctx.Request.Context(), &endCallRoomRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidCallInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid room id"})
		case errors.Is(err, apperrors.ErrCallRoomNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Room not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to end room"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Room ended successfully",
	})
}

// ListRoomParticipants godoc
// @Summary List who joined my code room
// @Description Lists everyone who joined the caller's code room, waiting ones included; inRoom marks the people connected right now.
// @Tags calls
// @Security BearerAuth
// @Produce json
// @Param id path int true "Room ID"
// @Success 200 {object} dtos.SuccessResponse{data=call.ListCallRoomParticipantsResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 403 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /doctors/me/call-rooms/{id}/participants [get]
func (ch *CallHandler) ListRoomParticipants(ctx *gin.Context) {
	var listCallRoomParticipantsRequest call.ListCallRoomParticipantsRequest
	if err := ctx.ShouldBindUri(&listCallRoomParticipantsRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid room id"})
		return
	}

	response, err := ch.callController.ListRoomParticipants(ctx.Request.Context(), &listCallRoomParticipantsRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrInvalidCallInput):
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid room id"})
		case errors.Is(err, apperrors.ErrCallRoomNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Room not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list room participants"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Room participants fetched successfully",
	})
}
