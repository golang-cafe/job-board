package twitter

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
)

type postFunc func(context.Context, string) (string, error)

func (f postFunc) Post(ctx context.Context, text string) (string, error) { return f(ctx, text) }
func monday() time.Time                                                  { return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) }

func schedulerDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("X97_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set X97_TEST_DATABASE_URL for isolated PostgreSQL integration")
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
	schema := pq.QuoteIdentifier("x97_fixture_" + hex.EncodeToString(b[:]))
	if _, err = db.Exec("CREATE SCHEMA " + schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Exec("DROP SCHEMA " + schema + " CASCADE"); db.Close() })
	if _, err = db.Exec("SET search_path TO " + schema); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE meta(key text PRIMARY KEY,value text); INSERT INTO meta VALUES('last_twitted_job_id','2');
	CREATE TABLE job(id integer PRIMARY KEY,job_title text,company text,location text,salary_range text,slug text,approved_at timestamp);
	INSERT INTO job SELECT id,'Job '||id,'Company','Remote','USD 100','job-'||id,CASE WHEN id=3 THEN NULL ELSE '2026-11-01'::timestamp END FROM (VALUES(8),(3),(6),(4),(7),(5)) q(id);`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../migrations/097_twitter_jobs.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestWeekendsAreNoOp(t *testing.T) {
	for _, day := range []int{7, 8} {
		s := Scheduler{Now: func() time.Time { return time.Date(2026, 11, day, 12, 0, 0, 0, time.UTC) }}
		if n, err := s.Run(context.Background()); n != 0 || err != nil {
			t.Fatal(n, err)
		}
	}
}

func TestSQLDailyLimitOrderingAndUTC(t *testing.T) {
	db := schedulerDB(t)
	var messages []string
	s := Scheduler{DB: db, SiteURL: "https://example.invalid", Now: monday, Poster: postFunc(func(_ context.Context, text string) (string, error) {
		messages = append(messages, text)
		return fmt.Sprint(len(messages)), nil
	})}
	for i := 0; i < 3; i++ {
		n, err := s.Run(context.Background())
		want := 0
		if i == 0 {
			want = 2
		}
		if err != nil || n != want {
			t.Fatal(n, want, err)
		}
	}
	if len(messages) != 2 || !strings.Contains(messages[0], "/job/job-4") || !strings.Contains(messages[1], "/job/job-5") {
		t.Fatal(messages)
	}
	var cursor string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='last_twitted_job_id'`).Scan(&cursor); err != nil || cursor != "5" {
		t.Fatal(cursor, err)
	}
	s.Now = func() time.Time { return monday().Add(24 * time.Hour) }
	if n, err := s.Run(context.Background()); n != 2 || err != nil {
		t.Fatal(n, err)
	}
	if !strings.Contains(messages[2], "job-6") || !strings.Contains(messages[3], "job-7") {
		t.Fatal(messages)
	}
	// Local Saturday is still Friday UTC, so it is eligible for the next day.
	s.Now = func() time.Time { return time.Date(2026, 11, 7, 5, 0, 0, 0, time.FixedZone("fixture+14", 14*3600)) }
	if n, err := s.Run(context.Background()); n != 1 || err != nil {
		t.Fatal(n, err)
	}
	if n, err := s.Run(context.Background()); n != 0 || err != nil {
		t.Fatal(n, err)
	}
}

func TestSQLFailureDoesNotSkipOrRetry(t *testing.T) {
	db := schedulerDB(t)
	calls := 0
	s := Scheduler{DB: db, SiteURL: "https://example.invalid", Now: monday, Poster: postFunc(func(context.Context, string) (string, error) {
		calls++
		if calls == 2 {
			return "", errors.New("fixture delivery uncertain")
		}
		return "first", nil
	})}
	if n, err := s.Run(context.Background()); n != 1 || err == nil {
		t.Fatal(n, err)
	}
	s.Now = func() time.Time { return monday().Add(24 * time.Hour) }
	if n, err := s.Run(context.Background()); n != 0 || !errors.Is(err, ErrPending) || calls != 2 {
		t.Fatal(n, err, calls)
	}
	var last, pending int
	if err := db.QueryRow(`SELECT last_job_id,pending_job_id FROM twitter_scheduler_state`).Scan(&last, &pending); err != nil || last != 4 || pending != 5 {
		t.Fatal(last, pending, err)
	}
}

