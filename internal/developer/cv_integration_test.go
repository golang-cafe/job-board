package developer

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

func TestCVRepositoryPostgres(t *testing.T) {
	dsn := os.Getenv("CV_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CV_TEST_DATABASE_URL to an isolated PostgreSQL test database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	// Temporary tables ensure the test never modifies application tables.
	_, err = db.Exec(`CREATE TEMP TABLE developer_profile (id character(27) PRIMARY KEY, email text, updated_at timestamp, cv bytea);
CREATE TEMP TABLE developer_cv_download (id bigserial PRIMARY KEY, developer_profile_id character(27), cv bytea NOT NULL, user_id character(27) NOT NULL, downloaded_at timestamp NOT NULL);
INSERT INTO developer_profile(id,email) VALUES ('fixture-profile','fixture@example.invalid');`)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	if _, err = repo.DownloadCV(ctx, "fixture-profile", "recruiter"); err != sql.ErrNoRows {
		t.Fatalf("missing CV: %v", err)
	}
	first := []byte("%PDF-1.4\nfirst synthetic CV")
	if err = repo.SaveCV(ctx, "FIXTURE@example.invalid", first); err != nil {
		t.Fatal(err)
	}
	got, err := repo.DownloadCV(ctx, "fixture-profile", "recruiter")
	if err != nil || !bytes.Equal(got, first) {
		t.Fatalf("download %q %v", got, err)
	}
	replacement := []byte("%PDF-1.7\nreplacement synthetic CV")
	if err = repo.SaveCV(ctx, "fixture@example.invalid", replacement); err != nil {
		t.Fatal(err)
	}
	var audited []byte
	var reader string
	if err = db.QueryRow(`SELECT cv,user_id FROM developer_cv_download`).Scan(&audited, &reader); err != nil || !bytes.Equal(audited, first) || strings.TrimSpace(reader) != "recruiter" {
		t.Fatalf("audit changed after replacement: %q %q %v", audited, reader, err)
	}
	if err = repo.SaveCV(ctx, "missing@example.invalid", first); err != sql.ErrNoRows {
		t.Fatalf("missing profile: %v", err)
	}
	if _, err = db.Exec(`ALTER TABLE developer_cv_download ADD CONSTRAINT fail_audit CHECK (user_id <> 'blocked')`); err != nil {
		t.Fatal(err)
	}
	if got, err = repo.DownloadCV(ctx, "fixture-profile", "blocked"); err == nil || got != nil {
		t.Fatal("audit failure released CV")
	}
	if _, err = repo.DownloadCV(ctx, "fixture-profile", "admin"); err != nil {
		t.Fatalf("transaction not cleaned up: %v", err)
	}
}
