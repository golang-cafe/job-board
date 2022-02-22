package developer

import "time"

type Developer struct {
	ID          string
	Name        string
	LinkedinURL string
	Email       string
	Location    string
	Available   bool
	ImageID     string
	Slug        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Skills      string
	GithubURL   *string
	TwitterURL  *string

	Bio                string
	SkillsArray        []string
	CreatedAtHumanized string
	UpdatedAtHumanized string
}

type DeveloperMessage struct {
	ID        string
	Email     string
	Content   string
	ProfileID string
	CreatedAt time.Time
	SentAt    time.Time
}
