package auth

import (
	"context"
	"highload-counters/internal/domain"
	"time"
)

type UserRepository interface {
	Create(context.Context, domain.CreateUserInput) (domain.User, error)
	FindByEmail(context.Context, string) (domain.User, error)
	FindByID(context.Context, int64) (domain.User, error)
	Lock(context.Context, int64) error
}
type CredentialRepository interface {
	Find(context.Context, int64) (domain.Credential, error)
	Set(context.Context, int64, string) error
}
type SessionRepository interface {
	Create(context.Context, domain.Session) error
	FindByHash(context.Context, string) (domain.Session, error)
	FindByID(context.Context, string) (domain.Session, error)
	Revoke(context.Context, string) error
	RevokeByUser(context.Context, int64) error
}
type PasswordResetRepository interface {
	Create(context.Context, domain.PasswordReset) error
	FindByHash(context.Context, string) (domain.PasswordReset, error)
	Consume(context.Context, string) (bool, error)
	InvalidateByUser(context.Context, int64) error
}
type TxManager interface {
	WithinTx(context.Context, func(context.Context) error) error
}
type TokenIssuer interface {
	Issue(int64, string) (string, time.Time, error)
	Parse(string) (domain.AccessClaims, error)
}
type Mailer interface {
	SendPasswordReset(context.Context, string, string) error
}
