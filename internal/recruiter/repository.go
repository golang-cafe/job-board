package recruiter

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/gosimple/slug"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db}
}

func (r *Repository) RecruiterProfileByID(id string) (Recruiter, error) {
	row := r.db.QueryRow(`SELECT id, email, name, company, company_url, slug, created_at, updated_at, title FROM recruiter_profile WHERE id = $1`, id)
	obj := Recruiter{}
	var nullTime sql.NullTime
	err := row.Scan(
		&obj.ID,
		&obj.Email,
		&obj.Name,
		&obj.Company,
		&obj.CompanyURL,
		&obj.Slug,
		&obj.CreatedAt,
		&nullTime,
		&obj.Title,
	)
	if nullTime.Valid {
		obj.UpdatedAt = nullTime.Time
	}
	if err != nil {
		return obj, err
	}

	return obj, nil
}

func (r *Repository) ActivateRecruiterProfile(email string) error {
	_, err := r.db.Exec(`UPDATE recruiter_profile SET updated_at = NOW() WHERE email = $1`, email)
	return err
}

func (r *Repository) RecruiterProfileByEmail(email string) (Recruiter, error) {
	row := r.db.QueryRow(`SELECT id, email, name, company, company_url, slug, created_at, updated_at, title FROM recruiter_profile WHERE email = $1`, email)
	obj := Recruiter{}
	var nullTime sql.NullTime
	err := row.Scan(
		&obj.ID,
		&obj.Email,
		&obj.Name,
		&obj.Company,
		&obj.CompanyURL,
		&obj.Slug,
		&obj.CreatedAt,
		&nullTime,
		&obj.Title,
	)
	if nullTime.Valid {
		obj.UpdatedAt = nullTime.Time
	} else {
		obj.UpdatedAt = obj.CreatedAt
	}
	if err == sql.ErrNoRows {
		return obj, nil
	}
	if err != nil {
		return obj, err
	}

	return obj, nil
}

func (r *Repository) SaveRecruiterProfile(dev Recruiter) error {
	dev.Slug = slug.Make(fmt.Sprintf("%s %d", dev.Name, time.Now().UTC().Unix()))
	_, err := r.db.Exec(
		`INSERT INTO recruiter_profile (id, email, name, title, company, company_url, slug, created_at, updated_at) VALUES ($1, $2, $3, 'title', 'company', $4, $5, NOW(), NOW())`,
		dev.ID,
		dev.Email,
		dev.Name,
		dev.CompanyURL,
	)
	return err
}
