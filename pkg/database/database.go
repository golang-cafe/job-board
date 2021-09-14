package database

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/gosimple/slug"
	"github.com/lib/pq"
	"github.com/segmentio/ksuid"
)

type Developer struct {
	ID          string
	Name        string
	LinkedinURL string
	Email       string
	Location    string
	Available   bool
	ImageID     string
	Slug        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Skills      string

	Bio                string
	SkillsArray        []string
	CreatedAtHumanized string
	UpdatedAtHumanized string
}

type DeveloperMessage struct {
	ID        string
	Email     string
	Content   string
	ProfileID string
	CreatedAt time.Time
	SentAt    time.Time
}

type Job struct {
	CreatedAt         int64
	JobTitle          string
	Company           string
	SalaryMin         string
	SalaryMax         string
	SalaryCurrency    string
	SalaryPeriod      string
	SalaryRange       string
	Location          string
	Description       string
	Perks             string
	InterviewProcess  string
	HowToApply        string
	Email             string
	Expired           bool
	LastWeekClickouts int
}

type JobRq struct {
	JobTitle         string `json:"job_title"`
	Location         string `json:"job_location"`
	Company          string `json:"company_name"`
	CompanyURL       string `json:"company_url"`
	SalaryMin        string `json:"salary_min"`
	SalaryMax        string `json:"salary_max"`
	SalaryCurrency   string `json:"salary_currency"`
	Description      string `json:"job_description"`
	HowToApply       string `json:"how_to_apply"`
	Perks            string `json:"perks"`
	InterviewProcess string `json:"interview_process,omitempty"`
	Email            string `json:"company_email"`
	StripeToken      string `json:"stripe_token,omitempty"`
	AdType           int64  `json:"ad_type"`
	CurrencyCode     string `json:"currency_code"`
	CompanyIconID    string `json:"company_icon_id,omitempty"`
	Feedback         string `json:"feedback,omitempty"`
}

type JobRqUpsell struct {
	Token        string `json:"token"`
	Email        string `json:"email"`
	StripeToken  string `json:"stripe_token,omitempty"`
	AdType       int64  `json:"ad_type"`
	CurrencyCode string `json:"currency_code"`
}

type JobRqUpdate struct {
	JobTitle         string `json:"job_title"`
	Location         string `json:"job_location"`
	Company          string `json:"company_name"`
	CompanyURL       string `json:"company_url"`
	SalaryMin        string `json:"salary_min"`
	SalaryMax        string `json:"salary_max"`
	SalaryCurrency   string `json:"salary_currency"`
	Description      string `json:"job_description"`
	HowToApply       string `json:"how_to_apply"`
	Perks            string `json:"perks"`
	InterviewProcess string `json:"interview_process"`
	Email            string `json:"company_email"`
	Token            string `json:"token"`
	CompanyIconID    string `json:"company_icon_id,omitempty"`
	SalaryPeriod     string `json:"salary_period"`
}

type JobPost struct {
	ID                int
	CreatedAt         int64
	TimeAgo           string
	JobTitle          string
	Company           string
	CompanyURL        string
	SalaryRange       string
	Location          string
	JobDescription    string
	Perks             string
	InterviewProcess  string
	HowToApply        string
	Slug              string
	SalaryCurrency    string
	AdType            int64
	SalaryMin         int64
	SalaryMax         int64
	CompanyIconID     string
	ExternalID        string
	IsQuickApply      bool
	ApprovedAt        *time.Time
	CompanyEmail      string
	SalaryPeriod      string
	CompanyURLEnc     string
	Expired           bool
	LastWeekClickouts int
}

type JobPostForEdit struct {
	ID                                                                        int
	JobTitle, Company, CompanyEmail, CompanyURL, Location                     string
	SalaryMin, SalaryMax                                                      int
	SalaryCurrency, JobDescription, Perks, InterviewProcess, HowToApply, Slug string
	CreatedAt                                                                 time.Time
	ApprovedAt                                                                pq.NullTime
	AdType                                                                    int64
	CompanyIconID                                                             string
	ExternalID                                                                string
	SalaryPeriod                                                              string
}

type ScrapedJob struct {
	Href           string
	JobTitle       string
	Company        string
	Location       string
	Salary         string
	Description    string
	CompanyWebsite string
	CompanyTwitter string
	Currency       string
}

type SEOLandingPage struct {
	URI      string
	Location string
	Skill    string
}

type SEOLocation struct {
	Name string
}

type SEOSkill struct {
	Name string `json:"name,omitempty"`
}

// Extensions
//
// CREATE EXTENSION fuzzystrmatch;
// CREATE EXTENSION pg_trgm;
//
// Table Structure:
//
// CREATE TABLE IF NOT EXISTS job (
// 	id        		   SERIAL NOT NULL,
// 	job_title          VARCHAR(128) NOT NULL,
// 	company            VARCHAR(128) NOT NULL,
// 	company_url        VARCHAR(128),
// 	company_twitter    VARCHAR(128),
// 	company_email      VARCHAR(128),
// 	salary_range       VARCHAR(100) NOT NULL,
// 	location           VARCHAR(200) NOT NULL,
// 	description        TEXT NOT NULL,
// 	perks              TEXT,
// 	interview_process  TEXT,
// 	how_to_apply       VARCHAR(512),
// 	created_at         TIMESTAMP NOT NULL,
// 	approved_at        TIMESTAMP,
// 	url_id             INTEGER NOT NULL,
// 	slug               VARCHAR(256),
//  PRIMARY KEY (id)
// );
// CREATE UNIQUE INDEX url_id_idx on job (url_id);
// CREATE UNIQUE INDEX slug_idx on job (slug);
// ALTER TABLE job ADD COLUMN salary_min INTEGER NOT NULL DEFAULT 1;
// ALTER TABLE job ADD COLUMN salary_max INTEGER NOT NULL DEFAULT 1;
// ALTER TABLE job ADD COLUMN salary_currency VARCHAR(4) NOT NULL DEFAULT '$';
// ALTER TABLE job ADD COLUMN external_id VARCHAR(28) NOT NULL;
// ALTER TABLE job ADD COLUMN external_id VARCHAR(28) DROP DEFAULT;
// ALTER TABLE job ADD COLUMN ad_type INTEGER NOT NULL DEFAULT 0;
// ALTER TABLE job ALTER COLUMN company_url SET NOT NULL;
// ALTER TABLE job ADD COLUMN company_icon_image_id VARCHAR(255) DEFAULT NULL;
// ALTER TABLE job ADD COLUMN salary_period VARCHAR(10) NOT NULL DEFAULT 'year';
// ALTER TABLE job ADD COLUMN estimated_salary BOOLEAN DEFAULT FALSE;
// ALTER TABLE job ADD COLUMN expired BOOLEAN DEFAULT FALSE;
// ALTER TABLE job ADD COLUMN last_week_clickouts INTEGER NOT NULL DEFAULT 0;

// CREATE TABLE IF NOT EXISTS image (
// 	id CHAR(27) NOT NULL UNIQUE,
// 	bytes BYTEA NOT NULL,
// 	PRIMARY KEY(id)
// )
// ALTER TABLE image ADD COLUMN media_type VARCHAR(100) NOT NULL;

// CREATE TABLE IF NOT EXISTS news (
// 	id CHAR(27) NOT NULL UNIQUE,
// 	title VARCHAR(80) NOT NULL,
// 	text TEXT NOT NULL,
// 	created_at TIMESTAMP NOT NULL,
// 	created_by CHAR(27) NOT NULL,
// 	PRIMARY KEY(id)
// );

// CREATE TABLE IF NOT EXISTS news_comment (
// 	id CHAR(27) NOT NULL UNIQUE,
// 	text TEXT NOT NULL,
// 	created_by CHAR(27) NOT NULL,
// 	created_at TIMESTAMP NOT NULL,
// 	parent_id CHAR(27) NOT NULL,
// 	PRIMARY KEY(id)
// );

// CREATE INDEX news_comment_parent_id_idx on news_comment (parent_id);

// CREATE TABLE IF NOT EXISTS users (
// 	id CHAR(27) NOT NULL UNIQUE,
// 	email VARCHAR(255) NOT NULL,
// 	username VARCHAR(255) NOT NULL,
// 	created_at TIMESTAMP,
// 	PRIMARY KEY (id)
// );
// ALTER TABLE users DROP COLUMN username;

// CREATE TABLE IF NOT EXISTS user_sign_on_token (
// 	token CHAR(27) NOT NULL UNIQUE,
// 	email VARCHAR(255) NOT NULL
// );

// CREATE INDEX user_sign_on_token_token_idx on user_sign_on_token (token);

// CREATE TABLE IF NOT EXISTS edit_token (
//   token      CHAR(27) NOT NULL,
//   job_id     INTEGER NOT NULL REFERENCES job (id),
//   created_at TIMESTAMP NOT NULL
// );
// CREATE UNIQUE INDEX token_idx on edit_token (token);

// CREATE TABLE IF NOT EXISTS purchase_event (
// 	stripe_session_id VARCHAR(255) NOT NULL,
//      amount INTEGER NOT NULL,
//      currency CHAR(3) NOT NULL,
// 	created_at TIMESTAMP NOT NULL,
// 	completed_at TIMESTAMP DEFAULT NULL,
//      description VARCHAR(255) NOT NULL,
//      ad_type INTEGER NOT NULL DEFAULT 0,
//      email VARCHAR(255) NOT NULL,
// 	job_id INTEGER NOT NULL REFERENCES job (id)
// );
// CREATE UNIQUE INDEX purchase_event_stripe_session_id_idx ON purchase_event (stripe_session_id);
// CREATE INDEX purchase_event_job_id_idx ON purchase_event (job_id);

// CREATE TABLE IF NOT EXISTS apply_token (
//   token        CHAR(27) NOT NULL,
//   job_id       INTEGER NOT NULL REFERENCES job (id),
//   created_at   TIMESTAMP NOT NULL,
//   confirmed_at TIMESTAMP DEFAULT NULL,
//   email        VARCHAR(255) NOT NULL,
//   cv           BYTEA NOT NULL,
// );
// CREATE UNIQUE INDEX token_idx on apply_token (token);

