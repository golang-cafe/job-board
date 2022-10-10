CREATE TABLE IF NOT EXISTS developer_profile (
  id        CHAR(27) NOT NULL,
  email       VARCHAR(255) NOT NULL,
  location VARCHAR(255) NOT NULL,
  available BOOLEAN NOT NULL,
  linkedin_url VARCHAR(255) NOT NULL,
  github_url VARCHAR(255) NOT NULL,
  image_id CHAR(27) NOT NULL,
  slug VARCHAR(255) NOT NULL,
  created_at   TIMESTAMP NOT NULL,
  updated_at TIMESTAMP DEFAULT NULL,
  PRIMARY KEY(id)
);

CREATE UNIQUE INDEX developer_profile_slug_idx on developer_profile (slug);
CREATE UNIQUE INDEX developer_profile_email_idx on developer_profile (email);
ALTER TABLE developer_profile ADD COLUMN skills VARCHAR(255) NOT NULL DEFAULT 'Go';
ALTER TABLE developer_profile ADD COLUMN name VARCHAR(255) NOT NULL;
ALTER TABLE developer_profile ADD COLUMN bio TEXT;
ALTER TABLE developer_profile DROP COLUMN github_url;
ALTER TABLE developer_profile ALTER COLUMN bio SET NOT NULL;
ALTER TABLE developer_profile ADD CONSTRAINT developer_profile_image_id_fk FOREIGN KEY (image_id) REFERENCES image(id);
ALTER TABLE developer_profile ADD COLUMN github_url VARCHAR(255) DEFAULT NULL;
ALTER TABLE developer_profile ADD COLUMN twitter_url VARCHAR(255) DEFAULT NULL;

CREATE TABLE IF NOT EXISTS developer_profile_event (
	event_type VARCHAR(128) NOT NULL,
	developer_profile_id CHAR(27) NOT NULL REFERENCES developer_profile(id),
	created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS developer_profile_message (
    id CHAR(27) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    profile_id CHAR(27) NOT NULL,
    created_at TIMESTAMP NOT NULL,
    sent_at TIMESTAMP,
    PRIMARY KEY(id)
);

ALTER TABLE developer_profile ADD COLUMN role_level VARCHAR(20) NOT NULL DEFAULT 'mid-level';
ALTER TABLE developer_profile ADD COLUMN search_status VARCHAR(20) NOT NULL DEFAULT 'casually-looking';
ALTER TABLE developer_profile ADD COLUMN role_types VARCHAR(20) NOT NULL DEFAULT 'full-time';
ALTER TABLE developer_profile ADD COLUMN detected_location_id VARCHAR(255) DEFAULT NULL;
ALTER TABLE developer_profile_message ADD COLUMN sender_id CHAR(27) DEFAULT NULL;
