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

	"appointment_system/internal/apps/appointment/adapters/payment/none"
	"appointment_system/internal/apps/appointment/adapters/payment/razorpay"
	"appointment_system/internal/apps/appointment/adapters/storage/local"
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
	AuthHandler          *handler.AuthHandler
	PatientHandler       *handler.PatientHandler
	DoctorHandler        *handler.DoctorHandler
	AdminHandler         *handler.AdminHandler
	AppointmentHandler   *handler.AppointmentHandler
	PaymentHandler       *handler.PaymentHandler
	MedicalRecordHandler *handler.MedicalRecordHandler
}

func NewContainer(
	db *gorm.DB,
	jwtSecret string,
	jwtIssuer string,
	tokenTTL time.Duration,
	clinicLocation *time.Location,
	paymentSettings PaymentSettings,
	storageProvider string,
	storageLocalDir string,
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

	patientRepository, err := repositories.NewPatientRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize patient repository: %w", err)
	}

	userPatientRepository, err := repositories.NewUserPatientRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize user-patient repository: %w", err)
	}

	patientService, err := services.NewPatientService(patientRepository, userPatientRepository, txManager)
	if err != nil {
		return nil, fmt.Errorf("initialize patient service: %w", err)
	}

	patientController, err := controller.NewPatientController(patientService)
	if err != nil {
		return nil, fmt.Errorf("initialize patient controller: %w", err)
	}

	patientHandler, err := handler.NewPatientHandler(patientController, tokenGenerator)
	if err != nil {
		return nil, fmt.Errorf("initialize patient handler: %w", err)
	}

	doctorRepository, err := repositories.NewDoctorRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize doctor repository: %w", err)
	}

	specialityRepository, err := repositories.NewSpecialityRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize speciality repository: %w", err)
	}

	doctorVerificationRepository, err := repositories.NewDoctorVerificationRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize doctor verification repository: %w", err)
	}

	adminUserService, err := services.NewAdminUserService(userRepository)
	if err != nil {
		return nil, fmt.Errorf("initialize admin user service: %w", err)
	}

	availabilityRepository, err := repositories.NewAvailabilityRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize availability repository: %w", err)
	}

	appointmentRepository, err := repositories.NewAppointmentRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize appointment repository: %w", err)
	}

	availabilityService, err := services.NewAvailabilityService(
		doctorRepository,
		availabilityRepository,
		appointmentRepository,
		txManager,
		clinicLocation,
		time.Now,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize availability service: %w", err)
	}

	doctorService, err := services.NewDoctorService(
		doctorRepository,
		specialityRepository,
		doctorVerificationRepository,
		availabilityService,
		txManager,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize doctor service: %w", err)
	}

	outboxRepository, err := repositories.NewOutboxRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize outbox repository: %w", err)
	}

	paymentRepository, err := repositories.NewPaymentRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize payment repository: %w", err)
	}

	paymentGateway, err := NewPaymentGateway(paymentSettings)
	if err != nil {
		return nil, fmt.Errorf("initialize payment gateway: %w", err)
	}

	paymentService, err := services.NewPaymentService(
		paymentRepository,
		appointmentRepository,
		outboxRepository,
		userPatientRepository,
		doctorRepository,
		paymentGateway,
		txManager,
		time.Now,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize payment service: %w", err)
	}

	appointmentService, err := services.NewAppointmentService(
		appointmentRepository,
		outboxRepository,
		userPatientRepository,
		doctorRepository,
		availabilityService,
		paymentService,
		txManager,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize appointment service: %w", err)
	}

	doctorVerificationService, err := services.NewDoctorVerificationService(
		doctorRepository,
		doctorVerificationRepository,
		appointmentService,
		txManager,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize doctor verification service: %w", err)
	}

	appointmentController, err := controller.NewAppointmentController(appointmentService)
	if err != nil {
		return nil, fmt.Errorf("initialize appointment controller: %w", err)
	}

	appointmentHandler, err := handler.NewAppointmentHandler(appointmentController, tokenGenerator)
	if err != nil {
		return nil, fmt.Errorf("initialize appointment handler: %w", err)
	}

	doctorController, err := controller.NewDoctorController(doctorService, availabilityService)
	if err != nil {
		return nil, fmt.Errorf("initialize doctor controller: %w", err)
	}

	medicalRecordRepository, err := repositories.NewMedicalRecordRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize medical record repository: %w", err)
	}

	fileStorage, err := NewFileStorage(storageProvider, storageLocalDir)
	if err != nil {
		return nil, fmt.Errorf("initialize file storage: %w", err)
	}

	medicalRecordService, err := services.NewMedicalRecordService(
		medicalRecordRepository,
		appointmentRepository,
		userPatientRepository,
		doctorRepository,
		fileStorage,
		outboxRepository,
		txManager,
		time.Now,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize medical record service: %w", err)
	}

	medicalRecordController, err := controller.NewMedicalRecordController(medicalRecordService)
	if err != nil {
		return nil, fmt.Errorf("initialize medical record controller: %w", err)
	}

	medicalRecordHandler, err := handler.NewMedicalRecordHandler(medicalRecordController, tokenGenerator)
	if err != nil {
		return nil, fmt.Errorf("initialize medical record handler: %w", err)
	}

	paymentController, err := controller.NewPaymentController(paymentService)
	if err != nil {
		return nil, fmt.Errorf("initialize payment controller: %w", err)
	}

	paymentHandler, err := handler.NewPaymentHandler(paymentController, tokenGenerator)
	if err != nil {
		return nil, fmt.Errorf("initialize payment handler: %w", err)
	}

	adminController, err := controller.NewAdminController(adminUserService, doctorVerificationService, doctorService, paymentService, medicalRecordService)
	if err != nil {
		return nil, fmt.Errorf("initialize admin controller: %w", err)
	}

	doctorHandler, err := handler.NewDoctorHandler(doctorController, tokenGenerator)
	if err != nil {
		return nil, fmt.Errorf("initialize doctor handler: %w", err)
	}

	adminHandler, err := handler.NewAdminHandler(adminController, tokenGenerator)
	if err != nil {
		return nil, fmt.Errorf("initialize admin handler: %w", err)
	}

	return &Container{
		AuthHandler:          authHandler,
		PatientHandler:       patientHandler,
		DoctorHandler:        doctorHandler,
		AdminHandler:         adminHandler,
		AppointmentHandler:   appointmentHandler,
		PaymentHandler:       paymentHandler,
		MedicalRecordHandler: medicalRecordHandler,
	}, nil
}

