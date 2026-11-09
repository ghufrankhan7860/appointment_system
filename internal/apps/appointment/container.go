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
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"appointment_system/internal/apps/appointment/adapters/callhub/memory"
	"appointment_system/internal/apps/appointment/adapters/payment/none"
	"appointment_system/internal/apps/appointment/adapters/payment/razorpay"
	"appointment_system/internal/apps/appointment/adapters/storage/local"
	"appointment_system/internal/apps/appointment/adapters/useragent/uap"
	"appointment_system/internal/apps/appointment/constant"
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
	CallHandler          *handler.CallHandler

	callHub *memory.Hub
}

func NewContainer(
	db *gorm.DB,
	jwtSecret string,
	jwtIssuer string,
	tokenTTL time.Duration,
	clinicLocation *time.Location,
	paymentSettings PaymentSettings,
	callSettings CallSettings,
	storageProvider string,
	storageLocalDir string,
	logger *slog.Logger,
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

	userAgentParser, err := uap.New()
	if err != nil {
		return nil, fmt.Errorf("initialize user agent parser: %w", err)
	}

	authService, err := services.NewAuthService(
		userRepository,
		sessionRepository,
		tokenGenerator,
		userAgentParser,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize auth service: %w", err)
	}

	sessionService, err := services.NewSessionService(
		sessionRepository,
		userRepository,
		time.Now,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize session service: %w", err)
	}

	authController, err := controller.NewAuthController(authService, sessionService)
	if err != nil {
		return nil, fmt.Errorf("initialize auth controller: %w", err)
	}

	authHandler, err := handler.NewAuthHandler(authController, tokenGenerator)
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

	userPatientHistoryRepository, err := repositories.NewUserPatientHistoryRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize user-patient history repository: %w", err)
	}

	patientInviteRepository, err := repositories.NewPatientInviteRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize patient invite repository: %w", err)
	}

	patientInviteAttemptRepository, err := repositories.NewPatientInviteAttemptRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize patient invite attempt repository: %w", err)
	}

	outboxRepository, err := repositories.NewOutboxRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize outbox repository: %w", err)
	}

	patientService, err := services.NewPatientService(patientRepository, userPatientRepository, userPatientHistoryRepository, txManager, logger)
	if err != nil {
		return nil, fmt.Errorf("initialize patient service: %w", err)
	}

	patientInviteService, err := services.NewPatientInviteService(
		patientInviteRepository,
		patientInviteAttemptRepository,
		userPatientRepository,
		userPatientHistoryRepository,
		patientRepository,
		outboxRepository,
		txManager,
		time.Now,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize patient invite service: %w", err)
	}

	patientMembershipService, err := services.NewPatientMembershipService(
		userPatientRepository,
		userRepository,
		userPatientHistoryRepository,
		outboxRepository,
		txManager,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize patient membership service: %w", err)
	}

	patientController, err := controller.NewPatientController(patientService, patientInviteService, patientMembershipService)
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

	adminUserService, err := services.NewAdminUserService(userRepository, logger)
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
		logger,
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
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize doctor service: %w", err)
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
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize payment service: %w", err)
	}

	callRoomRepository, err := repositories.NewCallRoomRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize call room repository: %w", err)
	}

	callParticipantRepository, err := repositories.NewCallParticipantRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize call participant repository: %w", err)
	}

	callGuestTokenRepository, err := repositories.NewCallGuestTokenRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize call guest token repository: %w", err)
	}

	callEventRepository, err := repositories.NewCallEventRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize call event repository: %w", err)
	}

	callJoinAttemptRepository, err := repositories.NewCallJoinAttemptRepository(db)
	if err != nil {
		return nil, fmt.Errorf("initialize call join attempt repository: %w", err)
	}

	appointmentService, err := services.NewAppointmentService(
		appointmentRepository,
		outboxRepository,
		userPatientRepository,
		doctorRepository,
		availabilityService,
		paymentService,
		callRoomRepository,
		callEventRepository,
		txManager,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize appointment service: %w", err)
	}

	doctorVerificationService, err := services.NewDoctorVerificationService(
		doctorRepository,
		doctorVerificationRepository,
		appointmentService,
		txManager,
		logger,
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
		logger,
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

	paymentHandler, err := handler.NewPaymentHandler(paymentController, tokenGenerator, logger)
	if err != nil {
		return nil, fmt.Errorf("initialize payment handler: %w", err)
	}

	adminController, err := controller.NewAdminController(adminUserService, doctorVerificationService, doctorService, paymentService, medicalRecordService, sessionService)
	if err != nil {
		return nil, fmt.Errorf("initialize admin controller: %w", err)
	}

	doctorHandler, err := handler.NewDoctorHandler(doctorController, tokenGenerator)
	if err != nil {
		return nil, fmt.Errorf("initialize doctor handler: %w", err)
	}

	adminHandler, err := handler.NewAdminHandler(adminController, tokenGenerator, logger)
	if err != nil {
		return nil, fmt.Errorf("initialize admin handler: %w", err)
	}

	callTokenManager, err := utils.NewCallTokenManager(jwtSecret, jwtIssuer, constant.CallTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("initialize call token manager: %w", err)
	}

	callHub := memory.New()
	iceSettings := services.ICESettings{
		STUNURLs:   callSettings.STUNURLs,
		TURNURLs:   callSettings.TURNURLs,
		TURNSecret: callSettings.TURNSecret,
	}

	callService, err := services.NewCallService(
		appointmentRepository,
		doctorRepository,
		userPatientRepository,
		userRepository,
		callRoomRepository,
		callParticipantRepository,
		callGuestTokenRepository,
		callEventRepository,
		callJoinAttemptRepository,
		outboxRepository,
		txManager,
		callTokenManager,
		callHub,
		iceSettings,
		time.Now,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize call service: %w", err)
	}

	codeRoomService, err := services.NewCodeRoomService(
		doctorRepository,
		userRepository,
		callRoomRepository,
		callParticipantRepository,
		callEventRepository,
		callJoinAttemptRepository,
		outboxRepository,
		txManager,
		callTokenManager,
		callHub,
		iceSettings,
		time.Now,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize code room service: %w", err)
	}

	callSessionService, err := services.NewCallSessionService(
		callRoomRepository,
		callParticipantRepository,
		callEventRepository,
		outboxRepository,
		txManager,
		callTokenManager,
		callHub,
		time.Now,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize call session service: %w", err)
	}

	callController, err := controller.NewCallController(callService, codeRoomService, callSessionService)
	if err != nil {
		return nil, fmt.Errorf("initialize call controller: %w", err)
	}

	callHandler, err := handler.NewCallHandler(callController, tokenGenerator, logger)
	if err != nil {
		return nil, fmt.Errorf("initialize call handler: %w", err)
	}

	return &Container{
		AuthHandler:          authHandler,
		PatientHandler:       patientHandler,
		DoctorHandler:        doctorHandler,
		AdminHandler:         adminHandler,
		AppointmentHandler:   appointmentHandler,
		PaymentHandler:       paymentHandler,
		MedicalRecordHandler: medicalRecordHandler,
		CallHandler:          callHandler,
		callHub:              callHub,
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
	c.CallHandler.RegisterRoutes(router)
}

// Close closes the open call sockets. The HTTP server's shutdown does not,
// because their connections were taken over from it.
func (c *Container) Close() {
	c.callHub.CloseAll()
}

// CallSettings are the ICE servers handed to call peers. TURN is used only
// when both TURNURLs and TURNSecret are set.
type CallSettings struct {
	STUNURLs   []string
	TURNURLs   []string
	TURNSecret string
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
