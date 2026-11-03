BEGIN;
ALTER TABLE developer_profile ADD COLUMN cv bytea;
CREATE TABLE developer_cv_download (
    id bigserial PRIMARY KEY,
    developer_profile_id character(27) NOT NULL REFERENCES developer_profile(id) ON DELETE CASCADE,
    cv bytea NOT NULL,
    user_id character(27) NOT NULL,
    downloaded_at timestamp without time zone NOT NULL
);
CREATE INDEX developer_cv_download_profile_idx ON developer_cv_download (developer_profile_id, downloaded_at);
COMMIT;