func (c *Container) RegisterRoutes(router *gin.RouterGroup) {
	c.AuthHandler.RegisterRoutes(router)
	c.PatientHandler.RegisterRoutes(router)
	c.DoctorHandler.RegisterRoutes(router)
	c.AdminHandler.RegisterRoutes(router)
	c.AppointmentHandler.RegisterRoutes(router)
	c.PaymentHandler.RegisterRoutes(router)
	c.MedicalRecordHandler.RegisterRoutes(router)
}

// NewFileStorage returns the file storage adapter for the configured provider.
// Config validation has already rejected unknown providers.
func NewFileStorage(provider string, localDir string) (services.FileStorage, error) {
	switch provider {
	case local.ProviderName:
		return local.New(localDir)
	default:
		return nil, fmt.Errorf("storage provider %q is not supported", provider)
	}
}

// PaymentSettings are the values the payment gateway adapters need. The
// Razorpay fields are used only when Provider is razorpay.
type PaymentSettings struct {
	Provider              string
	RazorpayKeyID         string
	RazorpayKeySecret     string
	RazorpayWebhookSecret string
}

// NewPaymentGateway returns the adapter for the configured provider. Config
// validation has already rejected unknown providers. Jobs that move money use
// it too, so they pick the same gateway as the API.
func NewPaymentGateway(settings PaymentSettings) (services.PaymentGateway, error) {
	switch settings.Provider {
	case none.ProviderName:
		return none.New(), nil
	case razorpay.ProviderName:
		return razorpay.New(settings.RazorpayKeyID, settings.RazorpayKeySecret, settings.RazorpayWebhookSecret)
	default:
		return nil, fmt.Errorf("payment provider %q is not supported", settings.Provider)
	}
}
