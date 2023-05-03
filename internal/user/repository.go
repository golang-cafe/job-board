package user

import (
	"database/sql"
	"errors"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/recruiter"
	"github.com/segmentio/ksuid"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db}
}

func (r *Repository) SaveTokenSignOn(email, token, userType string) error {
	if _, err := r.db.Exec(`INSERT INTO user_sign_on_token (token, email, user_type, created_at) VALUES ($1, $2, $3, NOW())`, token, email, userType); err != nil {
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

func (r *Repository) GetUserTypeByEmail(email string) (string, error) {
	var userType string
	row := r.db.QueryRow(`SELECT user_type FROM users WHERE email = $1`, email)
	err := row.Scan(&userType)
	if err == sql.ErrNoRows {
		// check if user is unverified recruiter/developer
		row = r.db.QueryRow(`SELECT 'recruiter' FROM recruiter_profile WHERE email = $1`, email)
		err = row.Scan(&userType)
		if err == nil {
			return userType, nil
		}
		row = r.db.QueryRow(`SELECT 'developer' FROM developer_profile WHERE email = $1`, email)
		err = row.Scan(&userType)
		if err == nil {
			return userType, nil
		}
		return userType, err
	}
	if err != nil {
		return userType, err
	}
	return userType, nil
}

// CreateUserWithEmail creates a new user with given email and user_type
// returns the user struct, and an error
func (r *Repository) CreateUserWithEmail(email, userType string) (User, error) {
	u := User{}

	userID, err := ksuid.NewRandom()
	if err != nil {
		return u, err
	}

	u.ID = userID.String()
	u.Email = email
	u.CreatedAt = time.Now()
	u.Type = userType
	u.CreatedAtHumanised = humanize.Time(u.CreatedAt.UTC())
	if _, err := r.db.Exec(`INSERT INTO users (id, email, created_at, user_type) VALUES ($1, $2, $3, $4)`, u.ID, u.Email, u.CreatedAt, u.Type); err != nil {
		return User{}, err
	}

	return u, nil
}

// GetUserTypeByEmailOrCreateUserIfRecruiter seeks a user in the users table or creates one and recruiter profile if the user has a job posting
// returns user struct and an error
func (r *Repository) GetUserTypeByEmailOrCreateUserIfRecruiter(email string, jobRepo *job.Repository, recRepo *recruiter.Repository) (string, error) {
	u, err := r.GetUserTypeByEmail(email)
	if err == nil {
		return u, nil
	}

	jobsCountByEmail, err := jobRepo.JobsCountByEmail(email)
	if err != nil {
		return "", err
	}
	if jobsCountByEmail > 0 {
		user, err := r.CreateUserWithEmail(email, "recruiter")
		if err == nil {
			err = recRepo.CreateRecruiterProfileBasedOnLastJobPosted(user.Email, jobRepo)
			if err != nil {
				return "", err
			}
			return user.Type, nil
		} else {
			return "", err
		}
	}

	return "", errors.New("not found")
}
