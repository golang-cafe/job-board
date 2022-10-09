package user

import "time"

const (
	UserTypeDeveloper = "developer"
	UserTypeAdmin     = "admin"
	UserTypeRecruiter = "recruiter"
)

type User struct {
	ID                 string
	Email              string
	CreatedAtHumanised string
	CreatedAt          time.Time
	IsAdmin            bool
	Type               string
}
