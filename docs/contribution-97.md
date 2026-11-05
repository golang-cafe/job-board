# 📣 Feature: **Share Daily (Week‑Day Only) Jobs on Twitter**  
**Target repo:** `@mau16ro` (presumably a Node/TS or Python backend)  
**Scope:** Replace the deprecated legacy Twitter integration used by `/x/task/twitter-jobs-scheduler` with a fresh implementation that talks to the **Twitter API v2** (POST tweet endpoint) and only publishes jobs on Monday‑Friday.

---  

## 1️⃣ High‑Level Overview  

| Component | Responsibility |
|-----------|-----------------|
| **Job Producer** | Pulls “daily jobs” from DB / service, filters by weekday, formats tweet text. |
| **Twitter Scheduler Handler** (`/x/task/twitter-jobs-scheduler`) | Executed by a cron (or a job queue) each day at a configurable time. Calls the **Twitter Publisher**. |
| **Twitter Publisher** | Thin wrapper around Twitter API v2 (`POST /2/tweets`). Handles auth, rate‑limit, retries, idempotency. |
| **Persistence / Idempotency Store** | Records `job_id → tweet_id` mappings to avoid duplicate tweets if the handler re‑runs. |
| **Observability** | Structured logs, metrics (success/failure counters, latency), alerts on error spikes. |
| **CI / Tests** | Unit tests for formatting, integration tests with a mock Twitter client, end‑to‑end test against a dev Twitter app. |

---  

## 2️⃣ Prerequisites  

1. **Twitter Developer Account** – create a Project & App with **OAuth 2.0 (Bearer Token) + OAuth 1.0a (User Context)**.  
   - **Bearer token** → for read‑only endpoints (optional).  
   - **OAuth 1.0a user access token & secret** → required for **POST /2/tweets** (needs a user context).  

2. **Environment variables** (store in your secret manager / `.env`):

| Variable | Example | Description |
|----------|---------|-------------|
| `TWITTER_CONSUMER_KEY` | `ABcDeFGhIjKLMNopQRsT` | OAuth 1.0a API key |
| `TWITTER_CONSUMER_SECRET` | `1a2B3c4D5e6F7g8H9iJ0kLmNoP` | OAuth 1.0a API secret |
| `TWITTER_ACCESS_TOKEN` | `1234567890-abcdefgHIJKLmnopqrSTUV` | OAuth 1.0a Access token (user) |
| `TWITTER_ACCESS_TOKEN_SECRET` | `aBcDeFgHiJkLmNoPqRsTuVwXyZ` | OAuth 1.0a Access token secret |
| `TWITTER_TWEET_TEMPLATE` | `"🚀 Today's job: {title} – {url}"` | Template for tweet content |
| `TWITTER_MAX_RETRIES` | `3` | Retry attempts on transient errors |
| `TWITTER_RETRY_BACKOFF_MS` | `2000` | Base back‑off for exponential retries |
| `TWITTER_JOB_SCHEDULE_CRON` | `"0 9 * * MON-FRI"` | Cron expression (UTC) when the handler runs |
| `TWITTER_TWEET_CHAR_LIMIT` | `280` | Hard‑coded but useful for validation |

3. **Permissions** – Ensure the Twitter App has the **“Tweet write”** permission (OAuth 1.0a) and is **approved** for elevated access if you exceed the standard 300 tweets/3 hrs limit (unlikely for a single daily tweet).

---  

## 3️⃣ Architectural Diagram  

```
+-------------------+       +-------------------+       +-----------------------+
|   Daily Job DB    | ----> | Job Producer      | ----> | Tweet Formatter       |
+-------------------+       +-------------------+       +-----------------------+
                                                |
                                                v
                                        +-------------------+
                                        | Twitter Publisher |
                                        +-------------------+
                                                |
                                                v
                                        +-------------------+
                                        | Twitter API v2    |
                                        +-------------------+
```

- The **Job Producer** is invoked by the **Scheduler Handler** (cron).  
- The **Twitter Publisher** is a stateless service that can be unit‑tested in isolation.  

---  

## 4️⃣ Detailed Implementation Steps  

### 4.1 Choose a Language / SDK  

