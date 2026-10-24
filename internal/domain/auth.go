package domain

import "time"

var (
	ErrInvalidCredentials = newError(ErrUnauthorized, "invalid_credentials", "invalid email or password")
	ErrTokenInvalid       = newError(ErrUnauthorized, "token_invalid", "token invalid")
	ErrTokenExpired       = newError(ErrUnauthorized, "token_expired", "token expired")
	ErrPasswordInvalid    = FieldError("password", "password_invalid", "password must be 8-128 characters")
)

type AccessClaims struct {
	UserID    int64
	SessionID string
}
type TokenPair struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}
