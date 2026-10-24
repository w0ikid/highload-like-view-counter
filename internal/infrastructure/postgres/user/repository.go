package user

import (
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"highload-counters/internal/domain"
	"highload-counters/internal/infrastructure/postgres"
)

type Repository struct{ db *sqlx.DB }

func New(db *sqlx.DB) *Repository { return &Repository{db: db} }

var _ domain.UserRepository = (*Repository)(nil)

func (r *Repository) Create(ctx context.Context, in domain.CreateUserInput) (u domain.User, err error) {
	err = postgres.Query(ctx, r.db).QueryRowxContext(ctx, `INSERT INTO users(email,name) VALUES($1,$2) RETURNING *`, in.Email, in.Name).StructScan(&u)
	err = postgres.MapError(err)
	if errors.Is(err, domain.ErrAlreadyExists) {
		return u, domain.ErrEmailTaken
	}
	return u, err
}
func (r *Repository) FindByEmail(ctx context.Context, email string) (u domain.User, err error) {
	err = postgres.Query(ctx, r.db).QueryRowxContext(ctx, `SELECT * FROM users WHERE email=$1`, email).StructScan(&u)
	return u, postgres.MapError(err)
}
func (r *Repository) FindByID(ctx context.Context, id int64) (u domain.User, err error) {
	err = postgres.Query(ctx, r.db).QueryRowxContext(ctx, `SELECT * FROM users WHERE id=$1`, id).StructScan(&u)
	return u, postgres.MapError(err)
}
func (r *Repository) Lock(ctx context.Context, id int64) error {
	var found int64
	return postgres.MapError(postgres.Query(ctx, r.db).QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&found))
}
