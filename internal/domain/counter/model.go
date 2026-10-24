package counter

// Counter is the stored PostgreSQL snapshot; pending views are not included.
type Counter struct {
	PostID    int64
	Likes     int64
	Views     int64
	LikedByMe bool
}

type View struct{ PostID, ViewerID int64 }
