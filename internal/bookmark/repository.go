package bookmark

import (
	"database/sql"
	"net/url"
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
			MAX(external_id) AS external_id,
			MAX(location) as location,
			MAX(salary_range) as salary_range,
			MAX(salary_period) as salary_period,
			MAX(created_at) as job_created_at,
			MAX(apply_token_entry) AS apply_token_entry
		FROM (
				SELECT b.user_id, b.job_id, b.created_at, b.applied_at, j.slug, j.job_title, j.company, j.external_id, j.location, j.salary_range, j.salary_period, j.created_at as job_created_at, 0 as apply_token_entry
				FROM bookmark b
				LEFT JOIN job j ON j.id = b.job_id
				WHERE b.user_id = $1
				UNION
				SELECT u.id, a.job_id, a.created_at, a.created_at, j.slug, j.job_title, j.company, j.external_id, j.location, j.salary_range, j.salary_period, j.created_at as job_created_at, 1 as apply_token_entry
				FROM apply_token a
				LEFT JOIN job j ON j.id = a.job_id
				LEFT JOIN users u ON u.email = a.email
				WHERE u.id = $1
				ORDER BY created_at DESC
			) AS subquery
		GROUP BY user_id, job_id
		ORDER BY first_created_at DESC;`,
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
			&bookmark.JobExternalID,
			&bookmark.JobLocation,
			&bookmark.JobSalaryRange,
			&bookmark.JobSalaryPeriod,
			&bookmark.JobCreatedAt,
			&bookmark.HasApplyRecord,
		)
		if err != nil {
			return bookmarks, err
		}
		bookmark.JobTimeAgo = bookmark.JobCreatedAt.UTC().Format("January 2006")
		bookmark.CompanyURLEnc = url.PathEscape(bookmark.CompanyName)

		bookmarks = append(bookmarks, bookmark)
	}
	err = rows.Err()
	if err != nil {
		return bookmarks, err
	}
	return bookmarks, nil
}

// GetBookmarksByJobId can be used to quickly & efficiently check whether a job has previously been bookmarked by a user
func (r *Repository) GetBookmarksByJobId(userID string) (map[int]*Bookmark, error) {
	bookmarksByJobId := make(map[int]*Bookmark)
	bookmarks, err := r.GetBookmarksForUser(userID)
	if err != nil {
		return bookmarksByJobId, err
	}

	for _, b := range bookmarks {
		bookmarksByJobId[b.JobPostID] = b
	}

	return bookmarksByJobId, nil
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

func (r *Repository) RemoveBookmark(userID string, jobID int) error {
	_, err := r.db.Exec(
		`DELETE FROM bookmark WHERE user_id = $1 AND job_id = $2`,
		userID,
		jobID,
	)
	return err
}
