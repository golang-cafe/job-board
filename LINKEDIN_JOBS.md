# LinkedIn job sharing

Apply `migrations/091_linkedin_jobs.sql` once to an existing database before
enabling the task. New databases include these tables in `latest-schema.sql`.
The initial cursor is zero: approved historical jobs are eligible. Before first
use, an operator can set `last_job_id` to an appropriate existing job ID if only
newer jobs should be published. Unapproved jobs are skipped; approving an older
ID after the cursor passes it does not backfill that job.

Configure these optional environment variables (unrelated routes still start
without them):

- `LINKEDIN_ACCESS_TOKEN`: operator-provided OAuth2 access token. Do not commit it.
- `LINKEDIN_AUTHOR_URN`: `urn:li:person:...` or `urn:li:organization:...`.
  A vanity page URL is not an author URN; use the actual authorized ID.
- `LINKEDIN_API_VERSION`: an operator-selected, supported `YYYYMM` version.
  Format validation does not prove that a version is enabled or still supported.
- Existing `MACHINE_TOKEN`, `URL_PROTOCOL` and `SITE_HOST` configure access and
  canonical job links as elsewhere in this application.

This uses the versioned Posts API, not the superseded UGC API linked in the
original issue. Member posting needs `w_member_social`; organization posting
needs `w_organization_social` and a permitted company-page role. Application
product access, token issuance/renewal, permissions and any costs remain the
deploying operator's responsibility. This code does not create accounts, obtain
credentials or authorize access on an operator's behalf.

References:

- https://learn.microsoft.com/en-us/linkedin/consumer/integrations/self-serve/share-on-linkedin
- https://learn.microsoft.com/en-us/linkedin/marketing/community-management/shares/posts-api

## Task behavior

`POST /x/task/linkedin-share-jobs` with the existing `x-machine-token` header
reserves **one** approved job in ascending ID order per invocation. Invoke it
from the existing external task scheduler at the desired posting cadence. This
change does not install a cron job or impose an unrequested daily quota.

The reservation commits before a detached, 30-second background operation sends
the post. `202` means queued, not published. `200` with `queued:0` means no eligible
job. Missing configuration fails closed (`503`), invalid machine authentication
returns `401`, and a pending reservation blocks another trigger (`409`).

The post includes a short job summary and canonical job URL. A `201` response
with a valid `x-restli-id` is required before the acknowledgement journal and
cursor are committed together. Redirects are not followed; raw provider bodies,
transport errors and SQL errors are not exposed in task responses or worker logs.
Failures, timeout, process termination, or acknowledgement-write failures leave
the reservation pending. There is deliberately no automatic HTTP retry.

## Pending reconciliation

PostgreSQL and LinkedIn do not share a transaction. This is not an exactly-once
guarantee or a durable background worker: restart recovery requires an operator.
Pause external triggers, inspect `linkedin_scheduler_state.pending_job_id`, and
check the authorized member/page's actual posts. Do not simply clear the row on
a timeout. If publication succeeded, transactionally insert the actual post ID
into `linkedin_post`, advance `last_job_id` to the pending ID, and clear pending.
Only after confirming non-publication should an operator clear pending without
advancing the cursor and invoke the task again. Resume triggers after review.

## Tests and limits

`CGO_ENABLED=0 go test -vet=off ./...` runs unit tests. Set
`LINKEDIN91_TEST_DATABASE_URL` to a disposable PostgreSQL instance to also run
isolated random-schema SQL tests; without it those tests explicitly skip.
These tests apply the real migration and exercise reservations, ordering,
unapproved filtering, successful HTTP acknowledgements and pending failure
paths. Local HTTP fixtures assert endpoint, authorization and payload, not live
LinkedIn app access, token validity, rendered link previews or account acceptance.
The existing baseline `handlers.go` formatting diagnostic prevents default vet
from passing; this change does not suppress vet globally.
