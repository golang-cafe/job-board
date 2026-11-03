package developer

import (
	"context"
	"database/sql"
)

func (r *Repository) SaveCV(ctx context.Context, email string, cv []byte) error {
	result, err := r.db.ExecContext(ctx, `UPDATE developer_profile SET cv = $1, updated_at = NOW() WHERE lower(email) = lower($2)`, cv, email)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return err
}

// Read and audit the exact version together; failed auditing must not release a CV.
func (r *Repository) DownloadCV(ctx context.Context, profileID, userID string) ([]byte, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var cv []byte
	err = tx.QueryRowContext(ctx, `SELECT cv FROM developer_profile WHERE id = $1 AND cv IS NOT NULL FOR SHARE`, profileID).Scan(&cv)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO developer_cv_download (developer_profile_id, cv, user_id, downloaded_at) VALUES ($1, $2, $3, NOW())`, profileID, cv, userID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return cv, nil
}
