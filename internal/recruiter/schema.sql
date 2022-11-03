CREATE TABLE IF NOT EXISTS recruiter_profile (
  id            CHAR(27) NOT NULL,
  email         VARCHAR(255) NOT NULL,
  title         VARCHAR(255) NOT NULL,
  company       VARCHAR(255) NOT NULL,
  company_url   VARCHAR(255) NOT NULL,
  slug          VARCHAR(255) NOT NULL,
  created_at    TIMESTAMP NOT NULL,
  updated_at    TIMESTAMP DEFAULT NULL,
  PRIMARY KEY(id)
);

CREATE UNIQUE INDEX recruiter_profile_slug_idx on recruiter_profile (slug);
CREATE UNIQUE INDEX recruiter_profile_email_idx on recruiter_profile (email);