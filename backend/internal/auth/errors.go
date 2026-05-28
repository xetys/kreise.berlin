package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user disabled")
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionRevoked     = errors.New("session revoked")
	ErrNoSession          = errors.New("no session")
)
