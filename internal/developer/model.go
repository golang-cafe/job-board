package developer

import (
	"sort"
	"strings"
	"time"
)

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

type RoleLevel struct {
	Id           string
	Label        string
	DisplayOrder int
}

var ValidRoleLevels = map[string]RoleLevel{
	"junior":    {"junior", "Junior", 0},
	"mid-level": {"mid-level", "Mid-Level", 1},
	"senior":    {"senior", "Senior", 2},
	"lead":      {"lead", "Lead/Staff Engineer", 3},
	"c-level":   {"c-level", "C-level", 4},
}

func SortedRoleLevels() (sortedRoleLevels []RoleLevel) {
	for _, role := range ValidRoleLevels {
		sortedRoleLevels = append(sortedRoleLevels, role)
	}
	sort.Slice(sortedRoleLevels, func(i, j int) bool {
		return sortedRoleLevels[i].DisplayOrder < sortedRoleLevels[j].DisplayOrder
	})
	return
}

type RoleType struct {
	Id           string
	Label        string
	DisplayOrder int
}

var ValidRoleTypes = map[string]RoleType{
	"full-time":  {"full-time", "Full-Time", 0},
	"part-time":  {"part-time", "Part-Time", 1},
	"contract":   {"contract", "Contract", 2},
	"internship": {"internship", "Internship", 3},
}

func SortedRoleTypes() (sortedRoleTypes []RoleType) {
	for _, role := range ValidRoleTypes {
		sortedRoleTypes = append(sortedRoleTypes, role)
	}
	sort.Slice(sortedRoleTypes, func(i, j int) bool {
		return sortedRoleTypes[i].DisplayOrder < sortedRoleTypes[j].DisplayOrder
	})
	return
}

type Developer struct {
	ID                 string
	Name               string
	LinkedinURL        string
	Email              string
	Location           string
	HourlyRate         int64
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

func (d Developer) RoleTypeAsString() string {
	return strings.Join(d.RoleTypes, ", ")
}

type DeveloperMessage struct {
	ID            string
	Email         string
	Content       string
	RecipientName string
	ProfileID     string
	ProfileSlug   string
	CreatedAt     time.Time
	SentAt        time.Time
	SenderID      string
}

type DeveloperMetadata struct {
	ID                 string
	DeveloperProfileID string
	MetadataType       string
	Title              string
	Description        string
	Link               *string
}

type DevStat struct {
	Date         string `json:"date"`
	PageViews    int    `json:"pageviews"`
	SentMessages int    `json:"messages"`
}