// CREATE TABLE IF NOT EXISTS developer_profile (
//   id        CHAR(27) NOT NULL,
//   email       VARCHAR(255) NOT NULL,
//   location VARCHAR(255) NOT NULL,
//   available BOOLEAN NOT NULL,
//   linkedin_url VARCHAR(255) NOT NULL,
//   github_url VARCHAR(255) NOT NULL,
//   image_id CHAR(27) NOT NULL,
//   slug VARCHAR(255) NOT NULL,
//   created_at   TIMESTAMP NOT NULL,
//   updated_at TIMESTAMP DEFAULT NULL,
//   PRIMARY KEY(id)
// );
// CREATE UNIQUE INDEX developer_profile_slug_idx on developer_profile (slug);
// CREATE UNIQUE INDEX developer_profile_email_idx on developer_profile (email);
// ALTER TABLE developer_profile ADD COLUMN skills VARCHAR(255) NOT NULL DEFAULT 'Go';
// ALTER TABLE developer_profile ADD COLUMN name VARCHAR(255) NOT NULL;
// ALTER TABLE developer_profile ADD COLUMN bio TEXT;
// ALTER TABLE developer_profile DROP COLUMN github_url;
// ALTER TABLE developer_profile ALTER COLUMN bio SET NOT NULL;

// CREATE TABLE IF NOT EXISTS job_event (
// 	event_type VARCHAR(128) NOT NULL,
// 	job_id INTEGER NOT NULL REFERENCES job (id),
// 	created_at TIMESTAMP NOT NULL
// );

// CREATE INDEX job_idx ON job_event (job_id);

// CREATE TABLE IF NOT EXISTS company_event (
// 	event_type VARCHAR(128) NOT NULL,
// 	company_id CHAR(27) NOT NULL REFERENCES company(id),
// 	created_at TIMESTAMP NOT NULL
// );

// ALTER TABLE company ALTER COLUMN slug SET NOT NULL;

// CREATE TABLE IF NOT EXISTS seo_salary (
//  id VARCHAR(255) NOT NULL,
//  location VARCHAR(255) NOT NULL,
//  currency VARCHAR(5) NOT NULL,
//  uri VARCHAR(100) NOT NULL
// );

// CREATE INDEX seo_salary_idx ON seo_salary (id);

// CREATE TABLE IF NOT EXISTS seo_skill (
//  name VARCHAR(255) NOT NULL UNIQUE
// );

// CREATE TABLE IF NOT EXISTS seo_location (
//  name VARCHAR(255) NOT NULL UNIQUE
// );
// ALTER TABLE seo_location ADD COLUMN currency VARCHAR(4) NOT NULL DEFAULT '$';
// ALTER TABLE seo_location ADD COLUMN country VARCHAR(255) DEFAULT NULL;
// alter table seo_location add column long FLOAT DEFAULT NULL
// alter table seo_location add column lat FLOAT DEFAULT NULL
// alter table seo_location add column iso3 CHAR(3) DEFAULT NULL
// alter table seo_location add column iso2 CHAR(2) DEFAULT NULL
// alter table seo_location add column region VARCHAR(255) DEFAULT NULL
// alter table seo_location add column population INTEGER DEFAULT NULL

// CREATE TABLE IF NOT EXISTS company (
//	id CHAR(27) NOT NULL UNIQUE,
//	name VARCHAR(255) NOT NULL,
//	url VARCHAR(255) NOT NULL,
// 	locations VARCHAR(255) NOT NULL,
//	last_job_created_at TIMESTAMP NOT NULL,
//	icon_image_id CHAR(27) NOT NULL,
//	total_job_count INT NOT NULL,
//	active_job_count INT NOT NULL,
//	PRIMARY KEY(id)
//);
// ALTER TABLE company ADD COLUMN featured_post_a_job BOOLEAN DEFAULT FALSE;
// ALTER TABLE company ADD COLUMN slug VARCHAR(255) DEFAULT NULL;

// CREATE UNIQUE INDEX company_name_idx ON company (name);
// ALTER TABLE company ADD COLUMN description VARCHAR(255) NOT NULL DEFAULT '';
// ALTER TABLE developer_profile ADD CONSTRAINT developer_profile_image_id_fk FOREIGN KEY (image_id) REFERENCES image(id);

// CREATE TABLE IF NOT EXISTS developer_profile_message (
//     id CHAR(27) NOT NULL UNIQUE,
//     email VARCHAR(255) NOT NULL,
//     content TEXT NOT NULL,
//     profile_id CHAR(27) NOT NULL,
//     created_at TIMESTAMP NOT NULL,
//     sent_at TIMESTAMP,
//     PRIMARY KEY(id)
// );

// CREATE TABLE IF NOT EXISTS seo_landing_page (
//  uri VARCHAR(255) NOT NULL UNIQUE,
//  location VARCHAR(255) NOT NULL,
//  skill VARCHAR(255) NOT NULL
// );

// CREATE INDEX seo_landing_page_uri ON seo_landing_page (uri);

// CREATE TABLE IF NOT EXISTS meta (
// 	key VARCHAR(255) NOT NULL UNIQUE,
// 	value VARCHAR(255) NOT NULL
// );

// CREATE TABLE search_event (
//     session_id varchar(255) not null,
//     location varchar(255) default null,
//     tag varchar(255) default null,
//     results int not null,
//     created_at timestamp not null,
// )
// ALTER TABLE search_event ADD COLUMN type VARCHAR(10) NOT NULL DEFAULT 'job';

// CREATE TABLE IF NOT EXISTS cloudflare_stats (
//     date DATE NOT NULL,
//     bytes BIGINT NOT NULL,
//     cached_bytes BIGINT NOT NULL,
//     page_views BIGINT NOT NULL,
//     requests BIGINT NOT NULL,
//     threats BIGINT NOT NULL,
//     PRIMARY KEY(date)
// );

// CREATE TABLE IF NOT EXISTS cloudflare_status_code_stats (
//    date DATE NOT NULL,
//    status_code INT NOT NULL,
//    requests BIGINT NOT NULL,
//    PRIMARY KEY(date, requests)
// );

// CREATE TABLE IF NOT EXISTS cloudflare_country_stats (
//    date DATE NOT NULL,
//    country_code CHAR(2) NOT NULL,
//    requests BIGINT NOT NULL,
//    threats BIGINT NOT NULL,
//    PRIMARY KEY(date, country_code)
// );

// CREATE TABLE IF NOT EXISTS cloudflare_browser_stats (
//    date DATE NOT NULL,
//    page_views BIGINT NOT NULL,
//    ua_browser_family VARCHAR(255) NOT NULL,
//    PRIMARY KEY(date, ua_browser_family)
// );

// CREATE TABLE IF NOT EXISTS sitemap (
//   loc varchar(255) NOT NULL,
//   changefreq varchar(20) NOT NULL DEFAULT 'weekly',
//   lastmod TIMESTAMP NOT NULL,
//   PRIMARY KEY(loc)
// );

// CREATE TABLE IF NOT EXISTS email_subscribers (
//   email VARCHAR(255) NOT NULL UNIQUE,
//   token CHAR(27) NOT NULL UNIQUE,
//   confirmed_at TIMESTAMP DEFAULT NULL,
//   created_at TIMESTAMP NOT NULL,
//   PRIMARY KEY(email)
// );

type EmailSubscriber struct {
	Email       string
	Token       string
	CreatedAt   time.Time
	ConfirmedAt *time.Time
}

func AddEmailSubscriber(conn *sql.DB, email, token string) error {
	_, err := conn.Exec(`INSERT INTO email_subscribers (email, token, created_at) VALUES ($1, $2, NOW()) ON CONFLICT DO NOTHING`, email, token)
	return err
}

func ConfirmEmailSubscriber(conn *sql.DB, token string) error {
	_, err := conn.Exec(`UPDATE email_subscribers SET confirmed_at = NOW() WHERE token = $1`, token)
	return err
}

func RemoveEmailSubscriber(conn *sql.DB, token string) error {
	_, err := conn.Exec(`DELETE FROM email_subscribers WHERE token = $1`, token)
	return err
}

func GetEmailSubscribers(conn *sql.DB) ([]EmailSubscriber, error) {
	rows, err := conn.Query(`SELECT * FROM email_subscribers WHERE confirmed_at IS NOT NULL`)
	res := make([]EmailSubscriber, 0)
	if err == sql.ErrNoRows {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var e EmailSubscriber
		var confirmedAt sql.NullTime
		if err := rows.Scan(
			&e.Email,
			&e.Token,
			&e.CreatedAt,
			&confirmedAt,
		); err != nil {
			return res, err
		}
		if confirmedAt.Valid {
			e.ConfirmedAt = &confirmedAt.Time
		}
		res = append(res, e)
	}

	return res, nil
}

const (
	jobEventPageView = "page_view"
	jobEventClickout = "clickout"

	companyEventPageView = "company_page_view"

	SearchTypeJob       = "job"
	SearchTypeSalary    = "salary"
	SearchTypeCompany   = "company"
	SearchTypeDeveloper = "developer"
)

// GetDbConn tries to establish a connection to postgres and return the connection handler
func GetDbConn(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// CloseDbConn closes db conn
func CloseDbConn(conn *sql.DB) {
	conn.Close()
}

type Company struct {
	ID               string
	Name             string
	URL              string
	Locations        string
	IconImageID      string
	Description      *string
	LastJobCreatedAt time.Time
	TotalJobCount    int
	ActiveJobCount   int
	Featured         bool
	Slug             string

	LastJobCreatedAtHumanized string
}

func InferCompaniesFromJobs(conn *sql.DB, since time.Time) ([]Company, error) {
	stmt := `SELECT   trim(from company), 
         max(company_url)               AS company_url, 
         max(location)                  AS locations, 
         max(company_icon_image_id)     AS company_icon_id, 
         max(created_at)                AS last_job_created_at, 
         count(id)                      AS job_count, 
         count(approved_at IS NOT NULL) AS live_jobs_count 
FROM     job 
WHERE    company_icon_image_id IS NOT NULL 
AND      created_at > $1
AND      approved_at IS NOT NULL
GROUP BY trim(FROM company) 
ORDER BY trim(FROM company)`
	rows, err := conn.Query(stmt, since)
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
		); err != nil {
			return res, err
		}
		res = append(res, c)
	}

	return res, nil
}

func DuplicateImage(conn *sql.DB, oldID, newID string) error {
	stmt := `INSERT INTO image (id, bytes, media_type) SELECT $1, bytes, media_type FROM image WHERE id = $2`
	_, err := conn.Exec(stmt, newID, oldID)
	return err
}

