package database

import (
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/segmentio/ksuid"
)

type SEOLandingPage struct {
	URI      string
	Location string
	Skill    string
}

type SEOLocation struct {
	Name       string
	Population int
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

// CREATE TABLE IF NOT EXISTS fx_rate (
//   base       CHAR(3) NOT NULL,
//   target     CHAR(3) NOT NULL,
//   value      FLOAT NOT NULL,
//   updated_at TIMESTAMP NOT NULL,
//   PRIMARY KEY(base, target)
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
// alter table seo_location add column long FLOAT DEFAULT NULL
// alter table seo_location add column lat FLOAT DEFAULT NULL
// alter table seo_location add column iso3 CHAR(3) DEFAULT NULL
// alter table seo_location add column iso2 CHAR(2) DEFAULT NULL
// alter table seo_location add column region VARCHAR(255) DEFAULT NULL
// alter table seo_location add column population INTEGER DEFAULT NULL
// ALTER TABLE seo_location ADD COLUMN emoji VARCHAR(5);

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

type FXRate struct {
	Base      string
	Target    string
	Value     float64
	UpdatedAt time.Time
}

func AddFXRate(conn *sql.DB, fx FXRate) error {
	_, err := conn.Exec(`INSERT INTO fx_rate (base, target, value, updated_at) VALUES ($1, $2, $3, $4) ON CONFLICT(base, target) DO UPDATE SET value = $3, updated_at = $4`, fx.Base, fx.Target, fx.Value, fx.UpdatedAt)
	return err
}

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

func CountEmailSubscribers(conn *sql.DB) (int, error) {
	row := conn.QueryRow(`SELECT count(*) FROM email_subscribers WHERE confirmed_at IS NOT NULL`)
	var count int
	err := row.Scan(&count)
	if count < 100 {
		count = 100
	}
	return count, err
}

// GetDbConn tries to establish a connection to postgres and return the connection handler
func GetDbConn(databaseUser string, databasePassword string, databaseHost string, databasePort string, databaseName string, sslMode string) (*sql.DB, error) {
	databaseURL := fmt.Sprintf("postgres://%v:%v@%v:%v/%v?sslmode=%s",
		databaseUser,
		databasePassword,
		databaseHost,
		databasePort,
		databaseName,
		sslMode,
	)
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

func DuplicateImage(conn *sql.DB, oldID, newID string) error {
	stmt := `INSERT INTO image (id, bytes, media_type) SELECT $1, bytes, media_type FROM image WHERE id = $2`
	_, err := conn.Exec(stmt, newID, oldID)
	return err
}

func DeleteImageByID(conn *sql.DB, id string) error {
	_, err := conn.Exec(`DELETE FROM image WHERE id = $1`, id)
	return err
}

func LocationsByPrefix(conn *sql.DB, prefix string) ([]Location, error) {
	locs := make([]Location, 0)
	if len(prefix) < 2 {
		return locs, nil
	}
	rows, err := conn.Query(`SELECT name, country, emoji FROM seo_location WHERE name ILIKE $1 || '%' ORDER BY population DESC LIMIT 5`, prefix)
	if err == sql.ErrNoRows {
		return locs, nil
	} else if err != nil {
		return locs, err
	}
	for rows.Next() {
		var loc Location
		var nullCountry sql.NullString
		if err := rows.Scan(&loc.Name, &nullCountry, &loc.Emoji); err != nil {
			return locs, err
		}
		if nullCountry.Valid {
			loc.Country = nullCountry.String
		}
		locs = append(locs, loc)
	}

	return locs, nil
}

func SkillsByPrefix(conn *sql.DB, prefix string) ([]SEOSkill, error) {
	skills := make([]SEOSkill, 0)
	if len(prefix) < 1 {
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
	rows, err := conn.Query(`SELECT name, population FROM seo_location`)
	if err != nil {
		return locations, err
	}
	defer rows.Close()
	for rows.Next() {
		loc := SEOLocation{}
		err = rows.Scan(&loc.Name, &loc.Population)
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

type Location struct {
	Name       string  `json:"name,omitempty"`
	Country    string  `json:"country,omitempty"`
	Region     string  `json:"region,omitempty"`
	Population int64   `json:"population,omitempty"`
	Lat        float64 `json:"lat,omitempty"`
	Long       float64 `json:"long,omitempty"`
	Currency   string  `json:"currency,omitempty"`
	Emoji      string  `json:"emoji,omitempty"`
}

func GetLocation(conn *sql.DB, location string) (Location, error) {
	var country, region, lat, long, population sql.NullString
	var loc Location
	res := conn.QueryRow(`SELECT name, currency, country, region, population, lat, long, emoji FROM seo_location WHERE LOWER(name) = LOWER($1)`, location)
	err := res.Scan(&loc.Name, &loc.Currency, &country, &region, &population, &lat, &long, &loc.Emoji)
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

func GetRandomLocationsForCountry(conn *sql.DB, country string, howMany int, excludeLoc string) ([]string, error) {
	locs := make([]string, 0)
	var rows *sql.Rows
	rows, err := conn.Query(`select name from seo_location where country = $1 and name != $2 order by population desc limit $3`, country, excludeLoc, howMany)
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

type PurchaseEvent struct {
	StripeSessionID string
	CreatedAt       time.Time
	CompletedAt     time.Time
	Amount          int
	Currency        string
	Description     string
	Email           string
	JobID           int
	PlanType        string
	PlanDuration    int
}

func GetPurchaseEvents(conn *sql.DB, jobID int) ([]PurchaseEvent, error) {
	var purchases []PurchaseEvent
	rows, err := conn.Query(`SELECT stripe_session_id, created_at, completed_at, amount/100 as amount, currency, description, plan_type, plan_duration, job_id FROM purchase_event WHERE job_id = $1 AND completed_at IS NOT NULL`, jobID)
	if err == sql.ErrNoRows {
		return purchases, nil
	}
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p PurchaseEvent
		if err := rows.Scan(&p.StripeSessionID, &p.CreatedAt, &p.CompletedAt, &p.Amount, &p.Currency, &p.Description, &p.PlanType, &p.PlanDuration, &p.JobID); err != nil {
			return purchases, err
		}
		purchases = append(purchases, p)
	}

	return purchases, nil
}

func InitiatePaymentEventForJobAd(conn *sql.DB, sessionID string, amount int64, description string, email string, jobID int, planType string, planDuration int64) error {
	stmt := `INSERT INTO purchase_event (stripe_session_id, amount, currency, description, ad_type, email, job_id, created_at, plan_type, plan_duration) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8, $9)`
	_, err := conn.Exec(stmt, sessionID, amount, "USD", description, 0, email, jobID, planType, planDuration)
	return err
}

func InitiatePaymentEventForDeveloperDirectoryAccess(conn *sql.DB, sessionID string, amount int64, description string, recruiterID string, email string, planDuration int64) error {
	stmt := `INSERT INTO developer_directory_purchase_event (stripe_session_id, amount, currency, description, created_at, expired_at, recruiter_id, email, duration) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := conn.Exec(stmt, sessionID, amount, "USD", description, time.Now().UTC(), time.Now().UTC().AddDate(0, int(planDuration), 0), recruiterID, email, planDuration)
	return err
}

func SaveSuccessfulPaymentForJobAd(conn *sql.DB, sessionID string) (int, error) {
	res := conn.QueryRow(`WITH rows AS (UPDATE purchase_event SET completed_at = NOW() WHERE stripe_session_id = $1 AND completed_at IS NULL RETURNING 1) SELECT count(*) as c FROM rows;`, sessionID)
	var affected int
	err := res.Scan(&affected)
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func SaveSuccessfulPaymentForDevDirectory(conn *sql.DB, sessionID string) (int, error) {
	res := conn.QueryRow(`WITH rows AS (UPDATE developer_directory_purchase_event SET completed_at = NOW() WHERE stripe_session_id = $1 AND completed_at IS NULL RETURNING 1) SELECT count(*) as c FROM rows;`, sessionID)
	var affected int
	err := res.Scan(&affected)
	if err != nil {
		return 0, err
	}
	return affected, nil
}

func IsJobAdPaymentEvent(conn *sql.DB, sessionID string) (bool, error) {
	res := conn.QueryRow(`SELECT count(*) = 1 as found FROM purchase_event WHERE stripe_session_id = $1`, sessionID)
	var found bool
	err := res.Scan(&found)
	if err != nil {
		return false, err
	}
	return found, nil
}

func IsDevDirectoryPaymentEvent(conn *sql.DB, sessionID string) (bool, error) {
	res := conn.QueryRow(`SELECT count(*) = 1 as found FROM developer_directory_purchase_event WHERE stripe_session_id = $1`, sessionID)
	var found bool
	err := res.Scan(&found)
	if err != nil {
		return false, err
	}
	return found, nil
}

type DevDirectoryPurchaseEvent struct {
	StripeSessionID string
	CreatedAt time.Time
	CompletedAt time.Time
	ExpiredAt time.Time
	Email string
	Amount int64
	Currency string
	Description string
	RecruiterID string
	Duration int64
}

func GetDevDirectoryPurchaseEventBySessionID(conn *sql.DB, sessionID string) (DevDirectoryPurchaseEvent, error) {
	res := conn.QueryRow(`SELECT stripe_session_id, created_at, completed_at, expired_at, email, amount, currency, description, recruiter_id, duration FROM developer_directory_purchase_event WHERE stripe_session_id = $1`, sessionID)
	var p DevDirectoryPurchaseEvent
	err := res.Scan(&p.StripeSessionID, &p.CreatedAt, &p.CompletedAt, &p.ExpiredAt, &p.Email, &p.Amount, &p.Currency, &p.Description, &p.RecruiterID, &p.Duration)
	if err != nil {
		return p, err
	}

	return p, nil
}

func GetJobAdPurchaseEventBySessionID(conn *sql.DB, sessionID string) (PurchaseEvent, error) {
	res := conn.QueryRow(`SELECT stripe_session_id, created_at, completed_at, email, amount, currency, description, plan_type, plan_duration FROM purchase_event WHERE stripe_session_id = $1`, sessionID)
	var p PurchaseEvent
	err := res.Scan(&p.StripeSessionID, &p.CreatedAt, &p.CompletedAt, &p.Email, &p.Amount, &p.Currency, &p.Description, &p.PlanType, &p.PlanDuration)
	if err != nil {
		return p, err
	}

	return p, nil
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

func GetSitemapIndex(conn *sql.DB, siteHost string) ([]SitemapEntry, error) {
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
	for i := 1; i <= int(slots); i++ {
		entries = append(entries, SitemapEntry{
			Loc:     fmt.Sprintf("https://%s/sitemap-%d.xml", siteHost, i),
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

func UpdateLastWeekClickouts(conn *sql.DB) error {
	_, err := conn.Exec(`WITH cte AS (SELECT job_id, count(*) AS clickouts FROM job_event WHERE event_type = 'clickout' AND created_at > CURRENT_DATE - 7 GROUP BY job_id)
UPDATE job SET last_week_clickouts = cte.clickouts FROM cte WHERE cte.job_id = id`)
	return err
}
