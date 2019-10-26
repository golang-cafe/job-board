package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/gosimple/slug"
	"github.com/lib/pq"
	"github.com/segmentio/ksuid"
)

type Job struct {
	CreatedAt                                                                                  int64
	JobTitle, Company, SalaryRange, Location, Description, Perks, InterviewProcess, HowToApply string
	Email                                                                                      string
}

type JobRq struct {
	JobTitle           string `json:"job_title"`
	Location           string `json:"job_location"`
	Company            string `json:"company_name"`
	CompanyURL         string `json:"company_url"`
	SalaryMin          string `json:"salary_min"`
	SalaryMax          string `json:"salary_max"`
	SalaryCurrency     string `json:"salary_currency"`
	Description        string `json:"job_description"`
	HowToApply         string `json:"how_to_apply"`
	Perks              string `json:"perks"`
	AffiliateReference string `json:"ref,omitempty"`
	InterviewProcess   string `json:"interview_process,omitempty"`
	Email              string `json:"company_email"`
	StripeToken        string `json:"stripe_token,omitempty"`
	AdType             int64  `json:"ad_type"`
	CurrencyCode       string `json:"currency_code"`
	CompanyIconID      string `json:"company_icon_id,omitempty"`
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
}

type JobPost struct {
	ID               int
	CreatedAt        int64
	TimeAgo          string
	JobTitle         string
	Company          string
	CompanyURL       string
	SalaryRange      string
	Location         string
	JobDescription   string
	Perks            string
	InterviewProcess string
	HowToApply       string
	Slug             string
	SalaryCurrency   string
	AdType           int64
	SalaryMin        int64
	SalaryMax        int64
	CompanyIconID    string
}

type JobPostForEdit struct {
	JobTitle, Company, CompanyEmail, CompanyURL, Location                     string
	SalaryMin, SalaryMax                                                      int
	SalaryCurrency, JobDescription, Perks, InterviewProcess, HowToApply, Slug string
	CreatedAt                                                                 time.Time
	ApprovedAt                                                                pq.NullTime
	AdType                                                                    int64
	CompanyIconID                                                             string
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
	Name string
}

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
// ALTER TABLE job ADD COLUMN ad_type INTEGER NOT NULL DEFAULT 0;
// ALTER TABLE job ALTER COLUMN company_url SET NOT NULL;
// ALTER TABLE job ADD COLUMN company_icon_image_id VARCHAR(255) DEFAULT NULL;

// CREATE TABLE IF NOT EXISTS image (
// 	id CHAR(27) NOT NULL UNIQUE,
// 	bytes BYTEA NOT NULL,
// 	PRIMARY KEY(id)
// )
// ALTER TABLE image ADD COLUMN media_type VARCHAR(100) NOT NULL;

// CREATE TABLE IF NOT EXISTS edit_token (
//   token      CHAR(27) NOT NULL,
//   job_id     INTEGER NOT NULL REFERENCES job (id),
//   created_at TIMESTAMP NOT NULL
// );
// CREATE UNIQUE INDEX token_idx on edit_token (token);

// CREATE TABLE IF NOT EXISTS purchase_event (
// 	stripe_session_id VARCHAR(255) NOT NULL,
//  amount INTEGER NOT NULL,
//  currency CHAR(3) NOT NULL,
// 	created_at TIMESTAMP NOT NULL,
// 	completed_at TIMESTAMP DEFAULT NULL
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

// CREATE TABLE IF NOT EXISTS job_event (
// 	event_type VARCHAR(128) NOT NULL,
// 	job_id INTEGER NOT NULL REFERENCES job (id),
// 	created_at TIMESTAMP NOT NULL
// );

// CREATE INDEX job_idx ON job_event (job_id);

// CREATE TABLE IF NOT EXISTS affiliate_event_log (
// 	affiliate_id VARCHAR(255) NOT NULL,
// 	type VARCHAR(128) NOT NULL,
//  meta JSON DEFAULT NULL,
// 	created_at TIMESTAMP NOT NULL
// );

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

const (
	jobEventPageView = "page_view"
	jobEventClickout = "clickout"
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
	return db, nil
}

// CloseDbConn closes db conn
func CloseDbConn(conn *sql.DB) {
	conn.Close()
}

func TrackJobView(conn *sql.DB, job *JobPost) error {
	stmt := `INSERT INTO job_event (event_type, job_id, created_at) VALUES ($1, $2, NOW())`
	_, err := conn.Exec(stmt, jobEventPageView, job.ID)
	return err
}

