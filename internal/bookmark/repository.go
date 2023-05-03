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
		`SELECT user_id,
			job_id,
			MIN(created_at) AS first_created_at,
			MIN(applied_at) AS first_applied_at,
			MAX(slug) AS slug,
			MAX(job_title) AS job_title,
			MAX(company) AS company,
			MAX(apply_token_entry) AS apply_token_entry
		FROM (
				SELECT b.user_id, b.job_id, b.created_at, b.applied_at, j.slug, j.job_title, j.company, 0 as apply_token_entry
				FROM bookmark b
				LEFT JOIN job j ON j.id = b.job_id
				WHERE b.user_id = $1
				UNION
				SELECT u.id, a.job_id, a.created_at, a.created_at, j.slug, j.job_title, j.company, 1 as apply_token_entry
				FROM apply_token a
				LEFT JOIN job j ON j.id = a.job_id
				LEFT JOIN users u ON u.email = a.email
				WHERE u.id = $1
				ORDER BY created_at ASC
			) AS subquery
		GROUP BY user_id, job_id
		ORDER BY first_created_at ASC;`,
		userID)
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
			&bookmark.HasApplyRecord,
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

func (r *Repository) BookmarkJob(userID string, jobID int, setApplied bool) error {
	appliedAtExpr := "NULL"
	if setApplied {
		appliedAtExpr = "NOW()"
	}

	stmt := `
		INSERT INTO bookmark (user_id, job_id, created_at, applied_at)
		VALUES ($1, $2, NOW(), ` + appliedAtExpr + `)
		ON CONFLICT (user_id, job_id) DO UPDATE
			SET applied_at = EXCLUDED.applied_at
			WHERE bookmark.applied_at IS NULL`
	_, err := r.db.Exec(stmt, userID, jobID)
	return err
}
