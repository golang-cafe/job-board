package developer

import (
	"database/sql"
	"time"
)

// CVDownloadEvent represents an audit record when a recruiter or admin downloads a developer CV.
type CVDownloadEvent struct {
	ID                 string
	DeveloperProfileID string
	CV                 []byte
	UserID             string
	DownloadedAt       time.Time
}

// SaveDeveloperCV stores or updates the binary PDF CV on a developer profile.
func (r *Repository) SaveDeveloperCV(profileID string, cvBytes []byte) error {
	_, err := r.db.Exec(
		`UPDATE developer_profile SET cv = $1, updated_at = NOW() WHERE id = $2`,
		cvBytes,
		profileID,
	)
	return err
}

// GetDeveloperCV retrieves the binary PDF CV for a developer profile ID.
func (r *Repository) GetDeveloperCV(profileID string) ([]byte, error) {
	row := r.db.QueryRow(`SELECT cv FROM developer_profile WHERE id = $1`, profileID)
	var cvBytes []byte
	if err := row.Scan(&cvBytes); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return cvBytes, nil
}

// RecordDeveloperCVDownload inserts an audit row into developer_cv_download when a CV is downloaded.
func (r *Repository) RecordDeveloperCVDownload(ev CVDownloadEvent) error {
	_, err := r.db.Exec(
		`INSERT INTO developer_cv_download (id, developer_profile_id, cv, user_id, downloaded_at) VALUES ($1, $2, $3, $4, $5)`,
		ev.ID,
		ev.DeveloperProfileID,
		ev.CV,
		ev.UserID,
		ev.DownloadedAt.UTC(),
	)
	return err
}