func DeleteImageByID(conn *sql.DB, id string) error {
	_, err := conn.Exec(`DELETE FROM image WHERE id = $1`, id)
	return err
}

func DeleteStaleImages(conn *sql.DB) error {
	stmt := `DELETE FROM image WHERE id NOT IN (SELECT company_icon_image_id FROM job WHERE company_icon_image_id IS NOT NULL) AND id NOT IN (SELECT icon_image_id FROM company) AND id NOT IN (SELECT image_id FROM developer_profile)`
	_, err := conn.Exec(stmt)
	return err
}

func SaveCompany(conn *sql.DB, c Company) error {
	var err error
	if c.Description != nil {
		stmt := `INSERT INTO company (id, name, url, locations, icon_image_id, last_job_created_at, total_job_count, active_job_count, description, slug)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) 
	ON CONFLICT (name) 
	DO UPDATE SET url = $3, locations = $4, icon_image_id = $5, last_job_created_at = $6, total_job_count = $7, active_job_count = $8, description = $9, slug = $10`

		_, err = conn.Exec(stmt, c.ID, c.Name, c.URL, c.Locations, c.IconImageID, c.LastJobCreatedAt, c.TotalJobCount, c.ActiveJobCount, c.Description, c.Slug)

	} else {
		stmt := `INSERT INTO company (id, name, url, locations, icon_image_id, last_job_created_at, total_job_count, active_job_count, slug)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) 
	ON CONFLICT (name) 
	DO UPDATE SET url = $3, locations = $4, icon_image_id = $5, last_job_created_at = $6, total_job_count = $7, active_job_count = $8, slug = $9`
		_, err = conn.Exec(stmt, c.ID, c.Name, c.URL, c.Locations, c.IconImageID, c.LastJobCreatedAt, c.TotalJobCount, c.ActiveJobCount, c.Slug)
	}

	return err
}

func TrackJobView(conn *sql.DB, job *JobPost) error {
	stmt := `INSERT INTO job_event (event_type, job_id, created_at) VALUES ($1, $2, NOW())`
	_, err := conn.Exec(stmt, jobEventPageView, job.ID)
	return err
}

func TrackCompanyView(conn *sql.DB, company *Company) error {
	stmt := `INSERT INTO company_event (event_type, company_id, created_at) VALUES ($1, $2, NOW())`
	_, err := conn.Exec(stmt, companyEventPageView, company.ID)
	return err
}

func TrackSearchEvent(conn *sql.DB, ua string, sessionID string, loc string, tag string, results int, typ string) error {
	hasBot := regexp.MustCompile(`(?i)(googlebot|bingbot|slurp|baiduspider|duckduckbot|yandexbot|sogou|exabot|facebookexternalhit|facebot|ia_archiver|linkedinbot|python-urllib|python-requests|go-http-client|msnbot|ahrefs)`)
	if hasBot.MatchString(ua) {
		return nil
	}
	loc = strings.TrimSpace(loc)
	tag = strings.TrimSpace(tag)
	if loc == "" && tag == "" {
		return nil
	}
	stmt := `INSERT INTO search_event (session_id, location, tag, results, type, created_at) VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, NOW())`
	_, err := conn.Exec(stmt, sessionID, loc, tag, results, typ)
	return err
}

func ApplyToJob(conn *sql.DB, jobID int, cv []byte, email, token string) error {
	stmt := `INSERT INTO apply_token (token, job_id, created_at, email, cv) VALUES ($1, $2, NOW(), $3, $4)`
	_, err := conn.Exec(stmt, token, jobID, email, cv)
	return err
}

func ConfirmApplyToJob(conn *sql.DB, token string) error {
	_, err := conn.Exec(
		`UPDATE apply_token SET confirmed_at = NOW() WHERE token = $1`,
		token,
	)
	return err
}

func DeveloperProfileBySlug(conn *sql.DB, slug string) (Developer, error) {
	row := conn.QueryRow(`SELECT * FROM developer_profile WHERE slug = $1`, slug)
	dev := Developer{}
	err := row.Scan(
		&dev.ID,
		&dev.Email,
		&dev.Location,
		&dev.Available,
		&dev.LinkedinURL,
		&dev.ImageID,
		&dev.Slug,
		&dev.CreatedAt,
		&dev.UpdatedAt,
		&dev.Skills,
		&dev.Name,
		&dev.Bio,
	)
	if err != nil {
		return dev, err
	}

	return dev, nil
}

func DeveloperProfileByEmail(conn *sql.DB, email string) (Developer, error) {
	row := conn.QueryRow(`SELECT * FROM developer_profile WHERE lower(email) = lower($1)`, email)
	dev := Developer{}
	err := row.Scan(
		&dev.ID,
		&dev.Email,
		&dev.Location,
		&dev.Available,
		&dev.LinkedinURL,
		&dev.ImageID,
		&dev.Slug,
		&dev.CreatedAt,
		&dev.UpdatedAt,
		&dev.Skills,
		&dev.Name,
		&dev.Bio,
	)
	if err == sql.ErrNoRows {
		return dev, nil
	}
	if err != nil {
		return dev, err
	}

	return dev, nil
}

func DeveloperProfileByID(conn *sql.DB, id string) (Developer, error) {
	row := conn.QueryRow(`SELECT * FROM developer_profile WHERE id = $1`, id)
	dev := Developer{}
	err := row.Scan(
		&dev.ID,
		&dev.Email,
		&dev.Location,
		&dev.Available,
		&dev.LinkedinURL,
		&dev.ImageID,
		&dev.Slug,
		&dev.CreatedAt,
		&dev.UpdatedAt,
		&dev.Skills,
		&dev.Name,
		&dev.Bio,
	)
	if err != nil {
		return dev, err
	}

	return dev, nil
}

func SendMessageDeveloperProfile(conn *sql.DB, message DeveloperMessage) error {
	_, err := conn.Exec(
		`INSERT INTO developer_profile_message (id, email, content, profile_id, created_at) VALUES ($1, $2, $3, $4, NOW())`,
		message.ID,
		message.Email,
		message.Content,
		message.ProfileID,
	)
	return err
}

func MessageForDeliveryByID(conn *sql.DB, id string) (DeveloperMessage, string, error) {
	row := conn.QueryRow(`SELECT dpm.id, dpm.email, dpm.content, dpm.profile_id, dpm.created_at, dp.email as dev_email FROM developer_profile_message dpm JOIN developer_profile dp ON dp.id = dpm.profile_id WHERE dpm.id = $1 AND dpm.sent_at IS NULL`, id)
	var devEmail string
	var message DeveloperMessage
	err := row.Scan(
		&message.ID,
		&message.Email,
		&message.Content,
		&message.ProfileID,
		&message.CreatedAt,
		&devEmail,
	)
	if err != nil {
		return message, devEmail, err
	}

	return message, devEmail, nil
}

func MarkDeveloperMessageAsSent(conn *sql.DB, id string) error {
	_, err := conn.Exec(`UPDATE developer_profile_message SET sent_at = NOW() WHERE id = $1`, id)
	return err
}

func DevelopersByLocationAndTag(conn *sql.DB, loc, tag string, pageID, pageSize int) ([]Developer, int, error) {
	var rows *sql.Rows
	var err error
	offset := pageID*pageSize - pageSize
	var developers []Developer
	switch {
	case tag != "" && loc != "":
		rows, err = conn.Query(`SELECT count(*) OVER() AS full_count, * FROM developer_profile WHERE location ILIKE '%' || $1 || '%' AND skills ILIKE '%' || $2 || '%' AND created_at != updated_at ORDER BY updated_at DESC LIMIT $3 OFFSET $4`, loc, tag, pageSize, offset)
	case tag != "" && loc == "":
		rows, err = conn.Query(`SELECT count(*) OVER() AS full_count, * FROM developer_profile WHERE skills ILIKE '%' || $1 || '%' AND created_at != updated_at ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, tag, pageSize, offset)
	case tag == "" && loc != "":
		rows, err = conn.Query(`SELECT count(*) OVER() AS full_count, * FROM developer_profile WHERE location ILIKE '%' || $1 || '%' AND created_at != updated_at ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, loc, pageSize, offset)
	default:
		rows, err = conn.Query(`SELECT count(*) OVER() AS full_count, * FROM developer_profile WHERE created_at != updated_at ORDER BY updated_at DESC LIMIT $1 OFFSET $2`, pageSize, offset)
	}
	if err == sql.ErrNoRows {
		return developers, 0, nil
	}
	var fullRowsCount int
	for rows.Next() {
		var dev Developer
		err := rows.Scan(
			&fullRowsCount,
			&dev.ID,
			&dev.Email,
			&dev.Location,
			&dev.Available,
			&dev.LinkedinURL,
			&dev.ImageID,
			&dev.Slug,
			&dev.CreatedAt,
			&dev.UpdatedAt,
			&dev.Skills,
			&dev.Name,
			&dev.Bio,
		)
		if err != nil {
			return developers, fullRowsCount, err
		}
		developers = append(developers, dev)
	}

	return developers, fullRowsCount, nil
}

func UpdateDeveloperProfile(conn *sql.DB, dev Developer) error {
	_, err := conn.Exec(`UPDATE developer_profile SET name = $1, location = $2, linkedin_url = $3, bio = $4, available = $5, image_id = $6, updated_at = NOW(), skills = $7  WHERE id = $8`, dev.Name, dev.Location, dev.LinkedinURL, dev.Bio, dev.Available, dev.ImageID, dev.Skills, dev.ID)
	return err
}

func DeleteDeveloperProfile(conn *sql.DB, id, email string) error {
	_, err := conn.Exec(`DELETE FROM developer_profile WHERE id = $1 AND email = $2`, id, email)
	return err
}

func DeleteUserByEmail(conn *sql.DB, email string) error {
	_, err := conn.Exec(`DELETE FROM users WHERE email = $1`, email)
	return err
}

func ActivateDeveloperProfile(conn *sql.DB, email string) error {
	_, err := conn.Exec(`UPDATE developer_profile SET updated_at = NOW() WHERE email = $1`, email)
	return err
}

func SaveDeveloperProfile(conn *sql.DB, dev Developer) error {
	dev.Slug = slug.Make(fmt.Sprintf("%s %d", dev.Name, time.Now().UTC().Unix()))
	_, err := conn.Exec(`INSERT INTO developer_profile (email, location, linkedin_url, bio, available, image_id, slug, created_at, updated_at, skills, name, id) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW(), $8, $9, $10)`, dev.Email, dev.Location, dev.LinkedinURL, dev.Bio, dev.Available, dev.ImageID, dev.Slug, dev.Skills, dev.Name, dev.ID)
	return err
}

