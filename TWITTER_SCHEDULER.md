# X API v2 Job Scheduler

Both machine-authenticated POST routes use the same scheduler and persistent
state: `/x/task/twitter-jobs-scheduler` and the legacy `/x/task/twitter-scheduler`.
Send the configured `x-machine-token`; empty configuration fails closed. The
handler reports the number acknowledged during that invocation instead of
starting an unobserved background goroutine.

## Deployment

Apply `migrations/097_twitter_jobs.sql` once on an existing database before
deploying. Fresh `latest-schema.sql` includes it already; do not apply twice.
The migration copies the existing `last_twitted_job_id` from `meta` (or starts at
zero if absent). Invalid existing integer values fail the migration rather than
silently resetting the cursor. Completed posts update that legacy cursor too.

Reuse `TWITTER_CLIENT_KEY`, `TWITTER_CLIENT_SECRET`, `TWITTER_ACCESS_TOKEN` and
`TWITTER_ACCESS_TOKEN_SECRET`. These must be OAuth 1.0a **user-context** credentials
for a write-enabled X app/account, not an app-only bearer token. The existing
OAuth library signs JSON `POST https://api.x.com/2/tweets`; no additional runtime
dependency, OAuth registration, or credential rotation is introduced.

X's endpoint authentication mapping explicitly supports user-context OAuth 1.0a:
https://docs.x.com/fundamentals/authentication/guides/v2-authentication-mapping .
The create-post endpoint is documented at
https://docs.x.com/x-api/posts/create-post .
The operator is responsible for current provider access, account permissions,
API costs and consent to publish; this change does not purchase API access or
perform an account setup.

Schedule one invocation per weekday with an appropriate HTTP timeout (two API
calls can each take up to 20 seconds). Scheduling days are UTC, Monday-Friday.
There is a hard cap of two durable reservations per UTC scheduling day, across
both aliases and repeated invocations; `TWITTER_JOBS_TO_POST` does not increase
this cap. The same invocation's two reservations use its starting UTC day.
Approved jobs are selected in increasing ID order after the acknowledged cursor.
No jobs, a weekend, or an already exhausted daily quota returns zero posted.

Post text contains a whitespace-normalized Unicode-safe teaser (up to 100 code
points) followed by the canonical job link. The API remains the final text
validator; this is not a full implementation of X's weighted-text/URL parsing
rules. Unexpected text rejections stop scheduling like other API failures.

## Uncertain Delivery

`pending_job_id` is committed **before** sending. A concurrent trigger sees it
and stops. Only an acknowledged post whose journal/cursor transaction commits
clears it. HTTP errors, cancellation, bad acknowledgements, or database failure
after delivery leave it intact. Even an explicit API rejection stops for review;
there are no blind automatic retries or cursor skips.

This is not exactly-once delivery across PostgreSQL and X. A timeout can mean
that X published the post while the application did not receive confirmation.
Before resuming, the operator must disable concurrent triggers, wait for any
in-flight request to finish, inspect the pending job and the X account, and
reconcile the result in a database transaction. If published, record the real
post ID in `twitter_post`, advance both cursors, and clear the pending field. If
confirmed not published, clear the pending field; only release its reserved daily
slot if that is still the current scheduling day. Never clear it just because
an API or database call returned an error. No reconciliation endpoint that can
silently repost is exposed.

## Validation

`go test ./internal/twitter` runs loopback HTTP, independent HMAC signature,
redirect, redaction and weekend tests. Set `X97_TEST_DATABASE_URL` to an isolated
PostgreSQL database and run `go test -p=1 -vet=off ./... -count=1 -v` for SQL tests.
The tests create and clean up uniquely named fixture schemas, apply the actual
migration, and use only synthetic credentials and loopback servers. No production
data, X accounts, live posts, paid API calls or environment credentials are used.

The development run used PostgreSQL WASM (PGlite) over TCP with lib/pq, CGO disabled,
and serialized packages. It covers ordered cursor migration, two/day and cross-day
limits, UTC boundaries, partial delivery failure, acknowledgement-write failure,
pending-state interleaving, cancellation, missing state, and the actual signed
HTTP-client/SQL combination. This is not a multi-session native PostgreSQL stress
test, production deployment, cron installation or live-provider validation.

Default `go test` still fails an unchanged baseline fmt vet error in
`internal/handler/handlers.go` (`%d` used with string `externalID`, now line 2643).
It is not claimed as passing. The initial draft also incorrectly treated missing
scheduler state as no work; a failing regression test exposed that and the final
full integration rerun passes after the fix.
