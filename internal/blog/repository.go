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

func (r Repository) GetBySlug(slug string) (BlogPost, error) {
	var bp BlogPost
	return bp, nil
}

func (r Repository) Create(blogPost BlogPost) error {
	return nil
}

func (r Repository) Update(blogPost BlogPost) error {

}
