package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/auth"
	"appointment_system/middleware"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authController *controller.AuthController
	tokenVerifier  middleware.TokenVerifier
}

func NewAuthHandler(
	authController *controller.AuthController,
	tokenVerifier middleware.TokenVerifier,
) (*AuthHandler, error) {
	if authController == nil {
		return nil, apperrors.ErrNilAuthController
	}
	if tokenVerifier == nil {
		return nil, apperrors.ErrNilTokenVerifier
	}
	return &AuthHandler{
		authController: authController,
		tokenVerifier:  tokenVerifier,
	}, nil
}

func (handler *AuthHandler) RegisterRoutes(router *gin.RouterGroup) {
	auth := router.Group("/auth")
	auth.POST("/login", handler.Login)
	auth.POST("/register", handler.Register)
	auth.POST("/refresh", handler.RefreshToken)
	auth.POST("/logout", handler.Logout)

	sessions := auth.Group("/sessions", middleware.Authenticate(handler.tokenVerifier))
	sessions.GET("", handler.ListSessions)
	sessions.DELETE("/:id", handler.RevokeSession)
	sessions.POST("/revoke-others", handler.RevokeOtherSessions)
}

// Login godoc
// @Summary Log in
// @Description Log in with exactly one of email or mobileNo and a password. Opens a session (one per device login) that records the client IP, the User-Agent and Accept-Language, and the optional X-Device-* and X-App-Version headers; GET /auth/sessions lists them. The jwtToken carries the session ID.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.LoginRequest true "Login credentials"
// @Param X-Device-Id header string false "Random ID the app keeps per install or browser (max 64)"
// @Param X-Device-Name header string false "Device name, such as Ghufran's Pixel 8 (max 100)"
// @Param X-Device-Platform header string false "web, android or ios"
// @Param X-App-Version header string false "App version (max 30)"
// @Param X-Device-Screen header string false "Screen size as WxH, such as 1080x2400"
// @Param X-Device-Timezone header string false "IANA time zone, such as Asia/Kolkata"
// @Success 200 {object} dtos.SuccessResponse{data=auth.LoginResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /auth/login [post]
func (handler *AuthHandler) Login(ctx *gin.Context) {
	var loginRequest auth.LoginRequest
	if err := ctx.ShouldBindJSON(&loginRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	var clientHeaders auth.ClientHeaders
	if err := ctx.ShouldBindHeader(&clientHeaders); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid device headers",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := handler.authController.Login(ctx.Request.Context(), &loginRequest, &clientHeaders, ctx.ClientIP())
	if err != nil {
		if errors.Is(err, apperrors.ErrLoginIdentifier) {
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Provide exactly one of email or mobileNo"})
			return
		}
		if errors.Is(err, apperrors.ErrInvalidSessionInput) {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid device headers",
				"details": apperrors.Reason(err),
			})
			return
		}
		if errors.Is(err, apperrors.ErrInvalidCredentials) {
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid credentials"})
			return
		}
		_ = ctx.Error(err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to login"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Login successful",
	})
}

// Register godoc
// @Summary Register an account
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.RegisterRequest true "Account details"
// @Success 201 {object} dtos.SuccessResponse{data=auth.RegisterResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 409 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /auth/register [post]
func (ah *AuthHandler) Register(ctx *gin.Context) {
	var registerRequest auth.RegisterRequest
	if err := ctx.ShouldBindJSON(&registerRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := ah.authController.Register(ctx.Request.Context(), &registerRequest)
	if err != nil {
		if errors.Is(err, apperrors.ErrNilRegisterRequest) {
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request payload"})
			return
		}
		if errors.Is(err, apperrors.ErrInvalidRegisterInput) {
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid request payload",
				"details": apperrors.Reason(err),
			})
			return
		}
		if errors.Is(err, apperrors.ErrUserAlreadyExists) {
			ctx.JSON(http.StatusConflict, gin.H{
				"success": false,
				"error":   "User already exists",
				"details": apperrors.Reason(err),
			})
			return
		}
		_ = ctx.Error(err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to register user"})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data":    response,
		"message": "User registered successfully",
	})
}

