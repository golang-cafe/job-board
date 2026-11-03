# Developer CV uploads

Implements #74 using the existing developer session and PostgreSQL repository.
No production accounts or real CVs are needed for the tests.

## Deployment

For an existing database, apply `migrations/074_developer_cv.sql` before deploying
the application. Fresh development databases get the same schema from
`latest-schema.sql`; do not also apply the migration to a fresh database.

Developers upload or replace their CV from the profile editor after signing in.
Only PDF-signature files up to 5 MiB are accepted. This is format/size validation,
not antivirus scanning or a guarantee that a document is safe. Downloads are
attachments with `nosniff` and `private, no-store` headers. Deployments should
retain their existing TLS/session protections.

Uploads resolve ownership from the signed-in email, not a submitted profile ID
or email. A same-origin custom upload header prevents cross-origin form posts.
Only authenticated recruiter/admin sessions may download a developer CV.
Missing CVs return 404. Download reads and audit insertion commit together; an
audit failure returns no CV bytes. LinkedIn is optional at registration; direct
LinkedIn/GitHub links on the profile are replaced by the CV action.

The requested audit stores the downloaded CV version, reader user ID and time,
so replacing the current CV does not rewrite history. Deleting the developer
profile cascades its audit records. Operators should define an appropriate
retention/access policy for these additional personal-data copies.

## Verification

Run `go test ./...`. On the checked upstream baseline, recent Go vet reports an
existing `%d`/string mismatch in `internal/handler/handlers.go` outside this change.
Run `go test -vet=off ./...` separately to execute the regression suite without
claiming that the baseline vet issue is fixed.

The HTTP tests cover anonymous/developer download denial, recruiter/admin access,
missing/audit-failure responses, PDF/size validation, replacement, ownership and
the upload header. The PostgreSQL test is opt-in:

```sh
CV_TEST_DATABASE_URL='postgres://localhost/isolated_cv_test?sslmode=disable' \
  go test -vet=off ./internal/developer -run TestCVRepositoryPostgres -v
```

It uses temporary tables and exercises the actual Go repository queries,
including historical bytes and rollback after audit failure.

Browser verification uses Node.js and Playwright (Chromium installed):

```sh
CV_TEST_DATABASE_URL='postgres://localhost/isolated_cv_test?sslmode=disable' \
CV_NODE=node CV_BROWSER_SCRIPT="$PWD/tests/cv-browser.cjs" \
  go test -vet=off . -run TestCVBrowserIntegration -v
```

Before that browser test, apply migration 074 to a test database containing a
`developer_profile` table with `id`, `email` and `updated_at` columns. It renders
the real templates and invokes real HTTP upload/download handlers and repository
SQL, with fixture-only synthetic sessions. It is not a production-login or
email-delivery test. Optional `CV_PLAYWRIGHT_MODULE`, `CV_CHROMIUM_PATH` and
`CV_SCREENSHOT_DIR` select an existing local browser runtime and output directory.
The browser test checks desktop/mobile upload, persisted availability, developer
denial, recruiter download and CV-control bounds.

Local validation used PostgreSQL WASM/PGlite through its loopback socket with
serialized test packages. This validates SQL/transactions, not multi-session
PostgreSQL concurrency, deployment migrations on production data, or external
mail and payment services. No new runtime dependencies are added.
