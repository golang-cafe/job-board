package handler

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"net/http"

	"github.com/golang-cafe/job-board/internal/database"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/profilephoto"
	"github.com/golang-cafe/job-board/internal/server"
)

func decodeProfileImage(data []byte) (image.Image, error) {
	c, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if c.Width <= 0 || c.Height <= 0 || c.Width > 4096 || c.Height > 4096 || int64(c.Width)*int64(c.Height) > 16000000 {
		return nil, errors.New("image dimensions exceed the supported limit")
	}
	im, _, err := image.Decode(bytes.NewReader(data))
	return im, err
}

func validateDeveloperMedia(svr server.Server, id string) error {
	if id == "" {
		return errors.New("profile image is required")
	}
	m, err := database.GetMediaByID(svr.Conn, id)
	if err != nil {
		return err
	}
	im, err := decodeProfileImage(m.Bytes)
	if err != nil {
		return err
	}
	if err = profilephoto.Validate(im); err != nil {
		return err
	}
	return database.ProtectDeveloperMedia(svr.Conn, id)
}

func privateDeveloperMedia(svr server.Server, w http.ResponseWriter, r *http.Request, id string, m database.Media) (database.Media, error) {
	protected, err := database.IsDeveloperMedia(svr.Conn, id)
	if err != nil {
		return database.Media{}, err
	}
	if !protected {
		return m, nil
	}
	// Both variants must bypass shared/browser caches, including legacy URL sizes.
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Add("Vary", "Cookie")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
	if err == nil && profile != nil {
		return m, nil
	}
	im, err := decodeProfileImage(m.Bytes)
	if err != nil {
		return database.Media{}, err
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, profilephoto.Blur(im)); err != nil {
		return database.Media{}, err
	}
	return database.Media{Bytes: encoded.Bytes(), MediaType: "image/png"}, nil
}
