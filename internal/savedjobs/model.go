package savedJobs

import (
	"time"
)

type JobBookmark struct {
	DeveloperID string
	JobID       string
	SavedAt     time.Time
	AppliedAt   time.Time
}

type JobBookmarkRq struct {
	DeveloperID string
	JobID       int
}
