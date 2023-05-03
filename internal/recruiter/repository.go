package recruiter

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/golang-cafe/job-board/internal/job"
	"github.com/gosimple/slug"
	"github.com/segmentio/ksuid"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db}
}

func (r *Repository) RecruiterProfileByID(id string) (Recruiter, error) {
	row := r.db.QueryRow(`SELECT id, email, name, company_url, slug, created_at, updated_at FROM recruiter_profile WHERE id = $1`, id)
	obj := Recruiter{}
	var nullTime sql.NullTime
	err := row.Scan(
		&obj.ID,
		&obj.Email,
		&obj.Name,
		&obj.CompanyURL,
		&obj.Slug,
		&obj.CreatedAt,
		&nullTime,
	)
	if nullTime.Valid {
		obj.UpdatedAt = nullTime.Time
	}
	if err != nil {
		return obj, err
	}

	return obj, nil
}

func (r *Repository) RecruiterProfilePlanExpiration(email string) (time.Time, error) {
	var expTime time.Time
	row := r.db.QueryRow(`SELECT plan_expired_at FROM recruiter_profile WHERE email = $1`, email)
	if err := row.Scan(&expTime); err != nil {
		return expTime, err
	}
	return expTime, nil
}

func (r *Repository) UpdateRecruiterPlanExpiration(email string, expiredAt time.Time) error {
	_, err := r.db.Exec(`UPDATE recruiter_profile SET plan_expired_at = $1 WHERE email = $2`, expiredAt, email)
	return err
}

func (r *Repository) ActivateRecruiterProfile(email string) error {
	_, err := r.db.Exec(`UPDATE recruiter_profile SET updated_at = NOW() WHERE email = $1`, email)
	return err
}

func (r *Repository) RecruiterProfileByEmail(email string) (Recruiter, error) {
	row := r.db.QueryRow(`SELECT id, email, name, company_url, slug, created_at, updated_at, plan_expired_at FROM recruiter_profile WHERE email = $1`, email)
	obj := Recruiter{}
	var nullTime sql.NullTime
	err := row.Scan(
		&obj.ID,
		&obj.Email,
		&obj.Name,
		&obj.CompanyURL,
		&obj.Slug,
		&obj.CreatedAt,
		&nullTime,
		&obj.PlanExpiredAt,
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
		`INSERT INTO recruiter_profile (id, email, name, company_url, slug, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`,
		dev.ID,
		dev.Email,
		dev.Name,
		dev.CompanyURL,
		dev.Slug,
	)
	return err
}

func (r *Repository) CreateRecruiterProfileBasedOnLastJobPosted(email string, jobRepo *job.Repository) error {
	k, err := ksuid.NewRandom()
	if err != nil {
		return err
	}

	job, err := jobRepo.LastJobPostedByEmail(email)
	if err != nil {
		return err
	}

	username := strings.Split(email, "@")[0]
	rec := Recruiter{
		ID:         k.String(),
		Email:      strings.ToLower(email),
		Name:       username,
		CompanyURL: job.CompanyURL,
	}
	return r.SaveRecruiterProfile(rec)
}
