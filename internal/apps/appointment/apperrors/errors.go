package apperrors

import "errors"

var (
	ErrNilDB                    = errors.New("db is nil")
	ErrNilUserRepository        = errors.New("user repository is nil")
	ErrNilTokenGenerator        = errors.New("token generator is nil")
	ErrNilAuthService           = errors.New("auth service is nil")
	ErrNilAuthController        = errors.New("auth controller is nil")
	ErrNilSessionRepository     = errors.New("session repository is nil")
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
)
