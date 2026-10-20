package apperrors

import (
	"errors"
	"fmt"
)

var (
	ErrNilDB                    = errors.New("db is nil")
	ErrNilUserRepository        = errors.New("user repository is nil")
	ErrNilTokenGenerator        = errors.New("token generator is nil")
	ErrNilTokenVerifier         = errors.New("token verifier is nil")
	ErrNilAuthService           = errors.New("auth service is nil")
	ErrNilAuthController        = errors.New("auth controller is nil")
	ErrNilSessionRepository     = errors.New("session repository is nil")
	ErrNilPatientService        = errors.New("patient service is nil")
	ErrNilPatientController     = errors.New("patient controller is nil")
	ErrNilPatientRepository     = errors.New("patient repository is nil")
	ErrNilUserPatientRepository = errors.New("user-patient repository is nil")
	ErrNilTransactionManager    = errors.New("transaction manager is nil")
)

var (
	ErrNilRegisterRequest = errors.New("register request is nil")
	ErrUserAlreadyExists  = errors.New("user already exists")
	ErrNilLoginRequest    = errors.New("login request is nil")
	ErrLoginIdentifier    = errors.New("provide exactly one of email or mobile number")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
)

var (
	ErrNilSessionRequest    = errors.New("session request is nil")
	ErrTokenAlreadyExists   = errors.New("refresh token already exists")
	ErrSessionNotFound      = errors.New("refresh token is invalid")
	ErrSessionRevoked       = errors.New("refresh token is revoked")
	ErrSessionExpired       = errors.New("session is expired")
	ErrSessionExpiryMissing = errors.New("session expiry is required")
)

var (
	ErrNilPatientRequest        = errors.New("patient request is nil")
	ErrInvalidPatientInput      = errors.New("invalid patient input")
	ErrPatientNotFound          = errors.New("patient not found")
	ErrPatientForbidden         = errors.New("patient access forbidden")
	ErrPatientAlreadyExists     = errors.New("patient already exists")
	ErrNilUserPatientRequest    = errors.New("user-patient request is nil")
	ErrUserPatientNotFound      = errors.New("user-patient not found")
	ErrUserPatientAlreadyExists = errors.New("user-patient already exists")
	ErrPrincipalNotFound        = errors.New("principal not found")
)

var (
	ErrNilDoctorRepository             = errors.New("doctor repository is nil")
	ErrNilSpecialityRepository         = errors.New("speciality repository is nil")
	ErrNilDoctorVerificationRepository = errors.New("doctor verification repository is nil")
	ErrNilDoctorService                = errors.New("doctor service is nil")
	ErrNilDoctorVerificationService    = errors.New("doctor verification service is nil")
	ErrNilAdminUserService             = errors.New("admin user service is nil")
	ErrNilDoctorController             = errors.New("doctor controller is nil")
	ErrNilAdminController              = errors.New("admin controller is nil")
)

var (
	ErrNilDoctorRequest              = errors.New("doctor request is nil")
	ErrInvalidDoctorInput            = errors.New("invalid doctor input")
	ErrDoctorNotFound                = errors.New("doctor not found")
	ErrDoctorAlreadyExists           = errors.New("doctor profile already exists for this user")
	ErrDoctorRegistrationExists      = errors.New("doctor registration number already exists for this council")
	ErrDoctorRoleRequired            = errors.New("only doctor accounts can manage a doctor profile")
	ErrSpecialityNotFound            = errors.New("speciality not found")
	ErrSpecialityAlreadyExists       = errors.New("speciality already exists")
	ErrInvalidVerificationTransition = errors.New("verification status transition is not allowed")
	ErrVerificationReasonRequired    = errors.New("a reason is required for this verification decision")
	ErrAdminAlreadyExists            = errors.New("an admin user already exists")
	ErrNilAdminRequest               = errors.New("admin request is nil")
	ErrNilSpecialityRequest          = errors.New("speciality request is nil")
	ErrNilDoctorVerificationRequest  = errors.New("doctor verification request is nil")
	ErrNilOverrideRequest            = errors.New("availability override request is nil")
	ErrInvalidAdminInput             = errors.New("invalid admin input")
)

var (
	ErrNilAvailabilityRepository = errors.New("availability repository is nil")
	ErrNilAvailabilityService    = errors.New("availability service is nil")
	ErrNilClinicLocation         = errors.New("clinic location is nil")
	ErrNilClock                  = errors.New("clock is nil")
	ErrInvalidAvailabilityInput  = errors.New("invalid availability input")
	ErrAvailabilityOverlap       = errors.New("availability blocks overlap on the same day")
)

// Which unique field clashed when creating a user. Both wrap ErrUserAlreadyExists,
// so code that only cares that the user exists can keep matching that.
var (
	ErrEmailAlreadyExists  = fmt.Errorf("email already registered: %w", ErrUserAlreadyExists)
	ErrMobileAlreadyExists = fmt.Errorf("mobile number already registered: %w", ErrUserAlreadyExists)
)
