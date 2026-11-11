package developer

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	MaxCVFileSizeBytes = 5 * 1024 * 1024 // 5 MB
	CVNotUploadedLabel = "CV not uploaded"
)

var (
	ErrCVEmpty          = errors.New("uploaded CV file is empty")
	ErrCVTooLarge       = errors.New("uploaded CV exceeds maximum size of 5MB")
	ErrCVInvalidFormat  = errors.New("uploaded CV must be a valid PDF document")
	ErrCVUnauthorized   = errors.New("only logged-in recruiters or admins may download developer CVs")
)

type DeveloperCVDownload struct {
	ID                 string
	DeveloperProfileID string
	CV                 []byte
	UserID             string
	DownloadedAt       time.Time
}

func ValidateCVBytes(data []byte) error {
	if len(data) == 0 {
		return ErrCVEmpty
	}
	if len(data) > MaxCVFileSizeBytes {
		return ErrCVTooLarge
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return ErrCVInvalidFormat
	}
	return nil
}

func FormatCVStatusLabel(cv []byte) string {
	if len(cv) == 0 {
		return CVNotUploadedLabel
	}
	return fmt.Sprintf("CV uploaded (%d KB)", (len(cv)+1023)/1024)
}

func AuthorizeCVDownload(isLoggedIn bool, isRecruiter bool, isAdmin bool) error {
	if !isLoggedIn || (!isRecruiter && !isAdmin) {
		return ErrCVUnauthorized
	}
	return nil
}

func UpdateDeveloperCV(db *sql.DB, profileID string, cvBytes []byte, filename string) error {
	if err := ValidateCVBytes(cvBytes); err != nil {
		return err
	}
	_, err := db.Exec(
		`UPDATE developer_profile SET cv = $1, cv_filename = $2, cv_uploaded_at = NOW(), updated_at = NOW() WHERE id = $3`,
		cvBytes,
		filename,
		profileID,
	)
	return err
}

func RecordAndFetchDeveloperCVDownload(
	db *sql.DB,
	downloadID string,
	profileID string,
	userID string,
	isLoggedIn bool,
	isRecruiter bool,
	isAdmin bool,
) ([]byte, error) {
	if err := AuthorizeCVDownload(isLoggedIn, isRecruiter, isAdmin); err != nil {
		return nil, err
	}
	var cvBytes []byte
	err := db.QueryRow(`SELECT cv FROM developer_profile WHERE id = $1`, profileID).Scan(&cvBytes)
	if err != nil {
		return nil, err
	}
	if len(cvBytes) == 0 {
		return nil, ErrCVEmpty
	}
	_, err = db.Exec(
		`INSERT INTO developer_cv_download (id, developer_profile_id, cv, user_id, downloaded_at) VALUES ($1, $2, $3, $4, NOW())`,
		downloadID,
		profileID,
		cvBytes,
		userID,
	)
	if err != nil {
		return nil, err
	}
	return cvBytes, nil
}

func ServeDeveloperCVHTTP(w http.ResponseWriter, profileSlug string, cvBytes []byte) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-cv.pdf"`, profileSlug))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cvBytes)
}
