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
	_, err := r.db.Exec(`UPDATE meta SET value = $1 WHERE key = $2`, val, key)
	return err
}
