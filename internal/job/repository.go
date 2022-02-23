package job

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gosimple/slug"
	"github.com/segmentio/ksuid"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db}
}

func (r *Repository) TrackJobView(job *JobPost) error {
	stmt := `INSERT INTO job_event (event_type, job_id, created_at) VALUES ($1, $2, NOW())`
	_, err := r.db.Exec(stmt, jobEventPageView, job.ID)
	return err
}

func (r *Repository) GetJobByApplyToken(token string) (JobPost, Applicant, error) {
	res := r.db.QueryRow(`SELECT t.cv, t.email, j.id, j.job_title, j.company, company_url, salary_range, location, how_to_apply, slug, j.external_id
	FROM job j JOIN apply_token t ON t.job_id = j.id AND t.token = $1 WHERE j.approved_at IS NOT NULL AND t.created_at < NOW() + INTERVAL '3 days' AND t.confirmed_at IS NULL`, token)
	job := JobPost{}
	applicant := Applicant{}
	err := res.Scan(&applicant.Cv, &applicant.Email, &job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.HowToApply, &job.Slug, &job.ExternalID)
	if err != nil {
		return JobPost{}, applicant, err
	}

	return job, applicant, nil
}

func (r *Repository) TrackJobClickout(jobID int) error {
	stmt := `INSERT INTO job_event (event_type, job_id, created_at) VALUES ($1, $2, NOW())`
	_, err := r.db.Exec(stmt, jobEventClickout, jobID)
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetJobByExternalID(externalID string) (JobPost, error) {
	res := r.db.QueryRow(`SELECT id, job_title, company, company_url, salary_range, location, how_to_apply, slug, external_id, salary_period FROM job WHERE external_id = $1`, externalID)
	var job JobPost
	err := res.Scan(&job.ID, &job.JobTitle, &job.Company, &job.CompanyURL, &job.SalaryRange, &job.Location, &job.HowToApply, &job.Slug, &job.ExternalID, &job.SalaryPeriod)
	if err != nil {
		return job, err
	}

	return job, nil
}

func (r *Repository) GetJobsOlderThan(since time.Time, adType JobAdType) ([]JobPost, error) {
	var jobs []JobPost
	rows, err := r.db.Query(`SELECT id, job_title, company, company_url, company_email, salary_range, location, how_to_apply, slug, external_id, approved_at FROM job j WHERE approved_at <= $1 AND ad_type = $2`, since, adType)
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

func (r *Repository) DemoteJobAdsOlderThan(since time.Time, jobAdType JobAdType) (int, error) {
	res := r.db.QueryRow(`WITH rows AS (UPDATE job SET ad_type = $1 WHERE ad_type = $2 AND approved_at <= $3 RETURNING 1) SELECT count(*) as c FROM rows;`, JobAdBasic, jobAdType, since)
	var affected int
	err := res.Scan(&affected)
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func (r *Repository) SaveDraft(job *JobRq) (int, error) {
	externalID, err := ksuid.NewRandom()
	if err != nil {
		return 0, err
	}
	sqlStatement := `
			INSERT INTO job (job_title, company, company_url, salary_range, salary_min, salary_max, salary_currency, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, company_email, ad_type, external_id, salary_period, salary_currency_iso, visa_sponsorship)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, 'year', $19, $20) RETURNING id`
	if job.CompanyIconID != "" {
		sqlStatement = `
			INSERT INTO job (job_title, company, company_url, salary_range, salary_min, salary_max, salary_currency, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, company_email, ad_type, company_icon_image_id, external_id, salary_period, salary_currency_iso, visa_sponsorship)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, 'year', $20, $21) RETURNING id`
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
	salaryRange := salaryToSalaryRangeString(salaryMinInt, salaryMaxInt, job.SalaryCurrency)
	var lastInsertID int
	var res *sql.Row
	if job.CompanyIconID != "" {
		res = r.db.QueryRow(sqlStatement, job.JobTitle, job.Company, job.CompanyURL, salaryRange, job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.Location, job.Description, job.Perks, job.InterviewProcess, job.HowToApply, time.Unix(createdAt, 0), createdAt, slugTitle, job.Email, job.AdType, job.CompanyIconID, externalID, job.SalaryCurrencyISO, job.VisaSponsorship)
	} else {
		res = r.db.QueryRow(sqlStatement, job.JobTitle, job.Company, job.CompanyURL, salaryRange, job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.Location, job.Description, job.Perks, job.InterviewProcess, job.HowToApply, time.Unix(createdAt, 0), createdAt, slugTitle, job.Email, job.AdType, externalID, job.SalaryCurrencyISO, job.VisaSponsorship)
	}
	res.Scan(&lastInsertID)
	if err != nil {
		return 0, err
	}
	return int(lastInsertID), err
}

func (r *Repository) UpdateJob(job *JobRqUpdate, jobID int) error {
	salaryMinInt, err := strconv.Atoi(strings.TrimSpace(job.SalaryMin))
	if err != nil {
		return err
	}
	salaryMaxInt, err := strconv.Atoi(strings.TrimSpace(job.SalaryMax))
	if err != nil {
		return err
	}
	salaryRange := salaryToSalaryRangeString(salaryMinInt, salaryMaxInt, job.SalaryCurrency)
	_, err = r.db.Exec(
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

func (r *Repository) ApproveJob(jobID int) error {
	_, err := r.db.Exec(
		`UPDATE job SET approved_at = NOW() WHERE id = $1`,
		jobID,
	)
	if err != nil {
		return err
	}
	return err
}

func (r *Repository) DisapproveJob(jobID int) error {
	_, err := r.db.Exec(
		`UPDATE job SET approved_at = NULL WHERE id = $1`,
		jobID,
	)
	if err != nil {
		return err
	}
	return err
}

func (r *Repository) GetViewCountForJob(jobID int) (int, error) {
	var count int
	row := r.db.QueryRow(`select count(*) as c from job_event where job_event.event_type = 'page_view' and job_event.job_id = $1`, jobID)
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, err
}

func (r *Repository) GetJobByStripeSessionID(sessionID string) (JobPost, error) {
	res := r.db.QueryRow(`SELECT j.id, j.job_title, j.company, j.company_url, j.salary_range, j.location, j.how_to_apply, j.slug, j.external_id, j.approved_at FROM purchase_event p LEFT JOIN job j ON p.job_id = j.id WHERE p.stripe_session_id = $1`, sessionID)
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

func (r *Repository) GetStatsForJob(jobID int) ([]JobStat, error) {
	var stats []JobStat
	rows, err := r.db.Query(`SELECT COUNT(*) FILTER (WHERE event_type = 'clickout') AS clickout, COUNT(*) FILTER (WHERE event_type = 'page_view') AS pageview, TO_CHAR(DATE_TRUNC('day', created_at), 'YYYY-MM-DD') FROM job_event WHERE job_id = $1 GROUP BY DATE_TRUNC('day', created_at) ORDER BY DATE_TRUNC('day', created_at) ASC`, jobID)
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

func (r *Repository) JobPostByCreatedAt() ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := r.db.Query(
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

func (r *Repository) TopNJobsByCurrencyAndLocation(currency, location string, max int) ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := r.db.Query(
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

func (r *Repository) JobPostBySlug(slug string) (*JobPost, error) {
	job := &JobPost{}
	row := r.db.QueryRow(
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

func (r *Repository) JobPostBySlugAdmin(slug string) (*JobPost, error) {
	job := &JobPost{}
	row := r.db.QueryRow(
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

func (r *Repository) JobPostByIDForEdit(jobID int) (*JobPostForEdit, error) {
	job := &JobPostForEdit{}
	row := r.db.QueryRow(
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

func (r *Repository) JobPostByExternalIDForEdit(externalID string) (*JobPostForEdit, error) {
	job := &JobPostForEdit{}
	row := r.db.QueryRow(
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

func (r *Repository) JobPostByURLID(URLID int64) (*JobPost, error) {
	job := &JobPost{}
	row := r.db.QueryRow(
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

func (r *Repository) DeleteJobCascade(jobID int) error {
	if _, err := r.db.Exec(
		`DELETE FROM image WHERE id IN (SELECT company_icon_image_id FROM job WHERE id = $1)`,
		jobID,
	); err != nil {
		return err
	}
	if _, err := r.db.Exec(
		`DELETE FROM edit_token WHERE job_id = $1`,
		jobID,
	); err != nil {
		return err
	}
	if _, err := r.db.Exec(
		`DELETE FROM apply_token WHERE job_id = $1`,
		jobID,
	); err != nil {
		return err
	}
	if _, err := r.db.Exec(
		`DELETE FROM job_event WHERE job_id = $1`,
		jobID,
	); err != nil {
		return err
	}
	if _, err := r.db.Exec(
		`DELETE FROM purchase_event WHERE job_id = $1`,
		jobID,
	); err != nil {
		return err
	}
	if _, err := r.db.Exec(
		`DELETE FROM job WHERE id = $1`,
		jobID,
	); err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetPendingJobs() ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := r.db.Query(`
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
func (r *Repository) GetCompanyJobs(companyName string, limit int) ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := r.db.Query(`
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
func (r *Repository) GetRelevantJobs(location string, jobID int, limit int) ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := r.db.Query(`
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

func (r *Repository) GetPinnedJobs() ([]*JobPost, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	rows, err := r.db.Query(`
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

func (r *Repository) JobsByQuery(location, tag string, pageId, salary int, currency string, jobsPerPage int, includePinnedJobs bool) ([]*JobPost, int, error) {
	jobs := []*JobPost{}
	var rows *sql.Rows
	offset := pageId*jobsPerPage - jobsPerPage
	// replace `|` with white space
	// remove double white spaces
	// join with `|` for ps query
	tag = strings.Join(strings.Fields(strings.ReplaceAll(tag, "|", " ")), "|")
	rows, err := getQueryForArgs(r.db, location, tag, salary, currency, offset, jobsPerPage, includePinnedJobs)
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

func (r *Repository) TokenByJobID(jobID int) (string, error) {
	tokenRow := r.db.QueryRow(
		`SELECT token
		FROM edit_token
		WHERE job_id = $1`, jobID)
	var token string
	err := tokenRow.Scan(&token)
	return token, err
}

func (r *Repository) JobPostIDByToken(token string) (int, error) {
	row := r.db.QueryRow(
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

func (r *Repository) GetLastNJobs(max int, loc string) ([]*JobPost, error) {
	var jobs []*JobPost
	var rows *sql.Rows
	var err error
	if strings.TrimSpace(loc) == "" {
		rows, err = r.db.Query(`SELECT id, job_title, description, company, salary_range, location, slug, salary_currency, company_icon_image_id, external_id, approved_at, salary_period FROM job WHERE approved_at IS NOT NULL ORDER BY approved_at DESC LIMIT $1`, max)
	} else {
		rows, err = r.db.Query(`SELECT
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

func (r *Repository) GetLastNJobsFromID(max, jobID int) ([]*JobPost, error) {
	var jobs []*JobPost
	var rows *sql.Rows
	rows, err := r.db.Query(`SELECT id, job_title, company, salary_range, location, slug, salary_currency, company_icon_image_id, external_id, salary_period FROM job WHERE id > $1 AND approved_at IS NOT NULL LIMIT $2`, jobID, max)
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

func (r *Repository) MarkJobAsExpired(jobID int) error {
	_, err := r.db.Exec(`UPDATE job SET expired = true WHERE id = $1`, jobID)
	return err
}

func (r *Repository) NewJobsLastWeekOrMonth() (int, int, error) {
	var week, month int
	row := r.db.QueryRow(`select lastweek.c as week, lastmonth.c as month 
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

func (r *Repository) GetJobApplyURLs() ([]JobApplyURL, error) {
	jobURLs := make([]JobApplyURL, 0)
	var rows *sql.Rows
	rows, err := r.db.Query(`SELECT id, how_to_apply FROM job WHERE approved_at IS NOT NULL AND expired = false`)
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

func (r *Repository) UpdateJobAdType(adType int, jobID int) error {
	_, err := r.db.Exec(`UPDATE job SET ad_type = $1, approved_at = NOW() WHERE id = $2`, adType, jobID)
	return err
}

func (r *Repository) GetClickoutCountForJob(jobID int) (int, error) {
	var count int
	row := r.db.QueryRow(`select count(*) as c from job_event where job_event.event_type = 'clickout' and job_event.job_id = $1`, jobID)
	err := row.Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, err
}

func (r *Repository) LastJobPosted() (time.Time, error) {
	row := r.db.QueryRow(`SELECT created_at FROM job WHERE approved_at IS NOT NULL ORDER BY created_at DESC LIMIT 1`)
	var last time.Time
	if err := row.Scan(&last); err != nil {
		return last, err
	}

	return last, nil
}

func (r *Repository) SaveTokenForJob(token string, jobID int) error {
	_, err := r.db.Exec(`INSERT INTO edit_token (token, job_id, created_at) VALUES ($1, $2, $3)`, token, jobID, time.Now().UTC())
	if err != nil {
		return err
	}
	return err
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

func (r *Repository) ApplyToJob(jobID int, cv []byte, email, token string) error {
	stmt := `INSERT INTO apply_token (token, job_id, created_at, email, cv) VALUES ($1, $2, NOW(), $3, $4)`
	_, err := r.db.Exec(stmt, token, jobID, email, cv)
	return err
}

func (r *Repository) ConfirmApplyToJob(token string) error {
	_, err := r.db.Exec(
		`UPDATE apply_token SET confirmed_at = NOW() WHERE token = $1`,
		token,
	)
	return err
}

func (r *Repository) CleanupExpiredApplyTokens() error {
	_, err := r.db.Exec(
		`DELETE FROM apply_token WHERE created_at < NOW() - INTERVAL '3 days' OR confirmed_at IS NOT NULL`,
	)
	return err
}

func salaryToSalaryRangeString(salaryMin, salaryMax int, currency string) string {
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

func getQueryForArgs(conn *sql.DB, location, tag string, salary int, currency string, offset, max int, includePinnedJobs bool) (*sql.Rows, error) {
	adTypeFilter := "AND ad_type NOT IN (2, 3, 5)"
	if includePinnedJobs {
		adTypeFilter = "AND 1=1"
	}
	if tag == "" && location == "" && salary == 0 {
		return conn.Query(`
		SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
		FROM job
		WHERE approved_at IS NOT NULL `+adTypeFilter+` ORDER BY created_at DESC LIMIT $2 OFFSET $1`, offset, max)
	}
	if tag == "" && location != "" && salary == 0 {
		return conn.Query(`
		SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts 
		FROM job
		WHERE approved_at IS NOT NULL `+adTypeFilter+` AND location ILIKE '%' || $1 || '%'
		ORDER BY created_at DESC LIMIT $3 OFFSET $2`, location, offset, max)
	}
	if tag != "" && location == "" && salary == 0 {
		return conn.Query(`
	SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
	FROM
	(
		SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts, to_tsvector(job_title) || to_tsvector(company) || to_tsvector(description) AS doc
		FROM job WHERE approved_at IS NOT NULL `+adTypeFilter+`
	) AS job_
	WHERE job_.doc @@ to_tsquery($1)
	ORDER BY ts_rank(job_.doc, to_tsquery($1)) DESC, created_at DESC LIMIT $3 OFFSET $2`, tag, offset, max)
	}
	if tag != "" && location != "" && salary == 0 {
		return conn.Query(`
	SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
	FROM
	(
		SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts, to_tsvector(job_title) || to_tsvector(company) || to_tsvector(description) AS doc
		FROM job WHERE approved_at IS NOT NULL `+adTypeFilter+`
	) AS job_
	WHERE job_.doc @@ to_tsquery($1)
	AND location ILIKE '%' || $2 || '%'
	ORDER BY ts_rank(job_.doc, to_tsquery($1)) DESC, created_at DESC LIMIT $4 OFFSET $3`, tag, location, offset, max)
	}
	if tag == "" && location == "" && salary != 0 {
		return conn.Query(`
		SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
		FROM job FULL JOIN fx_rate ON fx_rate.base = job.salary_currency_iso AND fx_rate.target = $4
		WHERE approved_at IS NOT NULL `+adTypeFilter+` AND (COALESCE(fx_rate.value, 1)*job.salary_max) >= $3 ORDER BY created_at DESC LIMIT $2 OFFSET $1`, offset, max, salary, currency)
	}
	if tag == "" && location != "" && salary != 0 {
		return conn.Query(`
		SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts 
		FROM job FULL JOIN fx_rate ON fx_rate.base = job.salary_currency_iso AND fx_rate.target = $5
		WHERE approved_at IS NOT NULL `+adTypeFilter+` AND location ILIKE '%' || $1 || '%' AND (COALESCE(fx_rate.value, 1)*job.salary_max) >= $4
		ORDER BY created_at DESC LIMIT $3 OFFSET $2`, location, offset, max, salary, currency)
	}
	if tag != "" && location == "" && salary != 0 {
		return conn.Query(`
	SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
	FROM
	(
		SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts, to_tsvector(job_title) || to_tsvector(company) || to_tsvector(description) AS doc
		FROM job FULL JOIN fx_rate ON fx_rate.base = job.salary_currency_iso AND fx_rate.target = $5 WHERE approved_at IS NOT NULL `+adTypeFilter+` AND (COALESCE(fx_rate.value, 1)*job.salary_max) >= $4
	) AS job_
	WHERE job_.doc @@ to_tsquery($1)
	ORDER BY ts_rank(job_.doc, to_tsquery($1)) DESC, created_at DESC LIMIT $3 OFFSET $2`, tag, offset, max, salary, currency)
	}

	return conn.Query(`
	SELECT count(*) OVER() AS full_count, id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts
	FROM
	(
		SELECT id, job_title, company, company_url, salary_range, location, description, perks, interview_process, how_to_apply, created_at, url_id, slug, ad_type, salary_min, salary_max, salary_currency, company_icon_image_id, external_id, salary_period, expired, last_week_clickouts, to_tsvector(job_title) || to_tsvector(company) || to_tsvector(description) AS doc
		FROM job FULL JOIN fx_rate ON fx_rate.base = job.salary_currency_iso AND fx_rate.target = $6 WHERE approved_at IS NOT NULL `+adTypeFilter+` AND (COALESCE(fx_rate.value, 1)*job.salary_max) >= $5
	) AS job_
	WHERE job_.doc @@ to_tsquery($1)
	AND location ILIKE '%' || $2 || '%'
	ORDER BY ts_rank(job_.doc, to_tsquery($1)) DESC, created_at DESC LIMIT $4 OFFSET $3`, tag, location, offset, max, salary, currency)
}
