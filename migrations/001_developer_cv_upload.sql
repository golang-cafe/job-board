-- Migration for golang-cafe/job-board#74: Developer CV upload (bytea) & download audit table
ALTER TABLE developer_profile
    ADD COLUMN IF NOT EXISTS cv BYTEA,
    ADD COLUMN IF NOT EXISTS cv_filename VARCHAR(255),
    ADD COLUMN IF NOT EXISTS cv_uploaded_at TIMESTAMP WITHOUT TIME ZONE;

ALTER TABLE developer_profile
    ALTER COLUMN linkedin_url DROP NOT NULL;

CREATE TABLE IF NOT EXISTS developer_cv_download (
    id VARCHAR(27) NOT NULL PRIMARY KEY,
    developer_profile_id VARCHAR(27) NOT NULL REFERENCES developer_profile(id) ON DELETE CASCADE,
    cv BYTEA NOT NULL,
    user_id VARCHAR(27) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    downloaded_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_developer_cv_download_profile_id
    ON developer_cv_download (developer_profile_id, downloaded_at DESC);
