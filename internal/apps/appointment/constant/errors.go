package constant

import "errors"

var (
	ErrNilDB             = errors.New("db is nil")
	ErrNilUserRepository = errors.New("user repository is nil")
	ErrNilAuthService    = errors.New("auth service is nil")
	ErrNilAuthController = errors.New("auth controller is nil")
)

var (
	ErrNilRegisterRequest = errors.New("register request is nil")
	ErrUserAlreadyExists  = errors.New("user already exists")
)
