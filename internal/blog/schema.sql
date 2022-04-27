CREATE TABLE IF NOT EXISTS blog_post (
	id CHAR(27) NOT NULL,
	title VARCHAR(255) NOT NULL,
	description VARCHAR(255) NOT NULL,
	slug VARCHAR(255) NOT NULL,
	tags VARCHAR(255) NOT NULL,
	text TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	created_by CHAR(27) NOT NULL,
	published_at TIMESTAMP DEFAULT NULL,
	PRIMARY KEY (id)
);

 CREATE UNIQUE INDEX blog_post_slug_idx on blog_post (slug);
