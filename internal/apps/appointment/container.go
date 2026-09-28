// Package appointment declares application-level dependency groups used by the
// composition root.
//
// Add a container only for a small, coherent group of application-scoped dependencies.
// Construct it from explicit repositories and capabilities, validate required values,
// and expose narrow fields or constructors. Never turn it into a request-scoped service
// locator or place raw Gin, GORM, Redis, Kafka, or vendor clients in it.
package appointment

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"appointment_system/internal/apps/appointment/controller"
	"appointment_system/internal/apps/appointment/handler"
	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/apps/appointment/utils"
)

// Container owns the assembled appointment application dependencies.
// It takes only narrow infrastructure values (a query handle plus token
// policy), never a broad store, config object, or request-scoped state.
type Container struct {
	AuthHandler *handler.AuthHandler
}

func NewContainer(
	db *gorm.DB,
	jwtSecret string,
	jwtIssuer string,
	tokenTTL time.Duration,
) (*Container, error) {
	userRepository, err := repositories.NewUserRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize user repository: %w", err)
	}

	sessionRepository, err := repositories.NewSessionRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize session repository: %w", err)
	}

	txManager, err := repositories.NewGORMTransactionManager(db)
	if err != nil {
		return nil, fmt.Errorf("initialize transaction manager: %w", err)
	}

	tokenGenerator, err := utils.NewJWTManager(jwtSecret, jwtIssuer, tokenTTL)
	if err != nil {
		return nil, fmt.Errorf("initialize token generator: %w", err)
	}

	authService, err := services.NewAuthService(
		userRepository,
		sessionRepository,
		tokenGenerator,
		txManager,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize auth service: %w", err)
	}

	authController, err := controller.NewAuthController(authService)
	if err != nil {
		return nil, fmt.Errorf("initialize auth controller: %w", err)
	}

	authHandler, err := handler.NewAuthHandler(authController)
	if err != nil {
		return nil, fmt.Errorf("initialize auth handler: %w", err)
	}

	return &Container{
		AuthHandler: authHandler,
	}, nil
}

func (c *Container) RegisterRoutes(router *gin.RouterGroup) {
	c.AuthHandler.RegisterRoutes(router)
}
