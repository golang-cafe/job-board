ALTER TABLE image ADD COLUMN developer_photo boolean NOT NULL DEFAULT false;
CREATE INDEX IF NOT EXISTS developer_profile_image_id_idx ON developer_profile(image_id);