| Language | Recommended SDK | Reason |
|----------|----------------|--------|
| **Node.js / TypeScript** | `twitter-api-v2` (npm) | Full support for OAuth 1.0a & v2 endpoints, TypeScript typings. |
| **Python** | `tweepy` (>=4.0) | Simple, well‑documented, supports OAuth 1.0a. |
| **Go** | `github.com/dghubble/go-twitter` | Good for high‑performance services. |

*Below we’ll provide concrete snippets for **Node.js / TypeScript**, but the same logic can be ported to Python/Go.*

### 4.2 Add New NPM Packages  

```bash
# In the repo root
npm i twitter-api-v2 dayjs cron-parser
npm i -D @types/dayjs
```

- `twitter-api-v2` – modern wrapper for Twitter v2 (includes OAuth 1.0a).  
- `dayjs` – lightweight date handling (weekday detection).  
- `cron-parser` – optional, for validation of the cron string.

### 4.3 Create a **TwitterPublisher** class  

```ts
// src/integrations/twitter-publisher.ts
import { TwitterApi, TwitterApiReadWrite } from 'twitter-api-v2';
import dayjs from 'dayjs';
import { Logger } from '../utils/logger';

export interface TweetPayload {
  text: string;
  // optional media, poll, etc.
}

export class TwitterPublisher {
  private readonly client: TwitterApiReadWrite;
  private readonly maxRetries: number;
  private readonly backoffMs: number;
  private readonly logger: Logger;

  constructor() {
    this.client = new TwitterApi({
      appKey: process.env.TWITTER_CONSUMER_KEY!,
      appSecret: process.env.TWITTER_CONSUMER_SECRET!,
      accessToken: process.env.TWITTER_ACCESS_TOKEN!,
      accessSecret: process.env.TWITTER_ACCESS_TOKEN_SECRET!,
    }).readWrite;

    this.maxRetries = Number(process.env.TWITTER_MAX_RETRIES ?? 3);
    this.backoffMs = Number(process.env.TWITTER_RETRY_BACKOFF_MS ?? 2000);
    this.logger = new Logger('TwitterPublisher');
  }

  /** Send a tweet, idempotent via tweetIdStore (see Section 5). */
  async publish(payload: TweetPayload, jobId: string): Promise<string> {
    // 1️⃣ Validate length (Twitter counts URLs as 23 chars, handle that)
    if (payload.text.length > Number(process.env.TWITTER_TWEET_CHAR_LIMIT ?? 280)) {
      throw new Error(`Tweet exceeds ${process.env.TWITTER_TWEET_CHAR_LIMIT} characters`);
    }

    // 2️⃣ Retry loop (exponential back‑off)
    let attempt = 0;
    while (attempt <= this.maxRetries) {
      try {
        const response = await this.client.v2.tweet(payload.text);
        this.logger.info('Tweet published', { jobId, tweetId: response.data.id });
        return response.data.id; // success
      } catch (err: any) {
        attempt++;
        const isRetryable = this.isRetryableError(err);
        this.logger.warn('Tweet publish failed', {
          jobId,
          attempt,
          error: err.message,
          retryable: isRetryable,
        });

        if (!isRetryable || attempt > this.maxRetries) {
          throw err; // bubble up
        }

        // exponential back‑off
        await new Promise((r) => setTimeout(r, this.backoffMs * Math.pow(2, attempt - 1)));
      }
    }

    throw new Error('Exhausted retries while publishing tweet');
  }

  /** Determine if error is transient (rate‑limit, 5xx, network). */
  private isRetryableError(err: any): boolean {
    if (!err?.code) return false;
    // Twitter v2 uses HTTP status codes in err.status
    const status = err.status ?? 0;
    return status === 429 || (status >= 500 && status < 600);
  }
}
```

### 4.4 Build the **Job Producer / Formatter**  

