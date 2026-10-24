package domain

import "time"

var (
	ErrPostIDInvalid       = FieldError("postId", "post_id_invalid", "postId must be a positive int64")
	ErrPostNotFound        = newError(ErrNotFound, "post_not_found", "post not found")
	ErrViewBatchInvalid    = FieldError("events", "view_batch_invalid", "events must contain 1-50 items")
	ErrViewEventInvalid    = FieldError("events", "view_event_invalid", "postId and viewerId must be positive int64")
	ErrCounterBatchInvalid = FieldError("postIds", "counter_batch_invalid", "postIds must contain 1-100 unique positive int64 values")
)

const MaxViewEvents = 50
const MaxCounterPosts = 100
const ViewDeduplicationWindow = time.Hour

type PostCounter struct {
	PostID int64 `db:"post_id"`
	Likes  int64 `db:"likes_count"`
	Views  int64 `db:"views_count"`
}

// Counter is the read projection for the current user.
type Counter struct {
	PostID, Likes, Views int64
	LikedByMe            bool
}
type ViewEvent struct{ PostID, ViewerID int64 }
type RecordViewsResult struct{ Accepted, Duplicates int }
type ViewDelta struct{ PostID, Delta int64 }

// ID is a stable UUID retained across every retry.
type FlushBatch struct {
	ID     string
	Deltas []ViewDelta
}
type FlushLog struct {
	BatchID   string    `db:"batch_id"`
	AppliedAt time.Time `db:"applied_at"`
}
