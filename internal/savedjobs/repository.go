package savedJobs

import (
	"database/sql"
	"time"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db}
}

func (r *Repository) SaveJob(jobID int, developerID string) error {
	_, err := r.db.Exec(
		`INSERT INTO developer_jobs_bookmark (job_id, developer_id, saved_at) VALUES ($1, $2, $3)`,
		jobID,
		developerID,
		time.Now().UTC(),
	)

	if err != nil {
		return err
	}

	return err
}
