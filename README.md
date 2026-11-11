## Job Board

This is the first Go job board with no recruiters & clear salary ranges.

- Clear salary range in each job description
- No third party recruiters (apply directly to companies)
- All jobs are manually vetted and reviewed
- Browse salary trends by region
- Browse companies hiring Go engineers and using Go in production
- Browse Go developers
- Weekly Job Newsletter Digest
- Open Source
- Filter by minimum Salary
- The site home page weights under 250kb ([188kb uncompressed](https://gtmetrix.com/reports/golang.cafe/FQEvpFuT/))

### Tech Stack

- [Go](https://golang.org)
- [HTML](https://www.w3.org/html/)
- [CSS](https://developer.mozilla.org/en-US/docs/Web/CSS)
- [JavaScript](https://developer.mozilla.org/en-US/docs/Web/JavaScript)
- [PostgreSQL](https://www.postgresql.org)
- [Digital Ocean App Platform](https://www.digitalocean.com/products/app-platform/)
- [Cloudflare](https://cloudflare.com)

### Local Development Mode - Setup Guide

It's possible to setup a blank instance of this job board locally. This is a local development mode and it's different from the way the app runs in production. In this scenario the app runs on a minimal database containing mock data. The app has reduced functionality when ran in local development mode. It's not possible to send emails or connect to third party services, like Twitter, Telegram and FX APIs. It's still possible to run and test the app but with limited functionality.

**Requirements**

These are basic requirements with the respective versions have been tested to work locally on MacOS. The same should apply both on Linux and WSL/Windows.

- Bash 3.2.x or higher
- Docker 20.10.x or higher
- Go 1.15.x or higher

**Dependencies**

- **PostgreSQL instance** mocked using local Docker container, with local schema and fixtures
- **Sparkpost Mail** emails are sent using http requests through Sparkpost APIs. This is not enabled in local development mode.
- **Telegram API** telegram updates are sent through the Job Board official Telegram channel. This is not enabled in local development mode.
- **Twitter API** twitter updates are sent through the Job Board official page. This is not enabled in local development mode.
- **FX API** a minimal set of Foreign Currency Exchange data is kept up-to-date to filter out salary ranges. This is not enabled in local development mode.

**Setup Guide**

The only thing that needs to be setup in order for the app to run is the PostgreSQL database instance. Please run the following command in order to setup your local database instance.

```
./setup-database.sh
```

Once this command is successful you can now start the application

```
./run-local-webserver.sh
```

**Test Cron-Jobs Locally**

There are a few cron-jobs that are triggered on a schedule, these are scheduled externally via custom http calls to the website. You can see all available cron-jobs by searching for all routes that start with /x/task/<task-name>.

In order to test a cron-job you can just make the following http request

```
curl -v -H 'x-machine-token: <machine-token>' -X POST http://localhost:9876/x/task/<task-name>
```

You can find the <machine-token> under your local environment variable configuration, in the `MACHINE_TOKEN` environment variable.

#### Sharing jobs on X (Twitter)

New jobs are shared on X once per weekday via the X API v2 `POST /2/tweets`
endpoint (OAuth 1.0a user context, no external Twitter library). Trigger it
with the machine-authenticated task route (the old
`/x/task/twitter-scheduler` route still works as an alias):

```
curl -v -H 'x-machine-token: <machine-token>' -X POST http://localhost:9876/x/task/twitter-jobs-scheduler
```

Behaviour: weekday-only (Monday–Friday in `TWITTER_TIMEZONE`), at most 2
jobs per day, once-per-day idempotent (tracked in the `meta` table as
`last_twitter_shared_date` / `last_twitted_job_id`). A job that X rejects
permanently (e.g. duplicate content) is skipped so it cannot block later
jobs; auth failures, rate limits (`429`, honouring `Retry-After`) and 5xx
errors abort the run and the cursor stays at the last posted job.

| Variable | Required | Default | Notes |
|---|---|---|---|
| `TWITTER_CLIENT_KEY`, `TWITTER_CLIENT_SECRET`, `TWITTER_ACCESS_TOKEN`, `TWITTER_ACCESS_TOKEN_SECRET` | Only when posting | — | OAuth 1.0a user-context credentials, all four or none. An app-only bearer token **cannot** create posts, so there is no bearer fallback. |
| `TWITTER_JOBS_TO_POST` | No | `2` | Clamped to at most 2 per day. |
| `TWITTER_API_URL` | No | `https://api.twitter.com/2/tweets` | Override for tests. |
| `TWITTER_TIMEZONE` | No | `UTC` | IANA timezone for the weekday / once-per-day rules, e.g. `America/New_York`. |
| `TWITTER_DRY_RUN` | No | `false` | `1`/`true` logs the exact tweet text instead of posting — verify the scheduler without spending paid X API quota (X charges per post). |

Suggested cron (weekday mornings; the handler also enforces weekdays
itself, so this is belt-and-braces):

```
0 9 * * 1-5 curl -s -H 'x-machine-token: <machine-token>' -X POST https://<host>/x/task/twitter-jobs-scheduler
```

### Telegram Group OSS discussions

https://t.me/+VloraT7W9yA1YTI8

### Feedback?

Feel free to open an issue on GitHub

### License

This source code is licensed under [BSD 3-Clause License](LICENSE.txt)
