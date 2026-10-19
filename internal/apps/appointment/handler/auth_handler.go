package handler

import (
	"appointment_system/internal/apps/appointment/apperrors"
	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/dtos/auth"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authController *controller.AuthController
}

func NewAuthHandler(authController *controller.AuthController) (*AuthHandler, error) {
	if authController == nil {
		return nil, apperrors.ErrNilAuthController
	}
	return &AuthHandler{
		authController: authController,
	}, nil
}

func (handler *AuthHandler) RegisterRoutes(router *gin.RouterGroup) {
	auth := router.Group("/auth")
	auth.POST("/login", handler.Login)
	auth.POST("/register", handler.Register)
	auth.POST("/refresh", handler.RefreshToken)
	auth.POST("/logout", handler.Logout)
}

// Login godoc
// @Summary Log in
// @Description Log in with exactly one of email or mobileNo and a password.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.LoginRequest true "Login credentials"
// @Success 200 {object} dtos.SuccessResponse{data=auth.LoginResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /auth/login [post]
func (handler *AuthHandler) Login(ctx *gin.Context) {
	var loginRequest auth.LoginRequest
	if err := ctx.ShouldBindJSON(&loginRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request payload"})
		return
	}

	response, err := handler.authController.Login(ctx.Request.Context(), &loginRequest)
	if err != nil {
		if errors.Is(err, apperrors.ErrLoginIdentifier) {
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Provide exactly one of email or mobileNo"})
			return
		}
		if errors.Is(err, apperrors.ErrInvalidCredentials) {
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid credentials"})
			return
		}
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
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request payload"})
		return
	}

	response, err := ah.authController.Register(ctx.Request.Context(), &registerRequest)
	if err != nil {
		if errors.Is(err, apperrors.ErrNilRegisterRequest) {
			ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request payload"})
			return
		}
		if errors.Is(err, apperrors.ErrUserAlreadyExists) {
			ctx.JSON(http.StatusConflict, gin.H{"success": false, "error": "User already exists"})
			return
		}
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
// @Description Rotates the refresh token and returns a new jwtToken and refreshToken.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body auth.RefreshRequest true "Refresh token"
// @Success 200 {object} dtos.SuccessResponse{data=auth.RefreshResponse}
// @Failure 400 {object} dtos.ErrorResponse
// @Failure 401 {object} dtos.ErrorResponse
// @Failure 500 {object} dtos.ErrorResponse
// @Router /auth/refresh [post]
func (handler *AuthHandler) RefreshToken(ctx *gin.Context) {
	var refreshRequest auth.RefreshRequest
	if err := ctx.ShouldBindJSON(&refreshRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false, "error": "Invalid Request payload"})
		return
	}

	response, err := handler.authController.Refresh(ctx.Request.Context(), &refreshRequest)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrSessionNotFound),
			errors.Is(err, apperrors.ErrSessionRevoked),
			errors.Is(err, apperrors.ErrSessionExpired):
			ctx.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid or expired refresh token"})
		default:
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
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid Request payload"})
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
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to logout"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Logged out successfully",
	})
}
