package linkedin

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"github.com/lib/pq"
	"net/http"
	"os"
	"strings"
	"testing"
)

func fixtureDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("LINKEDIN91_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set LINKEDIN91_TEST_DATABASE_URL for isolated SQL tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	var b [8]byte
	if _, err = rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	schema := pq.QuoteIdentifier("linkedin91_" + hex.EncodeToString(b[:]))
	if _, err = db.Exec("CREATE SCHEMA " + schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DROP SCHEMA " + schema + " CASCADE"); db.Close() })
	if _, err = db.Exec("SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE job(id integer PRIMARY KEY,job_title text,company text,location text,salary_range text,slug text,approved_at timestamp);
 INSERT INTO job VALUES(8,'Last','C','Remote','USD','last',now()),(3,'Unapproved','C','Remote','USD','unapproved',NULL),(4,'First','C','Remote','USD','first',now());`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/091_linkedin_jobs.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	return db
}

type postFunc func(context.Context, string) (string, error)

func (f postFunc) Post(ctx context.Context, text string) (string, error) { return f(ctx, text) }

func TestSQLReservationAndRealHTTPCompletion(t *testing.T) {
	db := fixtureDB(t)
	ctx := context.Background()
	calls := 0
	client := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("X-Restli-Id", "urn:li:ugcPost:1234")
		w.WriteHeader(201)
	})
	s := Scheduler{DB: db, Poster: client, SiteURL: "https://example.invalid"}
	d, err := s.Reserve(ctx)
	if err != nil || d == nil || d.ID != 4 || !strings.Contains(d.Text, "/job/first") {
		t.Fatal(d, err)
	}
	if _, err = s.Reserve(ctx); !errors.Is(err, ErrPending) {
		t.Fatal("overlapping reservation", err)
	}
	if err = s.Send(ctx, d); err != nil {
		t.Fatal(err)
	}
	var last, count int
	var pending sql.NullInt64
	if err = db.QueryRow(`SELECT last_job_id,pending_job_id FROM linkedin_scheduler_state`).Scan(&last, &pending); err != nil || last != 4 || pending.Valid {
		t.Fatal(last, pending, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM linkedin_post WHERE job_id=4 AND post_id='urn:li:ugcPost:1234'`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	d, err = s.Reserve(ctx)
	if err != nil || d.ID != 8 {
		t.Fatal(d, err)
	}
	if err = s.Send(ctx, d); err != nil {
		t.Fatal(err)
	}
	if d, err = s.Reserve(ctx); err != nil || d != nil || calls != 2 {
		t.Fatal(d, err, calls)
	}
}

func TestFailureAndDatabaseAckFailureKeepPending(t *testing.T) {
	for _, mode := range []string{"api", "ack", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			db := fixtureDB(t)
			ctx := context.Background()
			s := Scheduler{DB: db, SiteURL: "https://example.invalid", Poster: postFunc(func(context.Context, string) (string, error) {
				if mode == "api" {
					return "", errors.New("fixture")
				}
				return "urn:li:share:123", nil
			})}
			d, err := s.Reserve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "ack" {
				if _, err = db.Exec(`ALTER TABLE linkedin_post ADD CONSTRAINT fixture_failure CHECK(job_id<0)`); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err = s.Send(ctx, d); err == nil {
				t.Fatal("failure swallowed")
			}
			if _, err = s.Reserve(context.Background()); !errors.Is(err, ErrPending) {
				t.Fatal(err)
			}
			var last int
			if err = db.QueryRow(`SELECT last_job_id FROM linkedin_scheduler_state`).Scan(&last); err != nil || last != 0 {
				t.Fatal(last, err)
			}
		})
	}
}

func TestMissingStateAndInvalidSite(t *testing.T) {
	db := fixtureDB(t)
	s := Scheduler{DB: db, SiteURL: "https://example.invalid"}
	if _, err := db.Exec(`DELETE FROM linkedin_scheduler_state`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reserve(context.Background()); err == nil {
		t.Fatal("missing state accepted")
	}
	s.SiteURL = "https://user:secret@example.invalid"
	if _, err := s.Reserve(context.Background()); err == nil {
		t.Fatal("userinfo accepted")
	}
}
