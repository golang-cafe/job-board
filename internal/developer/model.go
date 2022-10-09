package developer

import "time"

const (
	SearchStatusNotAvailable     = "not-available"
	SearchStatusCasuallyLooking  = "casually-looking"
	SearchStatusActivelyApplying = "actively-applying"
)

var ValidSearchStatus = map[string]struct{}{
	SearchStatusActivelyApplying: {},
	SearchStatusCasuallyLooking:  {},
	SearchStatusNotAvailable:     {},
}

var ValidRoleLevels = map[string]struct{}{
	"junior":    {},
	"mid-level": {},
	"senior":    {},
	"lead":      {},
	"c-level":   {},
}

var ValidRoleTypes = map[string]struct{}{
	"full-time":  {},
	"part-time":  {},
	"contract":   {},
	"internship": {},
}

type Developer struct {
	ID                 string
	Name               string
	LinkedinURL        string
	Email              string
	Location           string
	Available          bool
	ImageID            string
	Slug               string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Skills             string
	GithubURL          *string
	TwitterURL         *string
	SearchStatus       string
	RoleLevel          string
	RoleTypes          []string
	DetectedLocationID *string

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
