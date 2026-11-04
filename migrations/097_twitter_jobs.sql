CREATE TABLE twitter_scheduler_state (
    id integer PRIMARY KEY CHECK (id = 1),
    last_job_id integer NOT NULL,
    posting_day date,
    posts_today integer NOT NULL DEFAULT 0 CHECK (posts_today BETWEEN 0 AND 2),
    pending_job_id integer
);
INSERT INTO twitter_scheduler_state(id,last_job_id)
VALUES (1, COALESCE((SELECT value::integer FROM meta WHERE key='last_twitted_job_id'),0));
CREATE TABLE twitter_post (
    job_id integer PRIMARY KEY,
    tweet_id text NOT NULL,
    posting_day date NOT NULL
);
