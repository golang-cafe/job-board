package user

import (
	"database/sql"
	"errors"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/segmentio/ksuid"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db}
}

func (r *Repository) SaveTokenSignOn(email, token, userType string) error {
	if _, err := r.db.Exec(`INSERT INTO user_sign_on_token (token, email, user_type, created_at) VALUES ($1, $2, $3, $4)`, token, email, userType, time.Now().UTC()); err != nil {
		return err
	}
	return nil
}

// GetOrCreateUserFromToken creates or get existing user given a token
// returns the user struct, whether the user existed already and an error
func (r *Repository) GetOrCreateUserFromToken(token string) (User, bool, error) {
	u := User{}
	row := r.db.QueryRow(`SELECT t.token, t.email, u.id, u.email, u.created_at, t.user_type FROM user_sign_on_token t LEFT JOIN users u ON t.email = u.email WHERE t.token = $1`, token)
	var tokenRes, id, email, tokenEmail, userType sql.NullString
	var createdAt sql.NullTime
	if err := row.Scan(&tokenRes, &tokenEmail, &id, &email, &createdAt, &userType); err != nil {
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
		u.Type = userType.String
		u.CreatedAtHumanised = humanize.Time(u.CreatedAt.UTC())
		if _, err := r.db.Exec(`INSERT INTO users (id, email, created_at, user_type) VALUES ($1, $2, $3, $4)`, u.ID, u.Email, u.CreatedAt, u.Type); err != nil {
			return User{}, false, err
		}

		return u, false, nil
	}
	u.ID = id.String
	u.Email = email.String
	u.CreatedAt = createdAt.Time
	u.Type = userType.String
	u.CreatedAtHumanised = humanize.Time(u.CreatedAt.UTC())

	return u, true, nil
}

func (r *Repository) DeleteUserByEmail(email string) error {
	_, err := r.db.Exec(`DELETE FROM users WHERE email = $1`, email)
	return err
}

// DeleteExpiredUserSignOnTokens deletes user_sign_on_tokens older than 1 week
func (r *Repository) DeleteExpiredUserSignOnTokens() error {
	_, err := r.db.Exec(`DELETE FROM user_sign_on_token WHERE created_at < NOW() - INTERVAL '7 DAYS'`)
	return err
}

func (r *Repository) GetUser(email string) (User, error) {
	u := User{}
	row := r.db.QueryRow(`SELECT id, email, created_at, user_type FROM users WHERE email = $1`, email)
	if err := row.Scan(&u.ID, &u.Email, &u.CreatedAt, &u.Type); err != nil {
		return u, err
	}
	return u, nil
}
