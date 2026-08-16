package handler

import (
	"appointment_system/internal/apps/appointment/constant"
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
		return nil, constant.ErrNilAuthController
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
}

func (handler *AuthHandler) Login(ctx *gin.Context) {
	// Implement the login logic here
}

func (ah *AuthHandler) Register(ctx *gin.Context) {
	var registerRequest auth.RegisterRequest
	if err := ctx.ShouldBindJSON(&registerRequest); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	response, err := ah.authController.Register(ctx.Request.Context(), &registerRequest)
	if err != nil {
		if errors.Is(err, constant.ErrNilRegisterRequest) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
		"message": "User registered successfully",
	})
}

func (handler *AuthHandler) RefreshToken(ctx *gin.Context) {

}
