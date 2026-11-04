package linkedin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var ErrPending = errors.New("LinkedIn delivery is pending operator reconciliation")

type Delivery struct {
	ID   int
	Text string
}
type Poster interface {
	Post(context.Context, string) (string, error)
}
type Scheduler struct {
	DB      *sql.DB
	Poster  Poster
	SiteURL string
}

// Reserve commits the pending job before any external I/O. No automatic retry
// can distinguish a lost acknowledgement from an unpublished post.
func (s Scheduler) Reserve(ctx context.Context) (*Delivery, error) {
	site, err := url.Parse(s.SiteURL)
	if err != nil || site == nil || (site.Scheme != "http" && site.Scheme != "https") || site.Host == "" || site.User != nil || site.RawQuery != "" || site.Fragment != "" {
		return nil, errors.New("invalid public site URL")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var last int
	var pending sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT last_job_id,pending_job_id FROM linkedin_scheduler_state WHERE id=1 FOR UPDATE`).Scan(&last, &pending); err != nil {
		return nil, err
	}
	if pending.Valid {
		return nil, ErrPending
	}
	var id int
	var title, company, location, salary, slug string
	err = tx.QueryRowContext(ctx, `SELECT id,job_title,company,location,salary_range,slug FROM job WHERE id>$1 AND approved_at IS NOT NULL ORDER BY id ASC LIMIT 1`, last).Scan(&id, &title, &company, &location, &salary, &slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	teaser := []rune(strings.Join(strings.Fields(fmt.Sprintf("%s with %s - %s | %s", title, company, location, salary)), " "))
	if len(teaser) > 500 {
		teaser = append(teaser[:497], '.', '.', '.')
	}
	text := string(teaser) + "\n\n" + strings.TrimRight(s.SiteURL, "/") + "/job/" + url.PathEscape(slug)
	if len([]rune(text)) > 3000 {
		return nil, errors.New("job link exceeds LinkedIn commentary limit")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE linkedin_scheduler_state SET pending_job_id=$1 WHERE id=1`, id); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &Delivery{ID: id, Text: text}, nil
}

func (s Scheduler) Send(ctx context.Context, d *Delivery) error {
	postID, err := s.Poster.Post(ctx, d.Text)
	if err != nil {
		return err
	}
	if postID == "" {
		return errors.New("missing LinkedIn acknowledgement")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var pending sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT pending_job_id FROM linkedin_scheduler_state WHERE id=1 FOR UPDATE`).Scan(&pending); err != nil {
		return err
	}
	if !pending.Valid || pending.Int64 != int64(d.ID) {
		return ErrPending
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO linkedin_post(job_id,post_id) VALUES($1,$2)`, d.ID, postID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE linkedin_scheduler_state SET last_job_id=$1,pending_job_id=NULL WHERE id=1`, d.ID); err != nil {
		return err
	}
	return tx.Commit()
}