func TestSQLAcknowledgementFailureKeepsReservation(t *testing.T) {
	db := schedulerDB(t)
	if _, err := db.Exec(`ALTER TABLE twitter_post ADD CHECK(tweet_id <> 'fixture-rejected')`); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := Scheduler{DB: db, SiteURL: "https://example.invalid", Now: monday, Poster: postFunc(func(context.Context, string) (string, error) { calls++; return "fixture-rejected", nil })}
	if n, err := s.Run(context.Background()); n != 0 || err == nil {
		t.Fatal(n, err)
	}
	if n, err := s.Run(context.Background()); n != 0 || !errors.Is(err, ErrPending) || calls != 1 {
		t.Fatal(n, err, calls)
	}
}

func TestSQLConcurrentTriggerSeesPending(t *testing.T) {
	db := schedulerDB(t)
	started := make(chan struct{})
	release := make(chan struct{})
	s := Scheduler{DB: db, SiteURL: "https://example.invalid", Now: monday, Poster: postFunc(func(context.Context, string) (string, error) {
		close(started)
		<-release
		return "", errors.New("fixture timeout")
	})}
	done := make(chan error, 1)
	go func() { _, err := s.Run(context.Background()); done <- err }()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("first invocation stopped before posting: %v", err)
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("first invocation did not reach post")
	}
	_, err := s.Run(context.Background())
	close(release)
	if !errors.Is(err, ErrPending) {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("fixture failure was hidden")
	}
}

func TestSQLBoundedUnicodeTeaser(t *testing.T) {
	db := schedulerDB(t)
	if _, err := db.Exec(`UPDATE job SET job_title=$1 WHERE id=4`, strings.Repeat("\u5f00\u53d1", 150)); err != nil {
		t.Fatal(err)
	}
	s := Scheduler{DB: db, SiteURL: "https://example.invalid", Now: monday, Poster: postFunc(func(_ context.Context, text string) (string, error) {
		if !utf8.ValidString(text) || len([]rune(strings.Split(text, "\n")[0])) > 100 || !strings.Contains(text, "https://example.invalid/job/") {
			t.Error("bad teaser", text)
		}
		return "fixture", nil
	})}
	if n, err := s.Run(context.Background()); n != 2 || err != nil {
		t.Fatal(n, err)
	}
}

func TestSQLCancelledContextAndMissingState(t *testing.T) {
	db := schedulerDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := Scheduler{DB: db, SiteURL: "https://example.invalid", Now: monday, Poster: postFunc(func(context.Context, string) (string, error) { t.Error("unexpected post"); return "", nil })}
	if _, err := s.Run(ctx); err == nil {
		t.Fatal("cancelled context accepted")
	}
	if _, err := db.Exec(`DELETE FROM twitter_scheduler_state`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Run(context.Background()); err == nil {
		t.Fatal("missing scheduler state silently accepted")
	}
}

func TestSQLWithSignedHTTPClient(t *testing.T) {
	db := schedulerDB(t)
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/2/tweets" || !strings.HasPrefix(r.Header.Get("Authorization"), "OAuth ") {
			t.Error("incorrect HTTP request")
		}
		n := atomic.AddInt32(&calls, 1)
		w.WriteHeader(201)
		fmt.Fprintf(w, `{"data":{"id":"fixture-%d"}}`, n)
	}))
	defer ts.Close()
	c := fixtureClient(t)
	c.endpoint = ts.URL + "/2/tweets"
	s := Scheduler{DB: db, Poster: c, SiteURL: "https://example.invalid", Now: monday}
	if n, err := s.Run(context.Background()); n != 2 || err != nil {
		t.Fatal(n, err)
	}
	if n, err := s.Run(context.Background()); n != 0 || err != nil {
		t.Fatal(n, err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM twitter_post`).Scan(&count); err != nil || count != 2 || atomic.LoadInt32(&calls) != 2 {
		t.Fatal(count, calls, err)
	}
}
