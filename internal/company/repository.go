package company

import (
	"database/sql"
	"time"
)

const (
	companyEventPageView = "company_page_view"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db}
}

// smart group by to map lower/upper case to same map entry with many entries and pickup the upper case one
// smart group by to find typos
func (r *Repository) InferCompaniesFromJobs(since time.Time) ([]Company, error) {
	stmt := `SELECT   trim(from company), 
         max(company_url)               AS company_url, 
         max(location)                  AS locations, 
         max(company_icon_image_id)     AS company_icon_id, 
         max(created_at)                AS last_job_created_at, 
         count(id)                      AS job_count, 
         count(approved_at IS NOT NULL) AS live_jobs_count,
		 max(company_page_eligibility_expired_at) AS company_page_eligibility_expired_at
FROM     job 
WHERE    company_icon_image_id IS NOT NULL 
AND      created_at > $1
AND      approved_at IS NOT NULL
GROUP BY trim(FROM company) 
ORDER BY trim(FROM company)`
	rows, err := r.db.Query(stmt, since)
	res := make([]Company, 0)
	if err == sql.ErrNoRows {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var c Company
		if err := rows.Scan(
			&c.Name,
			&c.URL,
			&c.Locations,
			&c.IconImageID,
			&c.LastJobCreatedAt,
			&c.TotalJobCount,
			&c.ActiveJobCount,
			&c.CompanyPageEligibilityExpiredAt,
		); err != nil {
			return res, err
		}
		res = append(res, c)
	}

	return res, nil
}

func (r *Repository) SaveCompany(c Company) error {
	if c.CompanyPageEligibilityExpiredAt.Before(time.Now()) {
		c.CompanyPageEligibilityExpiredAt = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	var err error
	stmt := `INSERT INTO company (id, name, url, locations, icon_image_id, last_job_created_at, total_job_count, active_job_count, description, slug, twitter, linkedin, github, company_page_eligibility_expired_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) 
	ON CONFLICT (name) 
	DO UPDATE SET url = $3, locations = $4, icon_image_id = $5, last_job_created_at = $6, total_job_count = $7, active_job_count = $8, slug = $10, company_page_eligibility_expired_at = $14`

	_, err = r.db.Exec(
		stmt,
		c.ID,
		c.Name,
		c.URL,
		c.Locations,
		c.IconImageID,
		c.LastJobCreatedAt,
		c.TotalJobCount,
		c.ActiveJobCount,
		c.Description,
		c.Slug,
		c.Twitter,
		c.Linkedin,
		c.Github,
		c.CompanyPageEligibilityExpiredAt,
	)

	return err
}

func (r *Repository) TrackCompanyView(company *Company) error {
	stmt := `INSERT INTO company_event (event_type, company_id, created_at) VALUES ($1, $2, NOW())`
	_, err := r.db.Exec(stmt, companyEventPageView, company.ID)
	return err
}

func (r *Repository) CompanyBySlug(slug string) (*Company, error) {
	company := &Company{}
	row := r.db.QueryRow(`SELECT id, name, url, locations, last_job_created_at, icon_image_id, total_job_count, active_job_count, description, featured_post_a_job, slug, github, linkedin, twitter FROM company WHERE slug = $1`, slug)
	if err := row.Scan(&company.ID, &company.Name, &company.URL, &company.Locations, &company.LastJobCreatedAt, &company.IconImageID, &company.TotalJobCount, &company.ActiveJobCount, &company.Description, &company.Featured, &company.Slug, &company.Github, &company.Linkedin, &company.Twitter); err != nil {
		return company, err
	}

	return company, nil
}

func (r *Repository) CompaniesByQuery(location string, pageID, companiesPerPage int) ([]Company, int, error) {
	companies := []Company{}
	var rows *sql.Rows
	offset := pageID*companiesPerPage - companiesPerPage
	rows, err := getCompanyQueryForArgs(r.db, location, offset, companiesPerPage)
	if err != nil {
		return companies, 0, err
	}
	defer rows.Close()
	var fullRowsCount int
	for rows.Next() {
		c := Company{}
		var description, twitter, github, linkedin sql.NullString
		err = rows.Scan(
			&fullRowsCount,
			&c.ID,
			&c.Name,
			&c.URL,
			&c.Locations,
			&c.IconImageID,
			&c.LastJobCreatedAt,
			&c.TotalJobCount,
			&c.ActiveJobCount,
			&description,
			&c.Slug,
			&twitter,
			&github,
			&linkedin,
			&c.CompanyPageEligibilityExpiredAt,
		)
		if err != nil {
			return companies, fullRowsCount, err
		}
		if description.Valid {
			c.Description = &description.String
		}
		if twitter.Valid {
			c.Twitter = &twitter.String
		}
		if github.Valid {
			c.Github = &github.String
		}
		if linkedin.Valid {
			c.Linkedin = &linkedin.String
		}
		companies = append(companies, c)
	}
	err = rows.Err()
	if err != nil {
		return companies, fullRowsCount, err
	}
	return companies, fullRowsCount, nil
}

func (r *Repository) FeaturedCompaniesPostAJob() ([]Company, error) {
	companies := []Company{}
	rows, err := r.db.Query(`SELECT name, icon_image_id FROM company WHERE featured_post_a_job IS TRUE LIMIT 15`)
	if err != nil {
		return companies, err
	}
	defer rows.Close()
	for rows.Next() {
		c := Company{}
		err = rows.Scan(
			&c.Name,
			&c.IconImageID,
		)
		if err != nil {
			return companies, err
		}
		companies = append(companies, c)
	}
	err = rows.Err()
	if err != nil {
		return companies, err
	}
	return companies, nil
}

func (r *Repository) GetCompanySlugs() ([]string, error) {
	slugs := make([]string, 0)
	var rows *sql.Rows
	rows, err := r.db.Query(`SELECT slug FROM company WHERE description IS NOT NULL`)
	if err != nil {
		return slugs, err
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return slugs, err
		}
		slugs = append(slugs, slug)
	}

	return slugs, nil
}

