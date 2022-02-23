CREATE TABLE IF NOT EXISTS job (
  id        		   SERIAL NOT NULL,
  job_title          VARCHAR(128) NOT NULL,
  company            VARCHAR(128) NOT NULL,
  company_url        VARCHAR(128),
  company_twitter    VARCHAR(128),
  company_email      VARCHAR(128),
  salary_range       VARCHAR(100) NOT NULL,
  location           VARCHAR(200) NOT NULL,
  description        TEXT NOT NULL,
  perks              TEXT,
  interview_process  TEXT,
  how_to_apply       VARCHAR(512),
  created_at         TIMESTAMP NOT NULL,
  approved_at        TIMESTAMP,
  url_id             INTEGER NOT NULL,
  slug               VARCHAR(256),
  PRIMARY KEY (id)
);

 CREATE UNIQUE INDEX url_id_idx on job (url_id);
 CREATE UNIQUE INDEX slug_idx on job (slug);
 ALTER TABLE job ADD COLUMN salary_min INTEGER NOT NULL DEFAULT 1;
 ALTER TABLE job ADD COLUMN salary_max INTEGER NOT NULL DEFAULT 1;
 ALTER TABLE job ADD COLUMN salary_currency VARCHAR(4) NOT NULL DEFAULT '$';
 ALTER TABLE job ADD COLUMN external_id VARCHAR(28) NOT NULL;
 ALTER TABLE job ADD COLUMN external_id VARCHAR(28) DROP DEFAULT;
 ALTER TABLE job ADD COLUMN ad_type INTEGER NOT NULL DEFAULT 0;
 ALTER TABLE job ALTER COLUMN company_url SET NOT NULL;
 ALTER TABLE job ADD COLUMN company_icon_image_id VARCHAR(255) DEFAULT NULL;
 ALTER TABLE job ADD COLUMN salary_period VARCHAR(10) NOT NULL DEFAULT 'year';
 ALTER TABLE job ADD COLUMN estimated_salary BOOLEAN DEFAULT FALSE;
 ALTER TABLE job ADD COLUMN expired BOOLEAN DEFAULT FALSE;
 ALTER TABLE job ADD COLUMN last_week_clickouts INTEGER NOT NULL DEFAULT 0;
 ALTER TABLE job ADD COLUMN salary_currency_iso CHAR(3) DEFAULT NULL;
 ALTER TABLE job ADD COLUMN visa_sponsorship BOOLEAN DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS job_event (
  event_type VARCHAR(128) NOT NULL,
  job_id INTEGER NOT NULL REFERENCES job (id),
  created_at TIMESTAMP NOT NULL
);

CREATE INDEX job_idx ON job_event (job_id);