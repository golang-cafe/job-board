CREATE TABLE linkedin_scheduler_state (
 id integer PRIMARY KEY CHECK (id=1),
 last_job_id integer NOT NULL DEFAULT 0,
 pending_job_id integer
);
INSERT INTO linkedin_scheduler_state(id) VALUES(1);
CREATE TABLE linkedin_post (
 job_id integer PRIMARY KEY,
 post_id text NOT NULL CHECK(post_id <> ''),
 created_at timestamp with time zone NOT NULL DEFAULT now()
);