type Applicant struct {
	Cv    []byte
	Email string
}

func GetJobByApplyToken(conn *sql.DB, token string) (JobPost, Applicant, error) {
	res := conn.QueryRow(`SELECT t.cv, t.email, j.id, j.job_title, j.company, company_url, salary_range, location, how_to_apply, slug, j.external_id
	FROM job j JOIN apply_token t ON t.job_id = j.id AND t.token = $1 WHERE j.approved_at IS NOT NULL AND t.created_at < NOW() + INTERVAL '3 days' AND t.confirmed_at IS NULL`, token)
	job := JobPost{}
	applicant := Applicant{}
	err := res.Scan(&applicant.Cv, &applicant.Email, &job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.HowToApply, &job.Slug, &job.ExternalID)
	if err != nil {
		return JobPost{}, applicant, err
	}

	return job, applicant, nil
}

func TrackJobClickout(conn *sql.DB, jobID int) error {
	stmt := `INSERT INTO job_event (event_type, job_id, created_at) VALUES ($1, $2, NOW())`
	_, err := conn.Exec(stmt, jobEventClickout, jobID)
	if err != nil {
		return err
	}
	return nil
}

func GetJobByExternalID(conn *sql.DB, externalID string) (JobPost, error) {
	res := conn.QueryRow(`SELECT id, job_title, company, company_url, salary_range, location, how_to_apply, slug, external_id, salary_period FROM job WHERE external_id = $1`, externalID)
	var job JobPost
	err := res.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.HowToApply, &job.Slug, &job.ExternalID, &job.SalaryPeriod)
	if err != nil {
		return job, err
	}

	return job, nil
}

type JobAdType int

const (
	JobAdBasic = iota
	JobAdSponsoredBackground
	JobAdSponsoredPinnedFor30Days
	JobAdSponsoredPinnedFor7Days
	JobAdWithCompanyLogo
	JobAdSponsoredPinnedFor60Days
)

