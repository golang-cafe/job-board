package developer

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/gosimple/slug"
)

const (
	developerProfileEventPageView    = "developer_profile_page_view"
	developerProfileEventMessageSent = "developer_profile_message_sent"
	SearchTypeDeveloper              = "developer"
)

type Repository interface {
	DeveloperProfileBySlug(slug string) (Developer, error)
	DeveloperProfileByEmail(email string) (Developer, error)
	DeveloperProfileByID(id string) (Developer, error)
	SendMessageDeveloperProfile(message DeveloperMessage) error
	MessageForDeliveryByID(id string) (DeveloperMessage, string, error)
	MarkDeveloperMessageAsSent(id string) error
	DevelopersByLocationAndTag(loc, tag string, pageID, pageSize int) ([]Developer, int, error)
	UpdateDeveloperProfile(dev Developer) error
	DeleteDeveloperProfile(id, email string) error
	DeleteUserByEmail(email string) error // TO CONFIRM
	ActivateDeveloperProfile(email string) error
	SaveDeveloperProfile(dev Developer) error // TO CONFIRM
	TrackDeveloperProfileView(dev Developer) error
	TrackDeveloperProfileMessageSent(dev Developer) error
	GetLastDevUpdatedAt() (time.Time, error)
	GetDevelopersRegisteredLastMonth() (int, error)
	GetDeveloperMessagesSentLastMonth() (int, error)
	GetDeveloperProfilePageViewsLastMonth() (int, error)
	GetTopDevelopers(limit int) ([]Developer, error)
	GetTopDeveloperSkills(limit int) ([]string, error)
	GetDeveloperSkills() ([]string, error)
	GetDeveloperSlugs() ([]string, error)
}

type repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) Repository {
	return &repository{db}
}

func (r *repository) DeveloperProfileBySlug(slug string) (Developer, error) {
	row := r.db.QueryRow(`SELECT id, email, location, available, linkedin_url, image_id, slug, created_at, updated_at, skills, name, bio FROM developer_profile WHERE slug = $1`, slug)
	dev := Developer{}
	err := row.Scan(
		&dev.ID,
		&dev.Email,
		&dev.Location,
		&dev.Available,
		&dev.LinkedinURL,
		&dev.ImageID,
		&dev.Slug,
		&dev.CreatedAt,
		&dev.UpdatedAt,
		&dev.Skills,
		&dev.Name,
		&dev.Bio,
	)
	if err != nil {
		return dev, err
	}

	return dev, nil
}

func (r *repository) DeveloperProfileByEmail(email string) (Developer, error) {
	row := r.db.QueryRow(`SELECT id, email, location, available, linkedin_url, image_id, slug, created_at, updated_at, skills, name, bio FROM developer_profile WHERE lower(email) = lower($1)`, email)
	dev := Developer{}
	err := row.Scan(
		&dev.ID,
		&dev.Email,
		&dev.Location,
		&dev.Available,
		&dev.LinkedinURL,
		&dev.ImageID,
		&dev.Slug,
		&dev.CreatedAt,
		&dev.UpdatedAt,
		&dev.Skills,
		&dev.Name,
		&dev.Bio,
	)
	if err == sql.ErrNoRows {
		return dev, nil
	}
	if err != nil {
		return dev, err
	}

	return dev, nil
}

func (r *repository) DeveloperProfileByID(id string) (Developer, error) {
	row := r.db.QueryRow(`SELECT id, email, location, available, linkedin_url, image_id, slug, created_at, updated_at, skills, name, bio FROM developer_profile WHERE id = $1`, id)
	dev := Developer{}
	err := row.Scan(
		&dev.ID,
		&dev.Email,
		&dev.Location,
		&dev.Available,
		&dev.LinkedinURL,
		&dev.ImageID,
		&dev.Slug,
		&dev.CreatedAt,
		&dev.UpdatedAt,
		&dev.Skills,
		&dev.Name,
		&dev.Bio,
	)
	if err != nil {
		return dev, err
	}

	return dev, nil
}

func (r *repository) SendMessageDeveloperProfile(message DeveloperMessage) error {
	_, err := r.db.Exec(
		`INSERT INTO developer_profile_message (id, email, content, profile_id, created_at) VALUES ($1, $2, $3, $4, NOW())`,
		message.ID,
		message.Email,
		message.Content,
		message.ProfileID,
	)
	return err
}

