-- 001-developer-cv.sql
-- Developer CV upload (bounty issue #74).
--
-- Applies the CV changes to an EXISTING database. Fresh installs get the
-- same statements from latest-schema.sql instead.
-- Safe to run more than once: the ALTERs are guarded, and the audit table
-- and indexes use IF NOT EXISTS.
--
--   psql "$DATABASE_URL" -f migrations/001-developer-cv.sql

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'developer_profile'
          AND column_name = 'linkedin_url'
          AND is_nullable = 'NO'
    ) THEN
        ALTER TABLE ONLY public.developer_profile ALTER COLUMN linkedin_url DROP NOT NULL;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'developer_profile'
          AND column_name = 'cv'
    ) THEN
        ALTER TABLE ONLY public.developer_profile ADD COLUMN cv BYTEA DEFAULT NULL;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS public.developer_cv_download (
    id CHAR(27) NOT NULL,
    developer_profile_id CHAR(27) NOT NULL,
    cv BYTEA NOT NULL,
    user_id CHAR(27) NOT NULL,
    downloaded_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id),
    CONSTRAINT fk_developer_profile FOREIGN KEY (developer_profile_id) REFERENCES public.developer_profile(id) ON DELETE CASCADE,
    CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS developer_cv_download_profile_id_idx ON public.developer_cv_download(developer_profile_id);
CREATE INDEX IF NOT EXISTS developer_cv_download_user_id_idx ON public.developer_cv_download(user_id);