```ts
// src/tasks/twitter-jobs-scheduler.ts
import { CronJob } from 'cron';
import dayjs from 'dayjs';
import { TwitterPublisher } from '../integrations/twitter-publisher';
import { JobRepository } from '../repositories/job-repository';
import { TweetStore } from '../repositories/tweet-store';
import { Logger } from '../utils/logger';

const logger = new Logger('TwitterJobsScheduler');
const publisher = new TwitterPublisher();
const jobRepo = new JobRepository(); // DB wrapper
const tweetStore = new TweetStore(); // persistence for idempotency

/** Helper – returns true if today is Mon‑Fri */
function isWeekday(date: dayjs.Dayjs = dayjs()): boolean {
  const day = date.day(); // 0 = Sunday, 6 = Saturday
  return day >= 1 && day <= 5;
}

/** Build tweet text using template env var */
function buildTweet(job: { id: string; title: string; url: string }): string {
  const tmpl = process.env.TWITTER_TWEET_TEMPLATE!;
  return tmpl
    .replace('{title}', job.title)
    .replace('{url}', job.url);
}

/** Main handler – can be invoked by cron or HTTP endpoint */
export async function runTwitterJobsScheduler(): Promise<void> {
  if (!isWeekday()) {
    logger.info('Today is weekend – skipping tweet');
    return;
  }

  // 1️⃣ Get today’s “daily job”. Adjust query to your schema.
  const todaysJob = await jobRepo.getDailyJobForDate(new Date());
  if (!todaysJob) {
    logger.warn('No daily job found for today');
    return;
  }

  // 2️⃣ Idempotency check – have we already tweeted this job?
  const alreadyTweeted = await tweetStore.getTweetId(todaysJob.id);
  if (alreadyTweeted) {
    logger.info('Job already tweeted', { jobId: todaysJob.id, tweetId: alreadyTweeted });
    return;
  }

  // 3️⃣ Build tweet payload
  const tweetText = buildTweet(todaysJob);
  const payload = { text: tweetText };

  // 4️⃣ Publish
  const tweetId = await publisher.publish(payload, todaysJob.id);

  // 5️⃣ Persist mapping
  await tweetStore.saveTweetId(todaysJob.id, tweetId);
}

/** Register a cron job (executed by the process manager or a container entrypoint) */
export function scheduleTwitterJobsCron(): void {
  const cronExpr = process.env.TWITTER_JOB_SCHEDULE_CRON ?? '0 9 * * MON-FRI';
  // Using UTC – adjust if you need a specific timezone.
  const job = new CronJob(cronExpr, runTwitterJobsScheduler, null, true, 'UTC');
  logger.info('Twitter jobs scheduler registered', { cron: cronExpr });
}
```

#### 4.4.1 Repository Stubs  

```ts
// src/repositories/job-repository.ts
export class JobRepository {
  async getDailyJobForDate(date: Date) {
    // Example: SELECT * FROM jobs WHERE DATE = $1 AND is_daily = true LIMIT 1;
    // Return shape { id: string; title: string; url: string }
    // Replace with your ORM / query builder.
    // Throw or return null if not found.
  }
}

// src/repositories/tweet-store.ts
import { PrismaClient } from '@prisma/client'; // or any DB client
const prisma = new PrismaClient();

export class TweetStore {
  async getTweetId(jobId: string): Promise<string | null> {
    const rec = await prisma.twitterJobTweet.findUnique({ where: { jobId } });
    return rec?.tweetId ?? null;
  }

  async saveTweetId(jobId: string, tweetId: string): Promise<void> {
    await prisma.twitterJobTweet.upsert({
      where: { jobId },
      update: { tweetId },
      create: { jobId, tweetId },
    });
  }
}
```

> **Schema (Prisma example)**  

```prisma
model TwitterJobTweet {
  jobId   String @id
  tweetId String
  createdAt DateTime @default(now())
}
```

### 4.5 Register the Handler in the Router / Server  

```ts
// src/routes/internal.ts
import express from 'express';
import { runTwitterJobsScheduler } from '../tasks/twitter-jobs-scheduler';

const router = express.Router();

/**
 * POST /x/task/twitter-jobs-scheduler
 * Manual trigger (protected with internal API key)
 */
router.post('/twitter-jobs-scheduler', async (req, res) => {
  // Optional auth middleware
  try {
    await runTwitterJobsScheduler();
    res.status(200).json({ status: 'ok' });
  } catch (e: any) {
    console.error('Scheduler error', e);
    res.status(500).json({ error: e.message });
  }
});

export default router;
```

Add the router to your main server (`