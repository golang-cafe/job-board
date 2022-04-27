package blog

import "time"

type BlogPost struct {
	ID          string
	Title       string
	Description string
	Tags        string
	Slug        string
	Text        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PublishedAt *time.Time
	CreatedBy   string
}

type CreateRq struct {
	Title       string
	Description string
	Tags        string
	Text        string
}

type UpdateRq struct {
	ID          string
	Title       string
	Description string
	Tags        string
	Text        string
}
