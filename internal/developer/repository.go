package developer

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/gosimple/slug"
)

const (
	developerProfileEventPageView    = "developer_profile_page_view"
	developerProfileEventMessageSent = "developer_profile_message_sent"
	SearchTypeDeveloper              = "developer"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db}
}

func (r *Repository) DeveloperProfileBySlug(slug string) (Developer, error) {
	row := r.db.QueryRow(`SELECT id, email, location, available, linkedin_url, hourly_rate, image_id, slug, created_at, updated_at, skills, name, bio, github_url, twitter_url, search_status, role_level, role_types FROM developer_profile WHERE slug = $1`, slug)
	dev := Developer{}
	var roleTypes string
	err := row.Scan(
		&dev.ID,
		&dev.Email,
		&dev.Location,
		&dev.Available,
		&dev.LinkedinURL,
		&dev.HourlyRate,
		&dev.ImageID,
		&dev.Slug,
		&dev.CreatedAt,
		&dev.UpdatedAt,
		&dev.Skills,
		&dev.Name,
		&dev.Bio,
		&dev.GithubURL,
		&dev.TwitterURL,
		&dev.SearchStatus,
		&dev.RoleLevel,
		&roleTypes,
	)
	dev.RoleTypes = strings.Split(roleTypes, ",")
	if err != nil {
		return dev, err
	}

	return dev, nil
}

func (r *Repository) DeveloperProfileByEmail(email string) (Developer, error) {
	row := r.db.QueryRow(`SELECT id, email, location, available, linkedin_url, hourly_rate, image_id, slug, created_at, updated_at, skills, name, bio FROM developer_profile WHERE lower(email) = lower($1)`, email)
	dev := Developer{}
	var nullUpdatedAt sql.NullTime
	err := row.Scan(
		&dev.ID,
		&dev.Email,
		&dev.Location,
		&dev.Available,
		&dev.LinkedinURL,
		&dev.HourlyRate,
		&dev.ImageID,
		&dev.Slug,
		&dev.CreatedAt,
		&nullUpdatedAt,
		&dev.Skills,
		&dev.Name,
		&dev.Bio,
	)
	if nullUpdatedAt.Valid {
		dev.UpdatedAt = nullUpdatedAt.Time
	} else {
		dev.UpdatedAt = dev.CreatedAt
	}
	if err == sql.ErrNoRows {
		return dev, nil
	}
	if err != nil {
		return dev, err
	}

	return dev, nil
}

func (r *Repository) DeveloperProfileByID(id string) (Developer, error) {
	row := r.db.QueryRow(`SELECT id, email, location, linkedin_url, hourly_rate, image_id, slug, created_at, updated_at, skills, name, bio, search_status, role_level FROM developer_profile WHERE id = $1`, id)
	dev := Developer{}
	var nullTime sql.NullTime
	err := row.Scan(
		&dev.ID,
		&dev.Email,
		&dev.Location,
		&dev.LinkedinURL,
		&dev.HourlyRate,
		&dev.ImageID,
		&dev.Slug,
		&dev.CreatedAt,
		&nullTime,
		&dev.Skills,
		&dev.Name,
		&dev.Bio,
		&dev.SearchStatus,
		&dev.RoleLevel,
	)
	if nullTime.Valid {
		dev.UpdatedAt = nullTime.Time
	}
	if err != nil {
		return dev, err
	}

	return dev, nil
}

func (r *Repository) SendMessageDeveloperProfile(message DeveloperMessage, senderID string) error {
	_, err := r.db.Exec(
		`INSERT INTO developer_profile_message (id, email, content, profile_id, created_at, sender_id) VALUES ($1, $2, $3, $4, NOW(), $5)`,
		message.ID,
		message.Email,
		message.Content,
		message.ProfileID,
		senderID,
	)
	return err
}