// DemoteJobAdsOlderThan
func DemoteJobAdsOlderThan(conn *sql.DB, since time.Time, jobAdType JobAdType) (int, error) {
	res := conn.QueryRow(`WITH rows AS (UPDATE job SET ad_type = $1 WHERE ad_type = $2 AND approved_at <= $3 RETURNING 1) SELECT count(*) as c FROM rows;`, JobAdBasic, jobAdType, since)
	var affected int
	err := res.Scan(&affected)
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func GetJobsOlderThan(conn *sql.DB, since time.Time, adType JobAdType) ([]JobPost, error) {
	var jobs []JobPost
	rows, err := conn.Query(`SELECT id, job_title, company, company_url, company_email, salary_range, location, how_to_apply, slug, external_id, approved_at FROM job j WHERE approved_at <= $1 AND ad_type = $2`, since, adType)
	if err == sql.ErrNoRows {
		return jobs, nil
	}
	for rows.Next() {
		var job JobPost
		var approvedAt sql.NullTime
		err := rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.CompanyEmail, &job.SalaryRange, &job.Location, &job.HowToApply, &job.Slug, &job.ExternalID, &approvedAt)
		if err != nil {
			return jobs, err
		}
		if approvedAt.Valid {
			job.ApprovedAt = &approvedAt.Time
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

func LocationsByPrefix(conn *sql.DB, prefix string) ([]Location, error) {
	locs := make([]Location, 0)
	if len(prefix) < 2 {
		return locs, nil
	}
	rows, err := conn.Query(`SELECT name FROM seo_location WHERE name ILIKE $1 || '%' ORDER BY population DESC LIMIT 5`, prefix)
	if err == sql.ErrNoRows {
		return locs, nil
	} else if err != nil {
		return locs, err
	}
	for rows.Next() {
		var loc Location
		if err := rows.Scan(&loc.Name); err != nil {
			return locs, err
		}
		locs = append(locs, loc)
	}

	return locs, nil
}

func SkillsByPrefix(conn *sql.DB, prefix string) ([]SEOSkill, error) {
	skills := make([]SEOSkill, 0)
	if len(prefix) < 2 {
		return skills, nil
	}
	rows, err := conn.Query(`SELECT name FROM seo_skill WHERE name ILIKE $1 || '%' ORDER BY name ASC LIMIT 5`, prefix)
	if err == sql.ErrNoRows {
		return skills, nil
	} else if err != nil {
		return skills, err
	}
	for rows.Next() {
		var skill SEOSkill
		if err := rows.Scan(&skill.Name); err != nil {
			return skills, err
		}
		skills = append(skills, skill)
	}

	return skills, nil
}

func UpdateJobAdType(conn *sql.DB, adType int, jobID int) error {
	_, err := conn.Exec(`UPDATE job SET ad_type = $1, approved_at = NOW() WHERE id = $2`, adType, jobID)
	return err
}

type SalaryDataPoint struct {
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

func GetSalaryDataForLocationAndCurrency(conn *sql.DB, location, currency string) ([]SalaryDataPoint, error) {
	var res []SalaryDataPoint
	var rows *sql.Rows
	rows, err := conn.Query(`
	SELECT salary_min, salary_max
		FROM job WHERE approved_at IS NOT NULL AND salary_currency = $1 AND location ILIKE '%' || $2 || '%' AND salary_period = 'year'`, currency, location)
	if err != nil {
		return res, err
	}
	defer rows.Close()
	for rows.Next() {
		dp := SalaryDataPoint{}
		err = rows.Scan(&dp.Min, &dp.Max)
		if err != nil {
			return res, err
		}
		res = append(res, dp)
	}
	err = rows.Err()
	if err != nil {
		return res, err
	}
	return res, nil
}

type SalaryTrendDataPoint struct {
	Date string `json:"date"`
	P10  int64  `json:"p10"`
	P25  int64  `json:"p25"`
	P50  int64  `json:"p50"`
	P75  int64  `json:"p75"`
	P90  int64  `json:"p90"`
}

func GetSalaryTrendsForLocationAndCurrency(conn *sql.DB, location, currency string) ([]SalaryTrendDataPoint, error) {
	var res []SalaryTrendDataPoint
	var rows *sql.Rows
	rows, err := conn.Query(`
	SELECT to_char(date_trunc('month', created_at), 'YYYY-MM-DD') as date, percentile_disc(0.10) within group (order by salary_max) as p10, percentile_disc(0.25) within group (order by salary_max) as p25, percentile_disc(0.50) within group (order by salary_max) as p50, percentile_disc(0.75) within group (order by salary_max) as p75, percentile_disc(0.90) within group (order by salary_max) as p90 FROM job WHERE approved_at IS NOT NULL AND salary_currency = $1 AND location ILIKE '%' || $2 || '%' AND salary_period = 'year' group by date_trunc('month', created_at) order by date_trunc('month', created_at) asc`,
		currency, location)
	if err != nil {
		return res, err
	}
	defer rows.Close()
	for rows.Next() {
		dp := SalaryTrendDataPoint{}
		err = rows.Scan(&dp.Date, &dp.P10, &dp.P25, &dp.P50, &dp.P75, &dp.P90)
		if err != nil {
			return res, err
		}
		res = append(res, dp)
	}
	err = rows.Err()
	if err != nil {
		return res, err
	}
	return res, nil
}

func SaveSEOLandingPage(conn *sql.DB, seoLandingPage SEOLandingPage) error {
	sqlStmt := `INSERT INTO seo_landing_page (uri, location, skill) VALUES ($1, $2, $3)`
	_, err := conn.Exec(sqlStmt, seoLandingPage.URI, seoLandingPage.Location, seoLandingPage.Skill)
	return err
}

func GetSEOLocations(conn *sql.DB) ([]SEOLocation, error) {
	var locations []SEOLocation
	var rows *sql.Rows
	rows, err := conn.Query(`SELECT name FROM seo_location`)
	if err != nil {
		return locations, err
	}
	defer rows.Close()
	for rows.Next() {
		loc := SEOLocation{}
		err = rows.Scan(&loc.Name)
		if err != nil {
			return locations, err
		}
		locations = append(locations, loc)
	}
	err = rows.Err()
	if err != nil {
		return locations, err
	}
	return locations, nil
}

func GetDeveloperSkills(conn *sql.DB) ([]string, error) {
	skills := make([]string, 0)
	var rows *sql.Rows
	rows, err := conn.Query(`select distinct trim(both from unnest(regexp_split_to_array(skills, ','))) as skill from developer_profile where updated_at != created_at`)
	if err != nil {
		return skills, err
	}
	defer rows.Close()
	for rows.Next() {
		var skill string
		if err := rows.Scan(&skill); err != nil {
			return skills, err
		}
		skills = append(skills, skill)
	}

	return skills, nil
}

func GetDeveloperSlugs(conn *sql.DB) ([]string, error) {
	slugs := make([]string, 0)
	var rows *sql.Rows
	rows, err := conn.Query(`select slug from developer_profile where updated_at != created_at`)
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

func SaveSEOLocation(conn *sql.DB, name, country, currency string) string {
	res := conn.QueryRow(`INSERT INTO seo_location (name, country, currency) VALUES ($1, $2, $3) on conflict do nothing returning name`, name, country, currency)
	var insert string
	res.Scan(&insert)

	return insert
}

func SaveSEOSkillFromCompany(conn *sql.DB) {
	_ = conn.QueryRow(`INSERT INTO seo_skill select distinct company from job on conflict do nothing`)
}

type Location struct {
	Name       string  `json:"name,omitempty"`
	Country    string  `json:"country,omitempty"`
	Region     string  `json:"region,omitempty"`
	Population int64   `json:"population,omitempty"`
	Lat        float64 `json:"lat,omitempty"`
	Long       float64 `json:"long,omitempty"`
	Currency   string  `json:"currency,omitempty"`
}

func GetLocation(conn *sql.DB, location string) (Location, error) {
	var country, region, lat, long, population sql.NullString
	var loc Location
	res := conn.QueryRow(`SELECT name, currency, country, region, population, lat, long FROM seo_location WHERE LOWER(name) = LOWER($1)`, location)
	err := res.Scan(&loc.Name, &loc.Currency, &country, &region, &population, &lat, &long)
	if err != nil {
		return loc, err
	}

	if country.Valid {
		countryVal, err := country.Value()
		if err != nil {
			return loc, nil
		}
		loc.Country = countryVal.(string)
	}

	if region.Valid {
		regionVal, err := region.Value()
		if err != nil {
			return loc, nil
		}
		loc.Region = regionVal.(string)
	}

	if population.Valid {
		populationVal, err := population.Value()
		if err != nil {
			return loc, nil
		}
		loc.Population, err = strconv.ParseInt(populationVal.(string), 10, 64)
		if err != nil {
			return loc, nil
		}
	}

	if lat.Valid {
		latVal, err := lat.Value()
		if err != nil {
			return loc, nil
		}
		loc.Lat, err = strconv.ParseFloat(latVal.(string), 64)
		if err != nil {
			return loc, nil
		}
	}

	if long.Valid {
		longVal, err := long.Value()
		if err != nil {
			return loc, nil
		}
		loc.Long, err = strconv.ParseFloat(longVal.(string), 64)
		if err != nil {
			return loc, nil
		}
	}

	return loc, nil
}

func GetRandomLocationsForCountry(conn *sql.DB, country string, howMany int) ([]string, error) {
	locs := make([]string, 0)
	var rows *sql.Rows
	rows, err := conn.Query(`select name from seo_location where country = $1 order by random() limit $2`, country, howMany)
	if err != nil {
		return locs, err
	}
	defer rows.Close()
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return locs, err
		}
		locs = append(locs, l)
	}

	return locs, nil
}

func GetSEOskills(conn *sql.DB) ([]SEOSkill, error) {
	var skills []SEOSkill
	var rows *sql.Rows
	rows, err := conn.Query(`SELECT name FROM seo_skill`)
	if err != nil {
		return skills, err
	}
	defer rows.Close()
	for rows.Next() {
		loc := SEOSkill{}
		if err := rows.Scan(&loc.Name); err != nil {
			return skills, err
		}
		skills = append(skills, loc)
	}
	err = rows.Err()
	if err != nil {
		return skills, err
	}
	return skills, nil
}

type User struct {
	ID                 string
	Email              string
	CreatedAtHumanised string
	CreatedAt          time.Time
	IsAdmin            bool
}

func SaveTokenSignOn(db *sql.DB, email, token string) error {
	if _, err := db.Exec(`INSERT INTO user_sign_on_token (token, email) VALUES ($1, $2)`, token, email); err != nil {
		return err
	}
	return nil
}

// GetOrCreateUserFromToken creates or get existing user given a token
// returns the user struct, whether the user existed already and an error
func GetOrCreateUserFromToken(db *sql.DB, token string) (User, bool, error) {
	u := User{}
	row := db.QueryRow(`SELECT t.token, t.email, u.id, u.email, u.created_at FROM user_sign_on_token t LEFT JOIN users u ON t.email = u.email WHERE t.token = $1`, token)
	var tokenRes, id, email, tokenEmail sql.NullString
	var createdAt sql.NullTime
	if err := row.Scan(&tokenRes, &tokenEmail, &id, &email, &createdAt); err != nil {
		return u, false, err
	}
	if !tokenRes.Valid {
		return u, false, errors.New("token not found")
	}
	if !email.Valid {
		// user not found create new one
		userID, err := ksuid.NewRandom()
		if err != nil {
			return u, false, err
		}
		u.ID = userID.String()
		u.Email = tokenEmail.String
		u.CreatedAt = time.Now()
		u.CreatedAtHumanised = humanize.Time(u.CreatedAt.UTC())
		if _, err := db.Exec(`INSERT INTO users (id, email, created_at) VALUES ($1, $2, $3)`, u.ID, u.Email, u.CreatedAt); err != nil {
			return User{}, false, err
		}

		return u, false, nil
	}
	u.ID = id.String
	u.Email = email.String
	u.CreatedAt = createdAt.Time
	u.CreatedAtHumanised = humanize.Time(u.CreatedAt.UTC())

	return u, true, nil
}

func SaveDraft(db *sql.DB, job *JobRq) (int, error) {
	externalID, err := ksuid.NewRandom()
	if err != nil {
		return 0, err
	}
	sqlStatement := `
			INSERT INTO job (job_title, company, company_url, salary_range, salary_min, salary_max, salary_currency, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, company_email, ad_type, external_id, salary_period)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, 'year') RETURNING id`
	if job.CompanyIconID != "" {
		sqlStatement = `
			INSERT INTO job (job_title, company, company_url, salary_range, salary_min, salary_max, salary_currency, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, company_email, ad_type, company_icon_image_id, external_id, salary_period)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, 'year') RETURNING id`
	}
	slugTitle := slug.Make(fmt.Sprintf("%s %s %d", job.JobTitle, job.Company, time.Now().UTC().Unix()))
	createdAt := time.Now().UTC().Unix()
	salaryMinInt, err := strconv.Atoi(strings.TrimSpace(job.SalaryMin))
	if err != nil {
		return 0, err
	}
	salaryMaxInt, err := strconv.Atoi(strings.TrimSpace(job.SalaryMax))
	if err != nil {
		return 0, err
	}
	salaryRange := SalaryToSalaryRangeString(salaryMinInt, salaryMaxInt, job.SalaryCurrency)
	var lastInsertID int
	var res *sql.Row
	if job.CompanyIconID != "" {
		res = db.QueryRow(sqlStatement, job.JobTitle, job.Company, job.CompanyURL, salaryRange, job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.Location, job.Description, job.Perks, job.InterviewProcess, job.HowToApply, time.Unix(createdAt, 0), createdAt, slugTitle, job.Email, job.AdType, job.CompanyIconID, externalID)
	} else {
		res = db.QueryRow(sqlStatement, job.JobTitle, job.Company, job.CompanyURL, salaryRange, job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.Location, job.Description, job.Perks, job.InterviewProcess, job.HowToApply, time.Unix(createdAt, 0), createdAt, slugTitle, job.Email, job.AdType, externalID)
	}
	res.Scan(&lastInsertID)
	if err != nil {
		return 0, err
	}
	return int(lastInsertID), err
}

func UpdateJob(conn *sql.DB, job *JobRqUpdate, jobID int) error {
	salaryMinInt, err := strconv.Atoi(strings.TrimSpace(job.SalaryMin))
	if err != nil {
		return err
	}
	salaryMaxInt, err := strconv.Atoi(strings.TrimSpace(job.SalaryMax))
	if err != nil {
		return err
	}
	salaryRange := SalaryToSalaryRangeString(salaryMinInt, salaryMaxInt, job.SalaryCurrency)
	_, err = conn.Exec(
		`UPDATE job SET job_title = $1, company = $2, company_url = $3, salary_min = $4, salary_max = $5, salary_currency = $6, salary_range = $7, location = $8, description = $9, perks = $10, interview_process = $11, how_to_apply = $12, company_icon_image_id = $13 WHERE id = $14`,
		job.JobTitle,
		job.Company,
		job.CompanyURL,
		job.SalaryMin,
		job.SalaryMax,
		job.SalaryCurrency,
		salaryRange,
		job.Location,
		job.Description,
		job.Perks,
		job.InterviewProcess,
		job.HowToApply,
		job.CompanyIconID,
		jobID,
	)
	if err != nil {
		return err
	}
	return err
}

func ApproveJob(conn *sql.DB, jobID int) error {
	_, err := conn.Exec(
		`UPDATE job SET approved_at = NOW() WHERE id = $1`,
		jobID,
	)
	if err != nil {
		return err
	}
	return err
}

func DisapproveJob(conn *sql.DB, jobID int) error {
	_, err := conn.Exec(
		`UPDATE job SET approved_at = NULL WHERE id = $1`,
		jobID,
	)
	if err != nil {
		return err
	}
	return err
}

func SalaryToSalaryRangeString(salaryMin, salaryMax int, currency string) string {
	salaryMinStr := fmt.Sprintf("%d", salaryMin)
	salaryMaxStr := fmt.Sprintf("%d", salaryMax)
	if currency != "₹" {
		if salaryMin > 1000 {
			salaryMinStr = fmt.Sprintf("%dk", salaryMin/1000)
		}
		if salaryMax > 1000 {
			salaryMaxStr = fmt.Sprintf("%dk", salaryMax/1000)
		}
	} else {
		if salaryMin > 100000 {
			salaryMinStr = fmt.Sprintf("%dL", salaryMin/100000)
		}
		if salaryMax > 100000 {
			salaryMaxStr = fmt.Sprintf("%dL", salaryMax/100000)
		}
	}

	return fmt.Sprintf("%s%s - %s%s", currency, salaryMinStr, currency, salaryMaxStr)
}

func CompanyExists(db *sql.DB, company string) (bool, error) {
	var count int
	row := db.QueryRow(`SELECT COUNT(*) as c FROM job WHERE company ILIKE '%` + company + `%'`)
	err := row.Scan(&count)
	if count > 0 {
		return true, err
	}

	return false, err
}

func GetViewCountForJob(conn *sql.DB, jobID int) (int, error) {
	var count int
	row := conn.QueryRow(`select count(*) as c from job_event where job_event.event_type = 'page_view' and job_event.job_id = $1`, jobID)
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, err
}

type PurchaseEvent struct {
	StripeSessionID string
	CreatedAt       time.Time
	CompletedAt     time.Time
	Amount          int
	Currency        string
	Description     string
	AdType          int
	Email           string
	JobID           int
}

func GetPurchaseEvents(conn *sql.DB, jobID int) ([]PurchaseEvent, error) {
	var purchases []PurchaseEvent
	rows, err := conn.Query(`SELECT stripe_session_id, created_at, completed_at, amount/100 as amount, currency, description, job_id FROM purchase_event WHERE job_id = $1 AND completed_at IS NOT NULL`, jobID)
	if err == sql.ErrNoRows {
		return purchases, nil
	}
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p PurchaseEvent
		if err := rows.Scan(&p.StripeSessionID, &p.CreatedAt, &p.CompletedAt, &p.Amount, &p.Currency, &p.Description, &p.JobID); err != nil {
			return purchases, err
		}
		purchases = append(purchases, p)
	}

	return purchases, nil
}

func InitiatePaymentEvent(conn *sql.DB, sessionID string, amount int64, currency string, description string, adType int64, email string, jobID int) error {
	stmt := `INSERT INTO purchase_event (stripe_session_id, amount, currency, description, ad_type, email, job_id, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`
	_, err := conn.Exec(stmt, sessionID, amount, currency, description, adType, email, jobID)
	return err
}

func SaveSuccessfulPayment(conn *sql.DB, sessionID string) (int, error) {
	res := conn.QueryRow(`WITH rows AS (UPDATE purchase_event SET completed_at = NOW() WHERE stripe_session_id = $1 AND completed_at IS NULL RETURNING 1) SELECT count(*) as c FROM rows;`, sessionID)
	var affected int
	err := res.Scan(&affected)
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func GetPurchaseEventBySessionID(conn *sql.DB, sessionID string) (PurchaseEvent, error) {
	res := conn.QueryRow(`SELECT stripe_session_id, created_at, completed_at, email, amount, currency, description, ad_type FROM purchase_event WHERE stripe_session_id = $1`, sessionID)
	var p PurchaseEvent
	err := res.Scan(&p.StripeSessionID, &p.CreatedAt, &p.CompletedAt, &p.Email, &p.Amount, &p.Currency, &p.Description, &p.AdType)
	if err != nil {
		return p, err
	}

	return p, nil
}

func GetJobByStripeSessionID(conn *sql.DB, sessionID string) (JobPost, error) {
	res := conn.QueryRow(`SELECT j.id, j.job_title, j.company, j.company_url, j.salary_range, j.location, j.how_to_apply, j.slug, j.external_id, j.approved_at FROM purchase_event p LEFT JOIN job j ON p.job_id = j.id WHERE p.stripe_session_id = $1`, sessionID)
	var job JobPost
	var approvedAt sql.NullTime
	err := res.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.HowToApply, &job.Slug, &job.ExternalID, &approvedAt)
	if err != nil {
		return job, err
	}
	if approvedAt.Valid {
		job.ApprovedAt = &approvedAt.Time
	}

	return job, nil
}

type JobStat struct {
	Date      string `json:"date"`
	Clickouts int    `json:"clickouts"`
	PageViews int    `json:"pageviews"`
}

func GetStatsForJob(conn *sql.DB, jobID int) ([]JobStat, error) {
	var stats []JobStat
	rows, err := conn.Query(`SELECT COUNT(*) FILTER (WHERE event_type = 'clickout') AS clickout, COUNT(*) FILTER (WHERE event_type = 'page_view') AS pageview, TO_CHAR(DATE_TRUNC('day', created_at), 'YYYY-MM-DD') FROM job_event WHERE job_id = $1 GROUP BY DATE_TRUNC('day', created_at) ORDER BY DATE_TRUNC('day', created_at) ASC`, jobID)
	if err == sql.ErrNoRows {
		return stats, nil
	}
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s JobStat
		if err := rows.Scan(&s.Clickouts, &s.PageViews, &s.Date); err != nil {
			return stats, err
		}
		stats = append(stats, s)
	}

	return stats, nil
}

func GetClickoutCountForJob(conn *sql.DB, jobID int) (int, error) {
	var count int
	row := conn.QueryRow(`select count(*) as c from job_event where job_event.event_type = 'clickout' and job_event.job_id = $1`, jobID)
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, err
}

func JobPostByCreatedAt(conn *sql.DB) ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := conn.Query(
		`SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
		FROM job
		WHERE approved_at IS NOT NULL
		ORDER BY created_at DESC`)
	if err != nil {
		return jobs, err
	}
	for rows.Next() {
		job := &JobPost{}
		var createdAt time.Time
		var perks, interview, companyIcon sql.NullString
		err = rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.SalaryPeriod, &job.Expired, &job.LastWeekClickouts)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if perks.Valid {
			job.Perks = perks.String
		}
		if interview.Valid {
			job.InterviewProcess = interview.String
		}
		job.TimeAgo = createdAt.UTC().Format("January 2006")
		if err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	err = rows.Err()
	if err != nil {
		return jobs, err
	}
	return jobs, nil
}

