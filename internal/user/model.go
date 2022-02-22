package user

import "time"

type User struct {
	ID                 string
	Email              string
	CreatedAtHumanised string
	CreatedAt          time.Time
	IsAdmin            bool
}
