package recruiter

import (
	"time"
)

type Recruiter struct {
	ID         string
	Name       string
	Email      string
	Title      string
	Company    string
	CompanyURL string
	Slug       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	PlanExpiredAt time.Time
}