func TopNJobsByCurrencyAndLocation(conn *sql.DB, currency, location string, max int) ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := conn.Query(
		`SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period
		FROM job
		WHERE salary_currency = $1
		AND location ILIKE '%' || $2 || '%'
		AND approved_at IS NOT NULL
		ORDER BY created_at DESC LIMIT $3`, currency, location, max)
	if err != nil {
		return jobs, err
	}
	for rows.Next() {
		job := &JobPost{}
		var createdAt time.Time
		var perks, interview, companyIcon sql.NullString
		err = rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.SalaryPeriod)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if perks.Valid {
			job.Perks = perks.String
		}
		if interview.Valid {
			job.InterviewProcess = interview.String
		}
		job.TimeAgo = createdAt.UTC().Format("January 2006")
		if err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	err = rows.Err()
	if err != nil {
		return jobs, err
	}
	return jobs, nil
}

func CompanyBySlug(conn *sql.DB, slug string) (*Company, error) {
	company := &Company{}
	row := conn.QueryRow(`SELECT id, name, url, locations, last_job_created_at, icon_image_id, total_job_count, active_job_count, description, featured_post_a_job, slug FROM company WHERE slug = $1`, slug)
	if err := row.Scan(&company.ID, &company.Name, &company.URL, &company.Locations, &company.LastJobCreatedAt, &company.IconImageID, &company.TotalJobCount, &company.ActiveJobCount, &company.Description, &company.Featured, &company.Slug); err != nil {
		return company, err
	}

	return company, nil
}

func JobPostBySlug(conn *sql.DB, slug string) (*JobPost, error) {
	job := &JobPost{}
	row := conn.QueryRow(
		`SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
		FROM job
		WHERE approved_at IS NOT NULL
		AND slug = $1`, slug)
	var createdAt time.Time
	var perks, interview, companyIcon sql.NullString
	err := row.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.SalaryPeriod, &job.Expired, &job.LastWeekClickouts)
	if companyIcon.Valid {
		job.CompanyIconID = companyIcon.String
	}
	if err != nil {
		return job, err
	}
	if perks.Valid {
		job.Perks = perks.String
	}
	if interview.Valid {
		job.InterviewProcess = interview.String
	}
	job.TimeAgo = createdAt.UTC().Format("January 2006")
	return job, nil
}

func JobPostBySlugAdmin(conn *sql.DB, slug string) (*JobPost, error) {
	job := &JobPost{}
	row := conn.QueryRow(
		`SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period
		FROM job
		WHERE slug = $1`, slug)
	var createdAt time.Time
	var perks, interview, companyIcon sql.NullString
	err := row.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.SalaryPeriod)
	if companyIcon.Valid {
		job.CompanyIconID = companyIcon.String
	}
	if err != nil {
		return job, err
	}
	if perks.Valid {
		job.Perks = perks.String
	}
	if interview.Valid {
		job.InterviewProcess = interview.String
	}
	job.TimeAgo = createdAt.UTC().Format("January 2006")
	return job, nil
}

func JobPostByIDForEdit(conn *sql.DB, jobID int) (*JobPostForEdit, error) {
	job := &JobPostForEdit{}
	row := conn.QueryRow(
		`SELECT job_title, company, company_email, company_url, salary_min, salary_max, salary_currency, location, description, perks, interview_process, how_to_apply, created_at, slug, approved_at, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period
		FROM job
		WHERE id = $1`, jobID)
	var perks, interview, companyURL, companyIconID sql.NullString
	err := row.Scan(&job.JobTitle, &job.Company, &job.CompanyEmail, &companyURL, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &job.CreatedAt, &job.Slug, &job.ApprovedAt, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIconID, &job.ExternalID, &job.SalaryPeriod)
	if err != nil {
		return job, err
	}
	if companyIconID.Valid {
		job.CompanyIconID = companyIconID.String
	}
	if perks.Valid {
		job.Perks = perks.String
	}
	if interview.Valid {
		job.InterviewProcess = interview.String
	}
	if companyURL.Valid {
		job.CompanyURL = companyURL.String
	} else {
		job.CompanyURL = ""
	}
	return job, nil
}

func JobPostByExternalIDForEdit(conn *sql.DB, externalID string) (*JobPostForEdit, error) {
	job := &JobPostForEdit{}
	row := conn.QueryRow(
		`SELECT id, job_title, company, company_email, company_url, salary_min, salary_max, salary_currency, location, description, perks, interview_process, how_to_apply, created_at, slug, approved_at, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period
		FROM job
		WHERE external_id = $1`, externalID)
	var perks, interview, companyURL, companyIconID sql.NullString
	err := row.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyEmail, &companyURL, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &job.CreatedAt, &job.Slug, &job.ApprovedAt, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIconID, &job.ExternalID, &job.SalaryPeriod)
	if err != nil {
		return job, err
	}
	if companyIconID.Valid {
		job.CompanyIconID = companyIconID.String
	}
	if perks.Valid {
		job.Perks = perks.String
	}
	if interview.Valid {
		job.InterviewProcess = interview.String
	}
	if companyURL.Valid {
		job.CompanyURL = companyURL.String
	} else {
		job.CompanyURL = ""
	}
	return job, nil
}

func JobPostByURLID(conn *sql.DB, URLID int64) (*JobPost, error) {
	job := &JobPost{}
	row := conn.QueryRow(
		`SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
		FROM job
		WHERE approved_at IS NOT NULL
		AND url_id = $1`, URLID)
	var createdAt time.Time
	var perks, interview, companyIcon sql.NullString
	err := row.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.SalaryPeriod, &job.Expired, &job.LastWeekClickouts)
	if err != nil {
		return job, err
	}
	if companyIcon.Valid {
		job.CompanyIconID = companyIcon.String
	}
	if perks.Valid {
		job.Perks = perks.String
	}
	if interview.Valid {
		job.InterviewProcess = interview.String
	}
	job.TimeAgo = createdAt.UTC().Format("January 2006")
	return job, nil
}

func DeleteJobCascade(conn *sql.DB, jobID int) error {
	if _, err := conn.Exec(
		`DELETE FROM image WHERE id IN (SELECT company_icon_image_id FROM job WHERE id = $1)`,
		jobID,
	); err != nil {
		return err
	}
	if _, err := conn.Exec(
		`DELETE FROM edit_token WHERE job_id = $1`,
		jobID,
	); err != nil {
		return err
	}
	if _, err := conn.Exec(
		`DELETE FROM apply_token WHERE job_id = $1`,
		jobID,
	); err != nil {
		return err
	}
	if _, err := conn.Exec(
		`DELETE FROM job_event WHERE job_id = $1`,
		jobID,
	); err != nil {
		return err
	}
	if _, err := conn.Exec(
		`DELETE FROM purchase_event WHERE job_id = $1`,
		jobID,
	); err != nil {
		return err
	}
	if _, err := conn.Exec(
		`DELETE FROM job WHERE id = $1`,
		jobID,
	); err != nil {
		return err
	}
	return nil
}

