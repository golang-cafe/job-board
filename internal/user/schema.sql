CREATE TABLE IF NOT EXISTS users (
	id CHAR(27) NOT NULL UNIQUE,
	email VARCHAR(255) NOT NULL,
	username VARCHAR(255) NOT NULL,
	created_at TIMESTAMP,
	PRIMARY KEY (id)
);
ALTER TABLE users DROP COLUMN username;

CREATE TABLE IF NOT EXISTS user_sign_on_token (
	token CHAR(27) NOT NULL UNIQUE,
	email VARCHAR(255) NOT NULL
);

CREATE INDEX user_sign_on_token_token_idx on user_sign_on_token (token);

CREATE TABLE IF NOT EXISTS edit_token (
  token      CHAR(27) NOT NULL,
  job_id     INTEGER NOT NULL REFERENCES job (id),
  created_at TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX token_idx on edit_token (token);
