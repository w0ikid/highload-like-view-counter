package domain

// Posts are seeded for SIS2. Author and editing workflow are not required yet.
type Post struct {
	ID    int64  `db:"id"`
	Title string `db:"title"`
}
