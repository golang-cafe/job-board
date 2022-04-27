package blog

import (
	"database/sql"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db}
}

func (r Repository) GetByIDAndAuthor(id, authorID string) (BlogPost, error) {
	var bp BlogPost
	row := r.db.QueryRow(`SELECT id, title, description, tags, slug, text, created_at, updated_at, created_by, published_at FROM blog_post WHERE id = $1 AND created_by = $2`, id, authorID)
	if err := row.Scan(&bp.ID, &bp.Title, &bp.Description, &bp.Tags, &bp.Slug, &bp.Text, &bp.CreatedAt, &bp.UpdatedAt, &bp.CreatedBy, &bp.PublishedAt); err != nil {
		return bp, err
	}

	return bp, nil
}

func (r Repository) GetBySlug(slug string) (BlogPost, error) {
	var bp BlogPost
	row := r.db.QueryRow(`SELECT id, title, description, tags, slug, text, created_at, updated_at, created_by FROM blog_post WHERE slug = $1 AND published_at IS NOT NULL`, slug)
	if err := row.Scan(&bp.ID, &bp.Title, &bp.Description, &bp.Tags, &bp.Slug, &bp.Text, &bp.CreatedAt, &bp.UpdatedAt, &bp.CreatedBy); err != nil {
		return bp, err
	}

	return bp, nil
}

func (r Repository) GetByCreatedBy(userID string) ([]BlogPost, error) {
	all := make([]BlogPost, 0)
	rows, err := r.db.Query(`SELECT id, title, description, tags, slug, text, updated_at, created_by, published_at FROM blog_post WHERE created_by = $1`, userID)
	if err == sql.ErrNoRows {
		return all, nil
	}
	if err != nil {
		return all, err
	}
	for rows.Next() {
		var bp BlogPost
		err := rows.Scan(&bp.ID, &bp.Title, &bp.Description, &bp.Tags, &bp.Slug, &bp.Text, &bp.UpdatedAt, &bp.CreatedBy, &bp.PublishedAt)
		if err != nil {
			return all, err
		}
		all = append(all, bp)
	}

	return all, nil
}

func (r Repository) GetAllPublished() ([]BlogPost, error) {
	all := make([]BlogPost, 0)
	rows, err := r.db.Query(`SELECT id, title, description, tags, slug, text, updated_at, created_by, published_at FROM blog_post WHERE published_at IS NOT NULL`)
	if err == sql.ErrNoRows {
		return all, nil
	}
	if err != nil {
		return all, err
	}
	for rows.Next() {
		var bp BlogPost
		err := rows.Scan(&bp.ID, &bp.Title, &bp.Description, &bp.Tags, &bp.Slug, &bp.Text, &bp.UpdatedAt, &bp.CreatedBy, &bp.PublishedAt)
		if err != nil {
			return all, err
		}
		all = append(all, bp)
	}

	return all, nil
}

func (r Repository) Create(bp BlogPost) error {
	_, err := r.db.Exec(`INSERT INTO blog_post (id, title, description, slug, tags, text, created_at, updated_at, created_by) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW(), $7)`, bp.ID, bp.Title, bp.Description, bp.Slug, bp.Tags, bp.Text, bp.CreatedBy)
	return err
}

func (r Repository) Update(bp BlogPost) error {
	_, err := r.db.Exec(`UPDATE blog_post SET title = $1, description = $2, tags = $3, text = $4, updated_at = NOW() WHERE id = $5 AND created_by = $6`, bp.Title, bp.Description, bp.Tags, bp.Text, bp.ID, bp.CreatedBy)
	return err
}

func (r Repository) Publish(bp BlogPost) error {
	_, err := r.db.Exec(`UPDATE blog_post SET published_at = NOW() WHERE id = $1 AND created_by = $2`, bp.ID, bp.CreatedBy)
	return err
}

func (r Repository) Unpublish(bp BlogPost) error {
	_, err := r.db.Exec(`UPDATE blog_post SET published_at = NULL WHERE id = $1 AND created_by = $2`, bp.ID, bp.CreatedBy)
	return err
}
