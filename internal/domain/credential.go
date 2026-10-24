package domain

import (
	"context"
	"time"
)

type Credential struct {
	UserID       int64     `db:"user_id"`
	PasswordHash string    `db:"password_hash" json:"-"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type CredentialRepository interface {
	Find(context.Context, int64) (Credential, error)
	Set(context.Context, int64, string) error
}
