package meta

import "database/sql"

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db}
}

func (r *Repository) GetValue(key string) (string, error) {
	res := r.db.QueryRow(`SELECT value FROM meta WHERE key = $1`, key)
	var val string
	err := res.Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

func (r *Repository) SetValue(key, val string) error {
	_, err := r.db.Exec(`INSERT INTO meta (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, key, val)
	return err
}

func (r *Repository) Delete(key string) error {
	_, err := r.db.Exec(`DELETE FROM meta WHERE key = $1`, key)
	return err
}
