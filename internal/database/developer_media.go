package database

import (
	"database/sql"
	"github.com/segmentio/ksuid"
)

func SaveDeveloperMedia(conn *sql.DB, media Media) (string, error) {
	id, err := ksuid.NewRandom()
	if err != nil {
		return "", err
	}
	_, err = conn.Exec(`INSERT INTO image (id, bytes, media_type, developer_photo) VALUES ($1,$2,$3,true)`, id.String(), media.Bytes, media.MediaType)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

func IsDeveloperMedia(conn *sql.DB, id string) (bool, error) {
	var protected bool
	err := conn.QueryRow(`SELECT developer_photo OR EXISTS (SELECT 1 FROM developer_profile WHERE image_id = image.id) FROM image WHERE id = $1`, id).Scan(&protected)
	return protected, err
}

func ProtectDeveloperMedia(conn *sql.DB, id string) error {
	res, err := conn.Exec(`UPDATE image SET developer_photo = true WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return sql.ErrNoRows
	}
	return err
}
