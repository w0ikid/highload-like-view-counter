package counter

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"highload-counters/internal/domain"
	"highload-counters/internal/infrastructure/postgres"
)

type Repository struct{ db *sqlx.DB }

func New(db *sqlx.DB) *Repository { return &Repository{db} }
func (r *Repository) SetLike(ctx context.Context, userID, postID int64, liked bool) error {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return postgres.MapError(err)
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT post_id FROM post_counters WHERE post_id=$1 FOR UPDATE`, postID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrPostNotFound
	}
	if err != nil {
		return postgres.MapError(err)
	}
	query := `DELETE FROM likes WHERE post_id=$1 AND user_id=$2`
	delta := int64(-1)
	if liked {
		query = `INSERT INTO likes(post_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`
		delta = 1
	}
	result, err := tx.ExecContext(ctx, query, postID, userID)
	if err != nil {
		return postgres.MapError(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		if _, err = tx.ExecContext(ctx, `UPDATE post_counters SET likes_count=likes_count+$2 WHERE post_id=$1`, postID, delta); err != nil {
			return postgres.MapError(err)
		}
	}
	return postgres.MapError(tx.Commit())
}
func (r *Repository) RequirePosts(ctx context.Context, ids []int64) error {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM posts WHERE id=ANY($1::bigint[])`, pq.Array(ids)).Scan(&n)
	if err != nil {
		return postgres.MapError(err)
	}
	if n != len(ids) {
		return domain.ErrPostNotFound
	}
	return nil
}
func (r *Repository) GetCounters(ctx context.Context, userID int64, ids []int64) ([]domain.Counter, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT c.post_id,c.likes_count,c.views_count,EXISTS(SELECT 1 FROM likes l WHERE l.post_id=c.post_id AND l.user_id=$2) FROM post_counters c WHERE c.post_id=ANY($1::bigint[])`, pq.Array(ids), userID)
	if err != nil {
		return nil, postgres.MapError(err)
	}
	defer rows.Close()
	found := make(map[int64]domain.Counter, len(ids))
	for rows.Next() {
		var c domain.Counter
		if err = rows.Scan(&c.PostID, &c.Likes, &c.Views, &c.LikedByMe); err != nil {
			return nil, postgres.MapError(err)
		}
		found[c.PostID] = c
	}
	if err = rows.Err(); err != nil {
		return nil, postgres.MapError(err)
	}
	out := make([]domain.Counter, 0, len(ids))
	for _, id := range ids {
		c, ok := found[id]
		if !ok {
			return nil, domain.ErrPostNotFound
		}
		out = append(out, c)
	}
	return out, nil
}
func (r *Repository) ApplyBatch(ctx context.Context, batch domain.FlushBatch) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return postgres.MapError(err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO counter_flush_log(batch_id) VALUES($1) ON CONFLICT DO NOTHING`, batch.ID)
	if err != nil {
		return postgres.MapError(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return postgres.MapError(tx.Commit())
	}
	deltas := append([]domain.ViewDelta(nil), batch.Deltas...)
	sort.Slice(deltas, func(i, j int) bool { return deltas[i].PostID < deltas[j].PostID })
	for _, delta := range deltas {
		if delta.PostID <= 0 || delta.Delta <= 0 {
			return domain.ErrViewEventInvalid
		}
		result, err = tx.ExecContext(ctx, `UPDATE post_counters SET views_count=views_count+$2 WHERE post_id=$1`, delta.PostID, delta.Delta)
		if err != nil {
			return postgres.MapError(err)
		}
		n, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return domain.ErrPostNotFound
		}
	}
	return postgres.MapError(tx.Commit())
}

// Reconcile acquires exactly the same row lock as SetLike. COUNT starts after
// the lock is acquired, so READ COMMITTED sees every previously committed like.
func (r *Repository) Reconcile(ctx context.Context) error {
	var ids []int64
	if err := r.db.SelectContext(ctx, &ids, `SELECT post_id FROM post_counters ORDER BY post_id`); err != nil {
		return postgres.MapError(err)
	}
	for _, id := range ids {
		if err := r.reconcilePost(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (r *Repository) reconcilePost(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return postgres.MapError(err)
	}
	defer tx.Rollback()
	var locked int64
	if err = tx.QueryRowContext(ctx, `SELECT post_id FROM post_counters WHERE post_id=$1 FOR UPDATE`, id).Scan(&locked); err != nil {
		return postgres.MapError(err)
	}
	_, err = tx.ExecContext(ctx, `UPDATE post_counters SET likes_count=(SELECT count(*) FROM likes WHERE post_id=$1) WHERE post_id=$1`, id)
	if err != nil {
		return postgres.MapError(err)
	}
	return postgres.MapError(tx.Commit())
}
