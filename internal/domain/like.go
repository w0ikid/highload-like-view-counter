package domain

import "time"

// Like identity is (PostID, UserID), not a separate synthetic ID.
type Like struct {
	PostID    int64     `db:"post_id"`
	UserID    int64     `db:"user_id"`
	CreatedAt time.Time `db:"created_at"`
}
