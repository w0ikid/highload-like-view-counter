package domain

import (
	"context"
	"time"
)

var (
	ErrEmailInvalid    = FieldError("email", "email_invalid", "email must be a valid email address")
	ErrEmailTaken      = newError(ErrAlreadyExists, "email_taken", "email is used by another account")
	ErrUserNameInvalid = FieldError("name", "user_name_invalid", "name must be 1-100 characters")
)

type User struct {
	ID        int64     `db:"id"`
	Email     string    `db:"email"`
	Name      string    `db:"name"`
	CreatedAt time.Time `db:"created_at"`
}

type CreateUserInput struct{ Email, Name string }

type UserRepository interface {
	Create(context.Context, CreateUserInput) (User, error)
	FindByEmail(context.Context, string) (User, error)
	FindByID(context.Context, int64) (User, error)
	// Lock serializes login, refresh and reset. Must be called inside a transaction.
	Lock(context.Context, int64) error
}