func GetPendingJobs(conn *sql.DB) ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := conn.Query(`
	SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period
		FROM job WHERE approved_at IS NULL`)
	if err == sql.ErrNoRows {
		return jobs, nil
	}
	if err != nil {
		return jobs, err
	}
	defer rows.Close()
	for rows.Next() {
		job := &JobPost{}
		var createdAt time.Time
		var perks, interview, companyIcon sql.NullString
		err = rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.SalaryPeriod)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if perks.Valid {
			job.Perks = perks.String
		}
		if interview.Valid {
			job.InterviewProcess = interview.String
		}
		job.TimeAgo = createdAt.UTC().Format("January 2006")
		if err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	err = rows.Err()
	if err != nil {
		return jobs, err
	}
	return jobs, nil
}

// GetCompanyJobs returns jobs for a given company
func GetCompanyJobs(conn *sql.DB, companyName string, limit int) ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := conn.Query(`
	SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, last_week_clickouts
		FROM job WHERE approved_at IS NOT NULL AND expired IS FALSE AND company = $1 ORDER BY ad_type DESC, approved_at DESC LIMIT $2`, companyName, limit)
	if err != nil {
		return jobs, err
	}
	defer rows.Close()
	for rows.Next() {
		job := &JobPost{}
		var createdAt time.Time
		var perks, interview, companyIcon sql.NullString
		err = rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.SalaryPeriod, &job.LastWeekClickouts)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if perks.Valid {
			job.Perks = perks.String
		}
		if interview.Valid {
			job.InterviewProcess = interview.String
		}
		job.TimeAgo = createdAt.UTC().Format("January 2006")
		if err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	err = rows.Err()
	if err != nil {
		return jobs, err
	}
	return jobs, nil
}

// GetRelevantJobs returns pinned and most recent jobs for now
func GetRelevantJobs(conn *sql.DB, location string, jobID int, limit int) ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := conn.Query(`
	SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, last_week_clickouts
		FROM job WHERE approved_at IS NOT NULL AND id != $1 AND expired IS FALSE ORDER BY ad_type DESC, approved_at DESC, word_similarity($2, location) LIMIT $3`, jobID, location, limit)
	if err != nil {
		return jobs, err
	}
	defer rows.Close()
	for rows.Next() {
		job := &JobPost{}
		var createdAt time.Time
		var perks, interview, companyIcon sql.NullString
		err = rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.SalaryPeriod, &job.LastWeekClickouts)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if perks.Valid {
			job.Perks = perks.String
		}
		if interview.Valid {
			job.InterviewProcess = interview.String
		}
		job.TimeAgo = createdAt.UTC().Format("January 2006")
		if err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	err = rows.Err()
	if err != nil {
		return jobs, err
	}
	return jobs, nil
}

func GetPinnedJobs(conn *sql.DB) ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := conn.Query(`
	SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, last_week_clickouts
		FROM job WHERE approved_at IS NOT NULL AND ad_type IN (2, 3, 5) ORDER BY approved_at DESC`)
	if err != nil {
		return jobs, err
	}
	defer rows.Close()
	for rows.Next() {
		job := &JobPost{}
		var createdAt time.Time
		var perks, interview, companyIcon sql.NullString
		err = rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.SalaryPeriod, &job.LastWeekClickouts)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if perks.Valid {
			job.Perks = perks.String
		}
		if interview.Valid {
			job.InterviewProcess = interview.String
		}
		job.TimeAgo = createdAt.UTC().Format("January 2006")
		if err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	err = rows.Err()
	if err != nil {
		return jobs, err
	}
	return jobs, nil
}

func JobsByQuery(conn *sql.DB, location, tag string, pageId, jobsPerPage int) ([]*JobPost, int, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	offset := pageId*jobsPerPage - jobsPerPage
	// replace `|` with white space
	// remove double white spaces
	// join with `|` for ps query
	tag = strings.Join(strings.Fields(strings.ReplaceAll(tag, "|", " ")), "|")
	rows, err := getQueryForArgs(conn, location, tag, offset, jobsPerPage)
	if err != nil {
		return jobs, 0, err
	}
	defer rows.Close()
	var fullRowsCount int
	for rows.Next() {
		job := &JobPost{}
		var createdAt time.Time
		var perks, interview, companyIcon sql.NullString
		err = rows.Scan(&fullRowsCount, &job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.SalaryPeriod, &job.Expired, &job.LastWeekClickouts)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if perks.Valid {
			job.Perks = perks.String
		}
		if interview.Valid {
			job.InterviewProcess = interview.String
		}
		job.TimeAgo = createdAt.UTC().Format("January 2006")
		if err != nil {
			return jobs, fullRowsCount, err
		}
		jobs = append(jobs, job)
	}
	err = rows.Err()
	if err != nil {
		return jobs, fullRowsCount, err
	}
	return jobs, fullRowsCount, nil
}

func CompaniesByQuery(conn *sql.DB, location string, pageID, companiesPerPage int) ([]Company, int, error) {
	companies := []Company{}
	var rows *sql.Rows
	offset := pageID*companiesPerPage - companiesPerPage
	rows, err := getCompanyQueryForArgs(conn, location, offset, companiesPerPage)
	if err != nil {
		return companies, 0, err
	}
	defer rows.Close()
	var fullRowsCount int
	for rows.Next() {
		c := Company{}
		var nullStr sql.NullString
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
			&nullStr,
		)
		if nullStr.Valid {
			c.Description = &nullStr.String
		}
		companies = append(companies, c)
	}
	err = rows.Err()
	if err != nil {
		return companies, fullRowsCount, err
	}
	return companies, fullRowsCount, nil
}

func FeaturedCompaniesPostAJob(conn *sql.DB) ([]Company, error) {
	companies := []Company{}
	rows, err := conn.Query(`SELECT name, icon_image_id FROM company WHERE featured_post_a_job IS TRUE LIMIT 15`)
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
		companies = append(companies, c)
	}
	err = rows.Err()
	if err != nil {
		return companies, err
	}
	return companies, nil
}
func TokenByJobID(conn *sql.DB, jobID int) (string, error) {
	tokenRow := conn.QueryRow(
		`SELECT token
		FROM edit_token
		WHERE job_id = $1`, jobID)
	var token string
	err := tokenRow.Scan(&token)
	return token, err
}

func JobPostIDByToken(conn *sql.DB, token string) (int, error) {
	row := conn.QueryRow(
		`SELECT job_id
		FROM edit_token
		WHERE token = $1`, token)
	var jobID int
	err := row.Scan(&jobID)
	if err != nil {
		return 0, err
	}
	return jobID, nil
}

func LastJobPosted(conn *sql.DB) (time.Time, error) {
	row := conn.QueryRow(`SELECT created_at FROM job WHERE approved_at IS NOT NULL ORDER BY created_at DESC LIMIT 1`)
	var last time.Time
	if err := row.Scan(&last); err != nil {
		return last, err
	}

	return last, nil
}

func SaveTokenForJob(conn *sql.DB, token string, jobID int) error {
	_, err := conn.Exec(`INSERT INTO edit_token (token, job_id, created_at) VALUES ($1, $2, $3)`, token, jobID, time.Now().UTC())
	if err != nil {
		return err
	}
	return err
}