func (r *Repository) CompanyExists(company string) (bool, error) {
	var count int
	row := r.db.QueryRow(`SELECT COUNT(*) as c FROM job WHERE company ILIKE '%` + company + `%'`)
	err := row.Scan(&count)
	if count > 0 {
		return true, err
	}

	return false, err
}

func (r *Repository) DeleteStaleImages(logoID string) error {
	stmt := `DELETE FROM image WHERE id NOT IN (SELECT company_icon_image_id FROM job WHERE company_icon_image_id IS NOT NULL) AND id NOT IN (SELECT icon_image_id FROM company) AND id NOT IN (SELECT image_id FROM developer_profile) AND id NOT IN ($1)`
	_, err := r.db.Exec(stmt, logoID)
	return err
}

func getCompanyQueryForArgs(conn *sql.DB, location string, offset, max int) (*sql.Rows, error) {
	if location == "" {
		return conn.Query(`
		SELECT Count(*)
         OVER() AS full_count,
       id,
       NAME,
       url,
       locations,
       icon_image_id,
       last_job_created_at,
       total_job_count,
       active_job_count,
       description,
       slug,
       twitter,
       github,
       linkedin,
	   company_page_eligibility_expired_at
FROM   company
ORDER  BY company_page_eligibility_expired_at DESC, last_job_created_at DESC
LIMIT $2 OFFSET $1`, offset, max)
	}

	return conn.Query(`
		SELECT Count(*)
         OVER() AS full_count,
       id,
       NAME,
       url,
       locations,
       icon_image_id,
       last_job_created_at,
       total_job_count,
       active_job_count,
       description,
       slug,
       twitter,
       github,
       linkedin,
	   company_page_eligibility_expired_at
FROM   company
WHERE locations ILIKE '%' || $1 || '%'
ORDER  BY company_page_eligibility_expired_at DESC, last_job_created_at DESC
LIMIT $3 OFFSET $2`, location, offset, max)
}