func (r *Repository) MessageForDeliveryByID(id string) (DeveloperMessage, string, error) {
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

func (r *Repository) MarkDeveloperMessageAsSent(id string) error {
	_, err := r.db.Exec(`UPDATE developer_profile_message SET sent_at = NOW() WHERE id = $1`, id)
	return err
}

func (r *Repository) DevelopersByLocationAndTag(loc, tag string, pageID, pageSize int) ([]Developer, int, error) {
	var rows *sql.Rows
	var err error
	offset := pageID*pageSize - pageSize
	var developers []Developer
	switch {
	case tag != "" && loc != "":
		rows, err = r.db.Query(`SELECT count(*) OVER() AS full_count, id, email, location, available, linkedin_url, hourly_rate, image_id, slug, created_at, updated_at, skills, name, bio, github_url, twitter_url, search_status, role_level, role_types FROM developer_profile WHERE location ILIKE '%' || $1 || '%' AND skills ILIKE '%' || $2 || '%' AND created_at != updated_at ORDER BY updated_at DESC LIMIT $3 OFFSET $4`, loc, tag, pageSize, offset)
	case tag != "" && loc == "":
		rows, err = r.db.Query(`SELECT count(*) OVER() AS full_count, id, email, location, available, linkedin_url, hourly_rate, image_id, slug, created_at, updated_at, skills, name, bio, github_url, twitter_url, search_status, role_level, role_types FROM developer_profile WHERE skills ILIKE '%' || $1 || '%' AND created_at != updated_at ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, tag, pageSize, offset)
	case tag == "" && loc != "":
		rows, err = r.db.Query(`SELECT count(*) OVER() AS full_count, id, email, location, available, linkedin_url, hourly_rate, image_id, slug, created_at, updated_at, skills, name, bio, github_url, twitter_url, search_status, role_level, role_types FROM developer_profile WHERE location ILIKE '%' || $1 || '%' AND created_at != updated_at ORDER BY updated_at DESC LIMIT $2 OFFSET $3`, loc, pageSize, offset)
	default:
		rows, err = r.db.Query(`SELECT count(*) OVER() AS full_count, id, email, location, available, linkedin_url, hourly_rate, image_id, slug, created_at, updated_at, skills, name, bio, github_url, twitter_url, search_status, role_level, role_types FROM developer_profile WHERE created_at != updated_at ORDER BY updated_at DESC LIMIT $1 OFFSET $2`, pageSize, offset)
	}
	if err == sql.ErrNoRows {
		return developers, 0, nil
	}
	var fullRowsCount int
	for rows.Next() {
		var dev Developer
		var roleTypes string
		err := rows.Scan(
			&fullRowsCount,
			&dev.ID,
			&dev.Email,
			&dev.Location,
			&dev.Available,
			&dev.LinkedinURL,
			&dev.HourlyRate,
			&dev.ImageID,
			&dev.Slug,
			&dev.CreatedAt,
			&dev.UpdatedAt,
			&dev.Skills,
			&dev.Name,
			&dev.Bio,
			&dev.GithubURL,
			&dev.TwitterURL,
			&dev.SearchStatus,
			&dev.RoleLevel,
			&roleTypes,
		)
		fmt.Printf("found dev %s and hourly rate %d\n", dev.ID, dev.HourlyRate)
		dev.RoleTypes = strings.Split(roleTypes, ",")
		if err != nil {
			return developers, fullRowsCount, err
		}
		developers = append(developers, dev)
	}

	return developers, fullRowsCount, nil
}

func (r *Repository) UpdateDeveloperProfile(dev Developer) error {
	_, err := r.db.Exec(`UPDATE developer_profile SET name = $1, location = $2, linkedin_url = $3, hourly_rate = $4, bio = $5, available = $6, image_id = $7, updated_at = NOW(), skills = $8, search_status = $9, role_level = $10  WHERE id = $11`, dev.Name, dev.Location, dev.LinkedinURL, dev.HourlyRate, dev.Bio, dev.Available, dev.ImageID, dev.Skills, dev.SearchStatus, dev.RoleLevel, dev.ID)
	return err
}

func (r *Repository) DeleteDeveloperProfile(id, email string) error {
	_, err := r.db.Exec(`DELETE FROM developer_profile WHERE id = $1 AND email = $2`, id, email)
	return err
}

func (r *Repository) ActivateDeveloperProfile(email string) error {
	_, err := r.db.Exec(`UPDATE developer_profile SET updated_at = NOW() WHERE email = $1`, email)
	return err
}

func (r *Repository) SaveDeveloperProfile(dev Developer) error {
	dev.Slug = slug.Make(fmt.Sprintf("%s %d", dev.Name, time.Now().UTC().Unix()))
	_, err := r.db.Exec(
		`INSERT INTO developer_profile (email, location, linkedin_url, hourly_rate, bio, available, image_id, slug, created_at, updated_at, skills, name, id, github_url, twitter_url, role_types, role_level, search_status, detected_location_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW(), $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		dev.Email,
		dev.Location,
		dev.LinkedinURL,
		dev.HourlyRate,
		dev.Bio,
		dev.Available,
		dev.ImageID,
		dev.Slug,
		dev.Skills,
		dev.Name,
		dev.ID,
		dev.GithubURL,
		dev.TwitterURL,
		strings.Join(dev.RoleTypes, ","),
		dev.RoleLevel,
		dev.SearchStatus,
		dev.DetectedLocationID,
	)
	return err
}

func (r *Repository) GetTopDevelopers(limit int) ([]Developer, error) {
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

func (r *Repository) GetTopDeveloperSkills(limit int) ([]string, error) {
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

func (r *Repository) GetDeveloperSkills() ([]string, error) {
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

func (r *Repository) GetDeveloperSlugs() ([]string, error) {
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

func (r *Repository) GetLastDevUpdatedAt() (time.Time, error) {
	var updatedAt time.Time
	row := r.db.QueryRow(`SELECT updated_at FROM developer_profile WHERE updated_at != created_at ORDER BY updated_at DESC LIMIT 1`)
	if err := row.Scan(&updatedAt); err != nil {
		return updatedAt, err
	}

	return updatedAt, nil
}

func (r *Repository) GetDevelopersRegisteredLastMonth() (int, error) {
	var count int
	row := r.db.QueryRow(`select count(*) from developer_profile where created_at > NOW() - INTERVAL '30 days'`)
	if err := row.Scan(&count); err != nil {
		return count, err
	}

	return count, nil
}

func (r *Repository) GetDeveloperMessagesSentLastMonth() (int, error) {
	var count int
	row := r.db.QueryRow(`select count(*) from developer_profile_message where created_at > NOW() - INTERVAL '30 days'`)
	if err := row.Scan(&count); err != nil {
		return count, err
	}

	return count, nil
}

func (r *Repository) GetDeveloperProfilePageViewsLastMonth() (int, error) {
	var count int
	row := r.db.QueryRow(`select count(*) as c from developer_profile_event where event_type = 'developer_profile_page_view' and created_at > NOW() - INTERVAL '30 days'`)
	if err := row.Scan(&count); err != nil {
		return count, err
	}

	return count, nil
}

func (r *Repository) TrackDeveloperProfileView(dev Developer) error {
	stmt := `INSERT INTO developer_profile_event (event_type, developer_profile_id, created_at) VALUES ($1, $2, NOW())`
	_, err := r.db.Exec(stmt, developerProfileEventPageView, dev.ID)
	return err
}

func (r *Repository) TrackDeveloperProfileMessageSent(dev Developer) error {
	stmt := `INSERT INTO developer_profile_event (event_type, developer_profile_id, created_at) VALUES ($1, $2, NOW())`
	_, err := r.db.Exec(stmt, developerProfileEventMessageSent, dev.ID)
	return err
}
