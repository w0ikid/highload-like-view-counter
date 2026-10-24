package sqltx

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryRowxContext(ctx context.Context, query string, args ...any) *sqlx.Row
	QueryxContext(ctx context.Context, query string, args ...any) (*sqlx.Rows, error)
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
	NamedExecContext(ctx context.Context, query string, arg any) (sql.Result, error)
}

type ctxKey struct{}

func Inject(ctx context.Context, tx *sqlx.Tx) context.Context {
	return context.WithValue(ctx, ctxKey{}, tx)
}

func From(ctx context.Context) *sqlx.Tx {
	tx, _ := ctx.Value(ctxKey{}).(*sqlx.Tx)
	return tx
}

type Manager struct {
	db *sqlx.DB
}

func NewManager(db *sqlx.DB) *Manager {
	return &Manager{db: db}
}

func (m *Manager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if From(ctx) != nil {
		return fn(ctx)
	}
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(Inject(ctx, tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