// RefreshToken godoc
// @Summary Refresh the access token
// @Description Rotates the refresh token on the same session and returns a new jwtToken and refreshToken. The session's last-seen IP and time, User-Agent, app version and timezone are updated from this request; its login IP and device ID never change.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.RefreshRequest true "Refresh token"
// @Param X-Device-Id header string false "The ID sent at login; a different one is logged"
// @Param X-App-Version header string false "App version (max 30)"
// @Param X-Device-Timezone header string false "IANA time zone, such as Asia/Kolkata"
// @Success 200 {object} dtos.SuccessResponse{data=auth.RefreshResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /auth/refresh [post]
func (handler *AuthHandler) RefreshToken(ctx *gin.Context) {
	var refreshRequest auth.RefreshRequest
	if err := ctx.ShouldBindJSON(&refreshRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}
	var clientHeaders auth.ClientHeaders
	if err := ctx.ShouldBindHeader(&clientHeaders); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid device headers",
			"details": bindErrorDetail(err),
		})
		return
	}

	response, err := handler.authController.Refresh(ctx.Request.Context(), &refreshRequest, &clientHeaders, ctx.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidSessionInput):
			ctx.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "Invalid device headers",
				"details": apperrors.Reason(err),
			})
		case errors.Is(err, apperrors.ErrSessionNotFound),
			errors.Is(err, apperrors.ErrSessionRevoked),
			errors.Is(err, apperrors.ErrSessionExpired):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid or expired refresh token"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to refresh token"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Token refreshed successfully",
	})
}

// Logout godoc
// @Summary Log out
// @Description Revokes the refresh token. Logging out an unknown token still succeeds.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.RefreshRequest true "Refresh token"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /auth/logout [post]
func (handler *AuthHandler) Logout(ctx *gin.Context) {
	var refreshRequest auth.RefreshRequest
	if err := ctx.ShouldBindJSON(&refreshRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request payload",
			"details": bindErrorDetail(err),
		})
		return
	}

	if err := handler.authController.Logout(ctx.Request.Context(), &refreshRequest); err != nil {
		if errors.Is(err, apperrors.ErrSessionNotFound) {
			ctx.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "Logged out successfully",
			})
			return
		}
		_ = ctx.Error(err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to logout"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged out successfully",
	})
}

// ListSessions godoc
// @Summary List my sessions
// @Description The caller's active sessions, one per device login, most recently used first. current marks the session this request's token belongs to. Times are UTC.
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} dtos.SuccessResponse{data=auth.ListSessionsResponse}
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /auth/sessions [get]
func (ah *AuthHandler) ListSessions(ctx *gin.Context) {
	response, err := ah.authController.ListSessions(ctx.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to list sessions"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Sessions retrieved successfully",
	})
}

// RevokeSession godoc
// @Summary Sign out one of my sessions
// @Description Revokes one of the caller's sessions, including the current one; its refresh token stops working at once and its access token within 15 minutes. Another user's session is reported as not found.
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Param id path int true "Session ID"
// @Success 200 {object} dtos.SuccessResponse
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 404 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /auth/sessions/{id} [delete]
func (ah *AuthHandler) RevokeSession(ctx *gin.Context) {
	var revokeSessionRequest auth.RevokeSessionRequest
	if err := ctx.ShouldBindUri(&revokeSessionRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid session id"})
		return
	}

	if err := ah.authController.RevokeSession(ctx.Request.Context(), &revokeSessionRequest); err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		case errors.Is(err, apperrors.ErrSessionNotFound):
			ctx.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Session not found"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to revoke session"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Session revoked successfully",
	})
}

// RevokeOtherSessions godoc
// @Summary Sign out all my other sessions
// @Description Revokes every session of the caller except the current one and returns how many were revoked. A token issued before session IDs were carried gets 401; refresh it and try again.
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} dtos.SuccessResponse{data=auth.RevokeOtherSessionsResponse}
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /auth/sessions/revoke-others [post]
func (ah *AuthHandler) RevokeOtherSessions(ctx *gin.Context) {
	response, err := ah.authController.RevokeOtherSessions(ctx.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrPrincipalNotFound):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
		default:
			_ = ctx.Error(err)
			ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to revoke sessions"})
		}
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "Other sessions revoked successfully",
	})
}
