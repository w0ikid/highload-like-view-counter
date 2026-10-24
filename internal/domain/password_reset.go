package domain

import (
	"context"
	"time"
)

type PasswordReset struct {
	UserID    int64      `db:"user_id"`
	TokenHash string     `db:"token_hash" json:"-"`
	ExpiresAt time.Time  `db:"expires_at"`
	UsedAt    *time.Time `db:"used_at"`
	CreatedAt time.Time  `db:"created_at"`
}

type PasswordResetRepository interface {
	Create(context.Context, PasswordReset) error
	FindByHash(context.Context, string) (PasswordReset, error)
	// Consume is atomic; false means used or expired.
	Consume(context.Context, string) (bool, error)
	InvalidateByUser(context.Context, int64) error
}
