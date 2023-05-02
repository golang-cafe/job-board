package bookmark

import (
	"database/sql"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db}
}

func (r *Repository) GetBookmarksForUser(userID string) ([]*Bookmark, error) {
	bookmarks := []*Bookmark{}
	var rows *sql.Rows
	rows, err := r.db.Query(
		`SELECT b.user_id, b.job_id, b.created_at, b.applied_at, j.slug, j.job_title, j.company
		FROM bookmark b
		LEFT JOIN job j ON j.id = b.job_id
		WHERE b.user_id = $1
		ORDER BY b.created_at ASC`, userID)
	if err != nil {
		return bookmarks, err
	}

	defer rows.Close()
	for rows.Next() {
		bookmark := &Bookmark{}
		err := rows.Scan(
			&bookmark.UserID,
			&bookmark.JobPostID,
			&bookmark.CreatedAt,
			&bookmark.AppliedAt,
			&bookmark.JobSlug,
			&bookmark.JobTitle,
			&bookmark.CompanyName,
		)
		if err != nil {
			return bookmarks, err
		}

		bookmarks = append(bookmarks, bookmark)
	}
	err = rows.Err()
	if err != nil {
		return bookmarks, err
	}
	return bookmarks, nil
}
