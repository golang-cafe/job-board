package blog

import "time"

type BlogPost struct {
	Title       string
	Description string
	Content     string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
}
