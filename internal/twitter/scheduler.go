package twitter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var ErrPending = errors.New("previous X delivery needs operator reconciliation")

type Job struct {
	ID                                     int
	Title, Company, Location, Salary, Slug string
}
type Poster interface {
	Post(context.Context, string) (string, error)
}
type Scheduler struct {
	DB      *sql.DB
	Poster  Poster
	SiteURL string
	Now     func() time.Time
}

// Run reserves each slot durably before external I/O. An uncertain outcome is
// never retried automatically: X and PostgreSQL do not share a transaction.
func (s Scheduler) Run(ctx context.Context) (int, error) {
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	now = now.UTC()
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return 0, nil
	}
	site, err := url.Parse(s.SiteURL)
	if err != nil || site == nil || (site.Scheme != "http" && site.Scheme != "https") || site.Host == "" || site.User != nil {
		return 0, errors.New("invalid public job-site URL")
	}
	posted := 0
	for posted < 2 {
		j, err := s.reserve(ctx, now)
		if errors.Is(err, sql.ErrNoRows) {
			return posted, nil
		}
		if err != nil {
			return posted, err
		}
		link := strings.TrimRight(s.SiteURL, "/") + "/job/" + url.PathEscape(j.Slug)
		// Keep a short Unicode-safe teaser and the full canonical job link.
		teaser := []rune(strings.Join(strings.Fields(fmt.Sprintf("%s with %s - %s | %s", j.Title, j.Company, j.Location, j.Salary)), " "))
		if len(teaser) > 100 {
			teaser = append(teaser[:97], '.', '.', '.')
		}
		id, err := s.Poster.Post(ctx, string(teaser)+"\n\n"+link)
		if err != nil {
			return posted, fmt.Errorf("job %d delivery is pending reconciliation: %w", j.ID, err)
		}
		if err = s.complete(ctx, j.ID, id); err != nil {
			return posted, fmt.Errorf("job %d acknowledgement could not be stored; reconcile delivery before retrying", j.ID)
		}
		posted++
	}
	return posted, nil
}

func (s Scheduler) reserve(ctx context.Context, now time.Time) (Job, error) {
	var j Job
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return j, err
	}
	defer tx.Rollback()
	var last, count int
	var day sql.NullTime
	var pending sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT last_job_id,posting_day,posts_today,pending_job_id FROM twitter_scheduler_state WHERE id=1 FOR UPDATE`).Scan(&last, &day, &count, &pending)
	if errors.Is(err, sql.ErrNoRows) {
		return j, errors.New("scheduler state is missing; apply migration 097")
	}
	if err != nil {
		return j, err
	}
	if pending.Valid {
		return j, ErrPending
	}
	if !day.Valid || day.Time.UTC().Format("2006-01-02") != now.Format("2006-01-02") {
		count = 0
	}
	if count >= 2 {
		return j, sql.ErrNoRows
	}
	err = tx.QueryRowContext(ctx, `SELECT id,job_title,company,location,salary_range,slug FROM job WHERE id>$1 AND approved_at IS NOT NULL ORDER BY id ASC LIMIT 1`, last).Scan(&j.ID, &j.Title, &j.Company, &j.Location, &j.Salary, &j.Slug)
	if err != nil {
		return j, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE twitter_scheduler_state SET posting_day=$1,posts_today=$2,pending_job_id=$3 WHERE id=1`, now.Format("2006-01-02"), count+1, j.ID)
	if err != nil {
		return j, err
	}
	return j, tx.Commit()
}

func (s Scheduler) complete(ctx context.Context, jobID int, tweetID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var pending sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT pending_job_id FROM twitter_scheduler_state WHERE id=1 FOR UPDATE`).Scan(&pending); err != nil {
		return err
	}
	if !pending.Valid || pending.Int64 != int64(jobID) {
		return ErrPending
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO twitter_post (job_id,tweet_id,posting_day) SELECT $1,$2,posting_day FROM twitter_scheduler_state WHERE id=1`, jobID, tweetID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE twitter_scheduler_state SET last_job_id=$1,pending_job_id=NULL WHERE id=1`, jobID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE meta SET value=$1 WHERE key='last_twitted_job_id'`, fmt.Sprint(jobID)); err != nil {
		return err
	}
	return tx.Commit()
}