func ApplyToJob(conn *sql.DB, jobID int, cv []byte, email, token string) error {
	stmt := `INSERT INTO apply_token (token, job_id, created_at, email, cv) VALUES ($1, $2, NOW(), $3, $4)`
	_, err := conn.Exec(stmt, token, jobID, email, cv)
	return err
}

func SavePaymentEvent(conn *sql.DB, sessionID string, amount int64, currency string, jobID int) error {
	stmt := `INSERT INTO purchase_event (stripe_session_id, amount, currency, job_id, created_at) VALUES ($1, $2, $3, $4, NOW())`
	_, err := conn.Exec(stmt, sessionID, amount, currency, jobID)
	return err
}

func ConfirmApplyToJob(conn *sql.DB, token string) error {
	_, err := conn.Exec(
		`UPDATE apply_token SET confirmed_at = NOW() WHERE token = $1`,
		token,
	)
	return err
}

type Applicant struct {
	Cv    []byte
	Email string
}

func GetJobByApplyToken(conn *sql.DB, token string) (JobPost, Applicant, error) {
	res := conn.QueryRow(`SELECT t.cv, t.email, j.id, j.job_title, j.company, company_url, salary_range, location, how_to_apply, slug
	FROM job j JOIN apply_token t ON t.job_id = j.id AND t.token = $1 WHERE j.approved_at IS NOT NULL AND t.created_at < NOW() + INTERVAL '3 days' AND t.confirmed_at IS NULL`, token)
	job := JobPost{}
	applicant := Applicant{}
	err := res.Scan(&applicant.Cv, &applicant.Email, &job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.HowToApply, &job.Slug)
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

type JobAdType int

const (
	JobAdBasic = iota
	JobAdSponsoredBackground
	JobAdSponsoredPinnedFor30Days
	JobAdSponsoredPinnedFor7Days
	JobAdWithCompanyLogo
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

type SalaryDataPoint struct {
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

func GetSalaryDataForLocationAndCurrency(conn *sql.DB, location, currency string) ([]SalaryDataPoint, error) {
	var res []SalaryDataPoint
	var rows *sql.Rows
	rows, err := conn.Query(`
	SELECT salary_min, salary_max
		FROM job WHERE approved_at IS NOT NULL AND salary_currency = $1 AND location ILIKE '%' || $2 || '%'`, currency, location)
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

func SaveSEOLocation(conn *sql.DB, name, country, currency string) string {
	res := conn.QueryRow(`INSERT INTO seo_location (name, country, currency) VALUES ($1, $2, $3) on conflict do nothing returning name`, name, country, currency)
	var insert string
	res.Scan(&insert)

	return insert
}

func SaveSEOSkillFromCompany(conn *sql.DB) {
	_ = conn.QueryRow(`INSERT INTO seo_skill select distinct company from job on conflict do nothing`)
}

func GetLocation(conn *sql.DB, location string) (string, string, string, error) {
	var loc string
	var currency string
	var country sql.NullString
	res := conn.QueryRow(`SELECT name, currency, country FROM seo_location WHERE name = $1`, location)
	err := res.Scan(&loc, &currency, &country)
	if err != nil {
		return "", "", "", err
	}

	if country.Valid {
		countryVal, err := country.Value()
		if err != nil {
			return loc, currency, "", nil
		}
		return loc, currency, countryVal.(string), nil
	}

	return loc, currency, "", nil
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
		err = rows.Scan(&loc.Name)
		if err != nil {
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

type AffiliateLogMeta struct {
	JobID    int    `json:"job_id"`
	AdType   int    `json:"ad_type"`
	Amount   int    `json:"amount"`
	Currency string `json:"currency"`
}

func SaveAffiliateLog(conn *sql.DB, jobID int, adType int64, amount int64, currencyCode string, affiliateRef string) error {
	stmt := `INSERT INTO affiliate_event_log (affiliate_id, type, meta, created_at) VALUES ($1, $2, $3, NOW())`
	metaStruct := AffiliateLogMeta{jobID, int(adType), int(amount), currencyCode}
	meta, err := json.Marshal(metaStruct)
	if err != nil {
		return err
	}
	_, err = conn.Exec(stmt, affiliateRef, "conversion", meta)
	return err
}

func SaveAffiliatePostAJobView(conn *sql.DB, affiliateRef string) error {
	stmt := `INSERT INTO affiliate_event_log (affiliate_id, type, created_at) VALUES ($1, $2, NOW())`
	_, err := conn.Exec(stmt, affiliateRef, "pageview")
	return err
}

func GetAffiliatePageViews(conn *sql.DB, affiliateID string) (int, error) {
	stmt := `select count(*) from affiliate_event_log where affiliate_id = $1 and type = 'pageview'`
	row := conn.QueryRow(stmt, affiliateID)
	var count int
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

type AffiliateConversion struct {
	JobID            int
	GrossRevenue     float64
	NetRevenue       float64
	AffiliateRevenue float64
	Currency         string
	CreatedAt        time.Time
}

func GetAffiliateConversions(conn *sql.DB, affiliateID string) ([]AffiliateConversion, error) {
	var affiliateConversions []AffiliateConversion
	stmt := `select meta->'job_id' as job_id,meta->'amount' as amount,meta->'currency' as currency, created_at from affiliate_event_log where affiliate_id = $1 and type = 'conversion'`
	res, err := conn.Query(stmt, affiliateID)
	if err != nil {
		return affiliateConversions, err
	}
	for res.Next() {
		var affiliateConversion AffiliateConversion
		res.Scan(&affiliateConversion.JobID, &affiliateConversion.GrossRevenue, &affiliateConversion.Currency, &affiliateConversion.CreatedAt)
		affiliateConversion.GrossRevenue = affiliateConversion.GrossRevenue / 100
		if affiliateConversion.Currency == "USD" {
			affiliateConversion.NetRevenue = (affiliateConversion.GrossRevenue - 0.2) - (affiliateConversion.GrossRevenue * 0.0029)
		} else {
			affiliateConversion.NetRevenue = (affiliateConversion.GrossRevenue - 0.2) - (affiliateConversion.GrossRevenue * 0.0014)
		}
		if affiliateConversion.GrossRevenue == 0 {
			affiliateConversion.NetRevenue = 0
		}
		affiliateConversion.AffiliateRevenue = affiliateConversion.NetRevenue * 0.3

		affiliateConversions = append(affiliateConversions, affiliateConversion)
	}
	return affiliateConversions, nil
}

func SaveDraft(db *sql.DB, job *JobRq) (int, error) {
	sqlStatement := `
			INSERT INTO job (job_title, company, company_url, salary_range, salary_min, salary_max, salary_currency, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, company_email, ad_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17) RETURNING id`
	if job.CompanyIconID != "" {
		sqlStatement = `
			INSERT INTO job (job_title, company, company_url, salary_range, salary_min, salary_max, salary_currency, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, company_email, ad_type, company_icon_image_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18) RETURNING id`
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
		res = db.QueryRow(sqlStatement, job.JobTitle, job.Company, job.CompanyURL, salaryRange, job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.Location, job.Description, job.Perks, job.InterviewProcess, job.HowToApply, time.Unix(createdAt, 0), createdAt, slugTitle, job.Email, job.AdType, job.CompanyIconID)
	} else {
		res = db.QueryRow(sqlStatement, job.JobTitle, job.Company, job.CompanyURL, salaryRange, job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.Location, job.Description, job.Perks, job.InterviewProcess, job.HowToApply, time.Unix(createdAt, 0), createdAt, slugTitle, job.Email, job.AdType)
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

func SaveScrapedAngelistJob(db *sql.DB, job *ScrapedJob) error {
	sqlStatement := `
		INSERT INTO job (job_title, company, company_url, company_twitter, salary_range, salary_currency, location, description, how_to_apply, created_at, url_id, slug)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`
	slugTitle := slug.Make(fmt.Sprintf("%s %s %d", job.JobTitle, job.Company, time.Now().UTC().Unix()))
	fmt.Printf("%s\n", slugTitle)
	_, err := db.Exec(sqlStatement, job.JobTitle, job.Company, job.CompanyWebsite, job.CompanyTwitter, job.Salary, job.Currency, job.Location, job.Description, job.Href, time.Now().UTC(), time.Now().UTC().Unix(), slugTitle)
	return err
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
		`SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id
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
		err = rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if perks.Valid {
			job.Perks = perks.String
		}
		if interview.Valid {
			job.InterviewProcess = interview.String
		}
		job.TimeAgo = humanize.Time(createdAt.UTC())
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

func JobPostBySlug(conn *sql.DB, slug string) (*JobPost, error) {
	job := &JobPost{}
	row := conn.QueryRow(
		`SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id
		FROM job
		WHERE approved_at IS NOT NULL
		AND slug = $1`, slug)
	var createdAt time.Time
	var perks, interview, companyIcon sql.NullString
	err := row.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon)
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
	job.TimeAgo = humanize.Time(createdAt.UTC())
	return job, nil
}

func JobPostByIDForEdit(conn *sql.DB, jobID int) (*JobPostForEdit, error) {
	job := &JobPostForEdit{}
	row := conn.QueryRow(
		`SELECT job_title, company, company_email, company_url, salary_min, salary_max, salary_currency, location, description, perks, interview_process, how_to_apply, created_at, slug, approved_at, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id
		FROM job
		WHERE id = $1`, jobID)
	var perks, interview, companyURL, companyIconID sql.NullString
	err := row.Scan(&job.JobTitle, &job.Company, &job.CompanyEmail, &companyURL, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &job.CreatedAt, &job.Slug, &job.ApprovedAt, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIconID)
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
		`SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id
		FROM job
		WHERE approved_at IS NOT NULL
		AND url_id = $1`, URLID)
	var createdAt time.Time
	var perks, interview, companyIcon sql.NullString
	err := row.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon)
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
	job.TimeAgo = humanize.Time(createdAt.UTC())
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
		`DELETE FROM job WHERE id = $1`,
		jobID,
	); err != nil {
		return err
	}
	return nil
}

func GetPinnedJobs(conn *sql.DB) ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := conn.Query(`
	SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id
		FROM job WHERE approved_at IS NOT NULL AND ad_type IN (2, 3)`)
	if err != nil {
		return jobs, err
	}
	defer rows.Close()
	for rows.Next() {
		job := &JobPost{}
		var createdAt time.Time
		var perks, interview, companyIcon sql.NullString
		err = rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if perks.Valid {
			job.Perks = perks.String
		}
		if interview.Valid {
			job.InterviewProcess = interview.String
		}
		job.TimeAgo = humanize.Time(createdAt.UTC())
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
		err = rows.Scan(&fullRowsCount, &job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.JobDescription, &perks, &interview, &job.HowToApply, &createdAt, &job.CreatedAt, &job.Slug, &job.AdType, &job.SalaryMin, &job.SalaryMax, &job.SalaryCurrency, &companyIcon)
		if companyIcon.Valid {
			job.CompanyIconID = companyIcon.String
		}
		if perks.Valid {
			job.Perks = perks.String
		}
		if interview.Valid {
			job.InterviewProcess = interview.String
		}
		job.TimeAgo = humanize.Time(createdAt.UTC())
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
		SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id
		FROM job
		WHERE approved_at IS NOT NULL
		AND ad_type not in (2, 3)
		ORDER BY created_at DESC LIMIT $2 OFFSET $1`, offset, max)
	}
	if tag == "" && location != "" {
		return conn.Query(`
		SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id
		FROM job
		WHERE approved_at IS NOT NULL
		AND ad_type not in (2, 3)
		AND location ILIKE '%' || $1 || '%'
		ORDER BY created_at DESC LIMIT $3 OFFSET $2`, location, offset, max)
	}
	if tag != "" && location == "" {
		return conn.Query(`
	SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id
	FROM
	(
		SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, to_tsvector(job_title) || to_tsvector(company) || to_tsvector(description) AS doc
		FROM job WHERE approved_at IS NOT NULL AND ad_type not in (2, 3)
	) AS job_
	WHERE job_.doc @@ to_tsquery($1)
	ORDER BY ts_rank(job_.doc, to_tsquery($1)) DESC, created_at DESC LIMIT $3 OFFSET $2`, tag, offset, max)
	}

	return conn.Query(`
	SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id
	FROM
	(
		SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, to_tsvector(job_title) || to_tsvector(company) || to_tsvector(description) AS doc
		FROM job WHERE approved_at IS NOT NULL AND ad_type not in (2, 3)
	) AS job_
	WHERE job_.doc @@ to_tsquery($1)
	AND location ILIKE '%' || $2 || '%'
	ORDER BY ts_rank(job_.doc, to_tsquery($1)) DESC, created_at DESC LIMIT $4 OFFSET $3`, tag, location, offset, max)
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

func GetLastNJobsFromID(conn *sql.DB, max, jobID int) ([]*JobPost, error) {
	var jobs []*JobPost
	var rows *sql.Rows
	rows, err := conn.Query(`SELECT id, job_title, company, salary_range, location, slug, salary_currency, company_icon_image_id  FROM job WHERE id > $1 AND approved_at IS NOT NULL LIMIT $2`, jobID, max)
	if err != nil {
		return jobs, err
	}
	for rows.Next() {
		job := &JobPost{}
		var companyIcon sql.NullString
		err := rows.Scan(&job.ID, &job.JobTitle, &job.Company, &job.SalaryRange, &job.Location, &job.Slug, &job.SalaryCurrency, &companyIcon)
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