func (r *repository) MessageForDeliveryByID(id string) (DeveloperMessage, string, error) {
	row := r.db.QueryRow(`SELECT dpm.id, dpm.email, dpm.content, dpm.profile_id, dpm.created_at, dp.email as dev_email FROM developer_profile_message dpm JOIN developer_profile dp ON dp.id = dpm.profile_id WHERE dpm.id = $1 AND dpm.sent_at IS NULL`, id)
	var devEmail string
	var message DeveloperMessage
	err := row.Scan(
		&message.ID,
		&message.Email,
		&message.Content,
		&message.ProfileID,
		&message.CreatedAt,
		&devEmail,
	)
	if err != nil {
		return message, devEmail, err
	}

	return message, devEmail, nil
}

func (r *repository) MarkDeveloperMessageAsSent(id string) error {
	_, err := r.db.Exec(`UPDATE developer_profile_message SET sent_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *repository) DevelopersByLocationAndTag(loc, tag string, pageID, pageSize int) ([]Developer, int, error) {
	var rows *sql.Rows
	var err error
	offset := pageID*pageSize - pageSize
	var developers []Developer
	switch {
	case tag != "" && loc != "":
		rows, err = r.db.Query(`SELECT count(*) OVER() AS full_count, id, email, location, available, linkedin_url, image_id, slug, created_at, updated_at, skills, name, bio FROM developer_profile WHERE location ILIKE '%' || $1 || '%' AND skills ILIKE '%' || $2 || '%' AND created_at != updated_at ORDER BY updated_at DESC LIMIT $3 OFFSET $4`, loc, tag, pageSize, offset)
	case tag != "" && loc == "":
		rows, err = r.db.Query(`SELECT count(*) OVER() AS full_count, id, email, location, available, linkedin_url, image_id, slug, created_at, updated_at, skills, name, bio FROM developer_profile WHERE skills ILIKE '%' || $1 || '%' AND created_at != updated_at ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, tag, pageSize, offset)
	case tag == "" && loc != "":
		rows, err = r.db.Query(`SELECT count(*) OVER() AS full_count, id, email, location, available, linkedin_url, image_id, slug, created_at, updated_at, skills, name, bio FROM developer_profile WHERE location ILIKE '%' || $1 || '%' AND created_at != updated_at ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, loc, pageSize, offset)
	default:
		rows, err = r.db.Query(`SELECT count(*) OVER() AS full_count, id, email, location, available, linkedin_url, image_id, slug, created_at, updated_at, skills, name, bio FROM developer_profile WHERE created_at != updated_at ORDER BY updated_at DESC LIMIT $1 OFFSET $2`, pageSize, offset)
	}
	if err == sql.ErrNoRows {
		return developers, 0, nil
	}
	var fullRowsCount int
	for rows.Next() {
		var dev Developer
		err := rows.Scan(
			&fullRowsCount,
			&dev.ID,
			&dev.Email,
			&dev.Location,
			&dev.Available,
			&dev.LinkedinURL,
			&dev.ImageID,
			&dev.Slug,
			&dev.CreatedAt,
			&dev.UpdatedAt,
			&dev.Skills,
			&dev.Name,
			&dev.Bio,
		)
		if err != nil {
			return developers, fullRowsCount, err
		}
		developers = append(developers, dev)
	}

	return developers, fullRowsCount, nil
}

func (r *repository) UpdateDeveloperProfile(dev Developer) error {
	_, err := r.db.Exec(`UPDATE developer_profile SET name = $1, location = $2, linkedin_url = $3, bio = $4, available = $5, image_id = $6, updated_at = NOW(), skills = $7  WHERE id = $8`, dev.Name, dev.Location, dev.LinkedinURL, dev.Bio, dev.Available, dev.ImageID, dev.Skills, dev.ID)
	return err
}

func (r *repository) DeleteDeveloperProfile(id, email string) error {
	_, err := r.db.Exec(`DELETE FROM developer_profile WHERE id = $1 AND email = $2`, id, email)
	return err
}

func (r *repository) DeleteUserByEmail(email string) error {
	_, err := r.db.Exec(`DELETE FROM users WHERE email = $1`, email)
	return err
}

func (r *repository) ActivateDeveloperProfile(email string) error {
	_, err := r.db.Exec(`UPDATE developer_profile SET updated_at = NOW() WHERE email = $1`, email)
	return err
}

func (r *repository) SaveDeveloperProfile(dev Developer) error {
	dev.Slug = slug.Make(fmt.Sprintf("%s %d", dev.Name, time.Now().UTC().Unix()))
	_, err := r.db.Exec(`INSERT INTO developer_profile (email, location, linkedin_url, bio, available, image_id, slug, created_at, updated_at, skills, name, id, github_url, twitter_url) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW(), $8, $9, $10, $11, $12)`, dev.Email, dev.Location, dev.LinkedinURL, dev.Bio, dev.Available, dev.ImageID, dev.Slug, dev.Skills, dev.Name, dev.ID, dev.GithubURL, dev.TwitterURL)
	return err
}

func (r *repository) GetTopDevelopers(limit int) ([]Developer, error) {
	devs := make([]Developer, 0, limit)
	var rows *sql.Rows
	rows, err := r.db.Query(`select name, image_id from developer_profile where updated_at != created_at order by updated_at desc limit $1`, limit)
	if err != nil {
		return devs, err
	}
	defer rows.Close()
	for rows.Next() {
		var dev Developer
		if err := rows.Scan(&dev.Name, &dev.ImageID); err != nil {
			return devs, err
		}
		devs = append(devs, dev)
	}

	return devs, nil
}

func (r *repository) GetTopDeveloperSkills(limit int) ([]string, error) {
	skills := make([]string, 0, limit)
	var rows *sql.Rows
	rows, err := r.db.Query(`select count(*) c, trim(both from unnest(regexp_split_to_array(skills, ','))) as skill from developer_profile where updated_at != created_at group by skill order by c desc limit $1`, limit)
	if err != nil {
		return skills, err
	}
	defer rows.Close()
	for rows.Next() {
		var c int
		var skill string
		if err := rows.Scan(&c, &skill); err != nil {
			return skills, err
		}
		skills = append(skills, skill)
	}

	return skills, nil

}

func (r *repository) GetDeveloperSkills() ([]string, error) {
	skills := make([]string, 0)
	var rows *sql.Rows
	rows, err := r.db.Query(`select distinct trim(both from unnest(regexp_split_to_array(skills, ','))) as skill from developer_profile where updated_at != created_at`)
	if err != nil {
		return skills, err
	}
	defer rows.Close()
	for rows.Next() {
		var skill string
		if err := rows.Scan(&skill); err != nil {
			return skills, err
		}
		skills = append(skills, skill)
	}

	return skills, nil
}

func (r *repository) GetDeveloperSlugs() ([]string, error) {
	slugs := make([]string, 0)
	var rows *sql.Rows
	rows, err := r.db.Query(`select slug from developer_profile where updated_at != created_at`)
	if err != nil {
		return slugs, err
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return slugs, err
		}
		slugs = append(slugs, slug)
	}

	return slugs, nil
}

func (r *repository) GetLastDevUpdatedAt() (time.Time, error) {
	var updatedAt time.Time
	row := r.db.QueryRow(`SELECT updated_at FROM developer_profile WHERE updated_at != created_at ORDER BY updated_at DESC LIMIT 1`)
	if err := row.Scan(&updatedAt); err != nil {
		return updatedAt, err
	}

	return updatedAt, nil
}

func (r *repository) GetDevelopersRegisteredLastMonth() (int, error) {
	var count int
	row := r.db.QueryRow(`select count(*) from developer_profile where created_at > NOW() - INTERVAL '30 days'`)
	if err := row.Scan(&count); err != nil {
		return count, err
	}

	return count, nil
}

func (r *repository) GetDeveloperMessagesSentLastMonth() (int, error) {
	var count int
	row := r.db.QueryRow(`select count(*) from developer_profile_message where created_at > NOW() - INTERVAL '30 days'`)
	if err := row.Scan(&count); err != nil {
		return count, err
	}

	return count, nil
}

func (r *repository) GetDeveloperProfilePageViewsLastMonth() (int, error) {
	var count int
	row := r.db.QueryRow(`select count(*) as c from developer_profile_event where event_type = 'developer_profile_page_view' and created_at > NOW() - INTERVAL '30 days'`)
	if err := row.Scan(&count); err != nil {
		return count, err
	}

	return count, nil
}

func (r *repository) TrackDeveloperProfileView(dev Developer) error {
	stmt := `INSERT INTO developer_profile_event (event_type, developer_profile_id, created_at) VALUES ($1, $2, NOW())`
	_, err := r.db.Exec(stmt, developerProfileEventPageView, dev.ID)
	return err
}

func (r *repository) TrackDeveloperProfileMessageSent(dev Developer) error {
	stmt := `INSERT INTO developer_profile_event (event_type, developer_profile_id, created_at) VALUES ($1, $2, NOW())`
	_, err := r.db.Exec(stmt, developerProfileEventMessageSent, dev.ID)
	return err
}