func getQueryForArgs(conn *sql.DB, location, tag string, offset, max int) (*sql.Rows, error) {
	if tag == "" && location == "" {
		return conn.Query(`
		SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
		FROM job
		WHERE approved_at IS NOT NULL
		AND ad_type not in (2, 3, 5)
		ORDER BY created_at DESC LIMIT $2 OFFSET $1`, offset, max)
	}
	if tag == "" && location != "" {
		return conn.Query(`
		SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts 
		FROM job
		WHERE approved_at IS NOT NULL
		AND ad_type not in (2, 3, 5)
		AND location ILIKE '%' || $1 || '%'
		ORDER BY created_at DESC LIMIT $3 OFFSET $2`, location, offset, max)
	}
	if tag != "" && location == "" {
		return conn.Query(`
	SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
	FROM
	(
		SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts, to_tsvector(job_title) || to_tsvector(company) || to_tsvector(description) AS doc
		FROM job WHERE approved_at IS NOT NULL AND ad_type not in (2, 3, 5)
	) AS job_
	WHERE job_.doc @@ to_tsquery($1)
	ORDER BY ts_rank(job_.doc, to_tsquery($1)) DESC, created_at DESC LIMIT $3 OFFSET $2`, tag, offset, max)
	}

	return conn.Query(`
	SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
	FROM
	(
		SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts, to_tsvector(job_title) || to_tsvector(company) || to_tsvector(description) AS doc
		FROM job WHERE approved_at IS NOT NULL AND ad_type not in (2, 3, 5)
	) AS job_
	WHERE job_.doc @@ to_tsquery($1)
	AND location ILIKE '%' || $2 || '%'
	ORDER BY ts_rank(job_.doc, to_tsquery($1)) DESC, created_at DESC LIMIT $4 OFFSET $3`, tag, location, offset, max)
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
       description 
FROM   company
ORDER  BY last_job_created_at DESC
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
       description 
FROM   company
WHERE locations ILIKE '%' || $1 || '%'
ORDER  BY last_job_created_at DESC
LIMIT $3 OFFSET $2`, location, offset, max)
}

func GetValue(conn *sql.DB, key string) (string, error) {
	res := conn.QueryRow(`SELECT value FROM meta WHERE key = $1`, key)
	var val string
	err := res.Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

func SetValue(conn *sql.DB, key, val string) error {
	_, err := conn.Exec(`UPDATE meta SET value = $1 WHERE key = $2`, val, key)
	return err
}

func GetLastNJobs(conn *sql.DB, max int, loc string) ([]*JobPost, error) {
	var jobs []*JobPost
	var rows *sql.Rows
	var err error
	if strings.TrimSpace(loc) == "" {
		rows, err = conn.Query(`SELECT id, job_title, description, company, salary_range, location, slug, salary_currency, company_icon_image_id, external_id, approved_at, salary_period FROM job WHERE approved_at IS NOT NULL ORDER BY approved_at DESC LIMIT $1`, max)
	} else {
		rows, err = conn.Query(`SELECT
	id, job_title, description, company, salary_range, location, slug, salary_currency, company_icon_image_id, external_id, approved_at, salary_period
	FROM
	job
	WHERE
	approved_at IS NOT NULL
	AND location ILIKE '%' || $1 || '%'
	ORDER BY approved_at DESC LIMIT $2`, loc, max)
	}
	if err != nil {
		return jobs, err
	}
	for rows.Next() {
		job := &JobPost{}
		var companyIcon sql.NullString
		err := rows.Scan(&job.ID, &job.JobTitle, &job.JobDescription, &job.Company, &job.SalaryRange, &job.Location, &job.Slug, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.ApprovedAt, &job.SalaryPeriod)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func GetLastNJobsFromID(conn *sql.DB, max, jobID int) ([]*JobPost, error) {
	var jobs []*JobPost
	var rows *sql.Rows
	rows, err := conn.Query(`SELECT id, job_title, company, salary_range, location, slug, salary_currency, company_icon_image_id, external_id, salary_period FROM job WHERE id > $1 AND approved_at IS NOT NULL LIMIT $2`, jobID, max)
	if err != nil {
		return jobs, err
	}
	for rows.Next() {
		job := &JobPost{}
		var companyIcon sql.NullString
		err := rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.SalaryRange, &job.Location, &job.Slug, &job.SalaryCurrency, &companyIcon, &job.ExternalID, &job.SalaryPeriod)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if err != nil {
			return jobs, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func CleanupExpiredApplyTokens(conn *sql.DB) error {
	_, err := conn.Exec(
		`DELETE FROM apply_token WHERE created_at < NOW() - INTERVAL '3 days' OR confirmed_at IS NOT NULL`,
	)
	return err
}

type Media struct {
	Bytes     []byte
	MediaType string
}

func SaveMedia(conn *sql.DB, media Media) (string, error) {
	mediaID, err := ksuid.NewRandom()
	if err != nil {
		return "", err
	}
	_, err = conn.Exec(`INSERT INTO image (id, bytes, media_type) VALUES ($1, $2, $3)`, mediaID.String(), media.Bytes, media.MediaType)
	if err != nil {
		return "", err
	}
	return mediaID.String(), nil
}

type CloudflareStat struct {
	Date        time.Time
	Bytes       uint64
	CachedBytes uint64
	PageViews   uint64
	Requests    uint64
	Threats     uint64
	Uniques     uint64
}

func SaveCloudflareStat(conn *sql.DB, s CloudflareStat) error {
	_, err := conn.Exec(
		`INSERT INTO cloudflare_stats VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT DO NOTHING`,
		s.Date.Format("2006-01-02"),
		s.Bytes,
		s.CachedBytes,
		s.PageViews,
		s.Requests,
		s.Threats,
		s.Uniques,
	)
	return err
}

type CloudflareStatusCodeStat struct {
	Date       time.Time
	StatusCode int
	Requests   uint64
}

func SaveCloudflareStatusCodeStat(conn *sql.DB, s CloudflareStatusCodeStat) error {
	_, err := conn.Exec(
		`INSERT INTO cloudflare_status_code_stats VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		s.Date.Format("2006-01-02"),
		s.StatusCode,
		s.Requests,
	)
	return err
}

type CloudflareCountryStat struct {
	Date        time.Time
	CountryCode string
	Requests    uint64
	Threats     uint64
}

func SaveCloudflareCountryStat(conn *sql.DB, s CloudflareCountryStat) error {
	_, err := conn.Exec(
		`INSERT INTO cloudflare_country_stats VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`,
		s.Date.Format("2006-01-02"),
		s.CountryCode,
		s.Requests,
		s.Threats,
	)
	return err
}

type CloudflareBrowserStat struct {
	Date            time.Time
	PageViews       uint64
	UABrowserFamily string
}

func SaveCloudflareBrowserStat(conn *sql.DB, s CloudflareBrowserStat) error {
	_, err := conn.Exec(
		`INSERT INTO cloudflare_browser_stats VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		s.Date.Format("2006-01-02"),
		s.PageViews,
		s.UABrowserFamily,
	)
	return err
}

func UpdateMedia(conn *sql.DB, media Media, mediaID string) error {
	_, err := conn.Exec(`UPDATE image SET bytes = $1, media_type = $2 WHERE id = $3`, media.Bytes, media.MediaType, mediaID)
	return err
}

func GetMediaByID(conn *sql.DB, mediaID string) (Media, error) {
	var m Media
	row := conn.QueryRow(
		`SELECT bytes, media_type 
		FROM image
		WHERE id = $1`, mediaID)
	err := row.Scan(&m.Bytes, &m.MediaType)
	if err != nil {
		return Media{}, err
	}
	return m, nil
}

type SitemapEntry struct {
	Loc        string
	ChangeFreq string
	LastMod    time.Time
}

const SitemapSize = 1000

func GetSitemapEntryCount(conn *sql.DB) (int, error) {
	var count int
	row := conn.QueryRow(`SELECT COUNT(*) as c FROM sitemap`)
	if err := row.Scan(&count); err != nil {
		return count, err
	}

	return count, nil
}

func NewJobsLastWeekOrMonth(conn *sql.DB) (int, int, error) {
	var week, month int
	row := conn.QueryRow(`select lastweek.c as week, lastmonth.c as month 
from 
(select count(*) as c, 1 as id from job  where approved_at >= (now() - '7 days'::interval)::date) as lastweek
left join 
(select count(*) as c, 1 as id from job  where approved_at >= (now() - '30 days'::interval)::date) as lastmonth on lastmonth.id = lastweek.id 
`)
	if err := row.Scan(&week, &month); err != nil {
		return week, month, err
	}
	return week, month, nil
}

func GetSitemapLastMod(conn *sql.DB) (time.Time, error) {
	var lastMod time.Time
	row := conn.QueryRow(`SELECT lastmod FROM sitemap ORDER BY lastmod DESC LIMIT 1`)
	if err := row.Scan(&lastMod); err != nil {
		return lastMod, err
	}

	return lastMod, nil
}

func GetWebsitePageViewsLast30Days(conn *sql.DB) (int, error) {
	var c int
	row := conn.QueryRow(`SELECT SUM(page_views) AS c FROM cloudflare_browser_stats WHERE date > CURRENT_DATE - 30 AND ua_browser_family NOT ILIKE '%bot%'`)
	if err := row.Scan(&c); err != nil {
		return 100000, nil
	}

	return c, nil
}

func GetJobPageViewsLast30Days(conn *sql.DB) (int, error) {
	var c int
	row := conn.QueryRow(`SELECT COUNT(*) AS c FROM job_event WHERE event_type = 'page_view' AND created_at > CURRENT_DATE - 30`)
	if err := row.Scan(&c); err != nil {
		return 100000, nil
	}

	return c, nil
}

func GetJobClickoutsLast30Days(conn *sql.DB) (int, error) {
	var c int
	row := conn.QueryRow(`SELECT COUNT(*) AS c FROM job_event WHERE event_type = 'clickout' AND created_at > CURRENT_DATE - 30`)
	if err := row.Scan(&c); err != nil {
		return 100000, nil
	}

	return c, nil
}

func GetSitemapIndex(conn *sql.DB) ([]SitemapEntry, error) {
	entries := make([]SitemapEntry, 0, 20)
	count, err := GetSitemapEntryCount(conn)
	if err != nil {
		return entries, err
	}
	lastMod, err := GetSitemapLastMod(conn)
	if err != nil {
		return entries, err
	}
	slots := math.Ceil(float64(count) / float64(SitemapSize))
	for i := 0; i <= int(slots); i++ {
		entries = append(entries, SitemapEntry{
			Loc:     fmt.Sprintf("https://golang.cafe/sitemap-%d.xml", i),
			LastMod: lastMod,
		})
	}

	return entries, nil
}

func GetSitemapNo(conn *sql.DB, n int) ([]SitemapEntry, error) {
	entries := make([]SitemapEntry, 0, SitemapSize)
	offset := (n - 1) * SitemapSize
	var rows *sql.Rows
	rows, err := conn.Query(`SELECT * FROM sitemap LIMIT $1 OFFSET $2`, SitemapSize, offset)
	if err != nil {
		return entries, err
	}
	for rows.Next() {
		var entry SitemapEntry
		if err := rows.Scan(&entry.Loc, &entry.ChangeFreq, &entry.LastMod); err != nil {
			return entries, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func SaveSitemapEntry(conn *sql.DB, entry SitemapEntry) error {
	_, err := conn.Exec(`INSERT INTO sitemap_tmp VALUES ($1, $2, $3)`, entry.Loc, entry.ChangeFreq, entry.LastMod)
	return err
}

func CreateTmpSitemapTable(conn *sql.DB) error {
	_, err := conn.Exec(`CREATE TABLE sitemap_tmp AS TABLE sitemap WITH NO DATA;`)
	return err
}

func SwapSitemapTable(conn *sql.DB) error {
	_, err := conn.Exec(`BEGIN; ALTER TABLE sitemap RENAME TO sitemap_old; ALTER TABLE sitemap_tmp RENAME TO sitemap; DROP TABLE sitemap_old; COMMIT;`)
	return err
}

func MarkJobAsExpired(conn *sql.DB, jobID int) error {
	_, err := conn.Exec(`UPDATE job SET expired = true WHERE id = $1`, jobID)
	return err
}

func UpdateLastWeekClickouts(conn *sql.DB) error {
	_, err := conn.Exec(`WITH cte AS (SELECT job_id, count(*) AS clickouts FROM job_event WHERE event_type = 'clickout' AND created_at > CURRENT_DATE - 7 GROUP BY job_id)
UPDATE job SET last_week_clickouts = cte.clickouts FROM cte WHERE cte.job_id = id`)
	return err
}

type JobApplyURL struct {
	ID  int
	URL string
}

func GetJobApplyURLs(conn *sql.DB) ([]JobApplyURL, error) {
	jobURLs := make([]JobApplyURL, 0)
	var rows *sql.Rows
	rows, err := conn.Query(`SELECT id, how_to_apply FROM job WHERE approved_at IS NOT NULL AND expired = false`)
	if err != nil {
		return jobURLs, err
	}
	for rows.Next() {
		jobURL := JobApplyURL{}
		if err := rows.Scan(&jobURL.ID, &jobURL.URL); err != nil {
			return jobURLs, err
		}
		jobURLs = append(jobURLs, jobURL)
	}
	return jobURLs, nil
}
