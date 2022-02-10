package user

import (
	"database/sql"
	"errors"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/segmentio/ksuid"
)

type Repository interface {
	SaveTokenSignOn(email, token string) error
	GetOrCreateUserFromToken(token string) (User, bool, error)
}

type repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) Repository {
	return &repository{db}
}

func (r *repository) SaveTokenSignOn(email, token string) error {
	if _, err := r.db.Exec(`INSERT INTO user_sign_on_token (token, email) VALUES ($1, $2)`, token, email); err != nil {
		return err
	}
	return nil
}

// GetOrCreateUserFromToken creates or get existing user given a token
// returns the user struct, whether the user existed already and an error
func (r *repository) GetOrCreateUserFromToken(token string) (User, bool, error) {
	u := User{}
	row := r.db.QueryRow(`SELECT t.token, t.email, u.id, u.email, u.created_at FROM user_sign_on_token t LEFT JOIN users u ON t.email = u.email WHERE t.token = $1`, token)
	var tokenRes, id, email, tokenEmail sql.NullString
	var createdAt sql.NullTime
	if err := row.Scan(&tokenRes, &tokenEmail, &id, &email, &createdAt); err != nil {
		return u, false, err
	}
	if !tokenRes.Valid {
		return u, false, errors.New("token not found")
	}
	if !email.Valid {
		// user not found create new one
		userID, err := ksuid.NewRandom()
		if err != nil {
			return u, false, err
		}
		u.ID = userID.String()
		u.Email = tokenEmail.String
		u.CreatedAt = time.Now()
		u.CreatedAtHumanised = humanize.Time(u.CreatedAt.UTC())
		if _, err := r.db.Exec(`INSERT INTO users (id, email, created_at) VALUES ($1, $2, $3)`, u.ID, u.Email, u.CreatedAt); err != nil {
			return User{}, false, err
		}

		return u, false, nil
	}
	u.ID = id.String
	u.Email = email.String
	u.CreatedAt = createdAt.Time
	u.CreatedAtHumanised = humanize.Time(u.CreatedAt.UTC())

	return u, true, nil
}
