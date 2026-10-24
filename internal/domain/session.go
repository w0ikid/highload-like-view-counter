package domain

import (
	"context"
	"time"
)

type Session struct {
	ID               string     `db:"id"`
	UserID           int64      `db:"user_id"`
	RefreshTokenHash string     `db:"refresh_token_hash" json:"-"`
	ExpiresAt        time.Time  `db:"expires_at"`
	RevokedAt        *time.Time `db:"revoked_at"`
	CreatedAt        time.Time  `db:"created_at"`
}

func (s Session) Active(now time.Time) bool { return s.RevokedAt == nil && now.Before(s.ExpiresAt) }

type SessionRepository interface {
	Create(context.Context, Session) error
	FindByHash(context.Context, string) (Session, error)
	FindByID(context.Context, string) (Session, error)
	Revoke(context.Context, string) error
	RevokeByUser(context.Context, int64) error
}
