package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"highload-counters/internal/domain"
	"highload-counters/internal/infrastructure/sqltx"
)

func Query(ctx context.Context, db *sqlx.DB) sqltx.Querier {
	if tx := sqltx.From(ctx); tx != nil {
		return tx
	}
	return db
}
func MapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	var pe *pq.Error
	if errors.As(err, &pe) && pe.Code == "23505" {
		return domain.ErrAlreadyExists
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	return fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
}
