package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/golang-cafe/job-board/internal/database"
	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/recruiter"
	"github.com/golang-cafe/job-board/internal/server"
)

func ReceivedMessages(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, _ := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if !profile.IsAdmin && !profile.IsDeveloper {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}

			dev, err := devRepo.DeveloperProfileByEmail(profile.Email)
			if err != nil {
				svr.Log(err, "DeveloperProfileByEmail")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}

			messages, err := devRepo.GetDeveloperMessagesSentTo(dev.ID)
			if err != nil {
				svr.Log(err, "GetDeveloperMessagesSentTo")
			}

			viewCount, err := devRepo.GetViewCountForProfile(dev.ID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve job view count for dev id %s", dev.ID))
			}
			messagesCount, err := devRepo.GetMessagesCountForJob(dev.ID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve job messages count for dev id %s", dev.ID))
			}
			stats, err := devRepo.GetStatsForProfile(dev.ID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve stats for dev id %s", dev.ID))
			}
			statsSet, err := json.Marshal(stats)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to marshal stats for dev id %s", dev.ID))
			}

			err = svr.Render(r, w, http.StatusOK, "messages.html", map[string]interface{}{
				"Messages":      messages,
				"ViewCount":     viewCount,
				"MessagesCount": messagesCount,
				"Stats":         string(statsSet),
			})
			if err != nil {
				svr.Log(err, "unable to render sent messages page")
			}
		})
}

func UploadDeveloperCVHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		maxPdfSize := int64(5 * 1024 * 1024)
		r.Body = http.MaxBytesReader(w, r.Body, maxPdfSize)
		cv, header, err := r.FormFile("cv")
		if err != nil {
			svr.Log(err, "unable to read developer cv file")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		defer cv.Close()
		fileBytes, err := ioutil.ReadAll(cv)
		if err != nil {
			svr.Log(err, "unable to read developer cv file content")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		contentType := http.DetectContentType(fileBytes)
		if contentType != "application/pdf" {
			svr.Log(errors.New("CV file is not application/pdf"), fmt.Sprintf("CV file is not application/pdf got %s", contentType))
			svr.JSON(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		if header.Size > maxPdfSize {
			svr.Log(errors.New("CV file is too large"), fmt.Sprintf("CV file too large: %d > %d", header.Size, maxPdfSize))
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		id, err := database.SaveMedia(svr.Conn, database.Media{
			Bytes:     fileBytes,
			MediaType: "application/pdf",
		})
		if err != nil {
			svr.Log(err, "unable to save developer cv to db")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.JSON(w, http.StatusOK, map[string]interface{}{"id": id})
	}
}

func DownloadDeveloperCVHandler(svr server.Server, devRepo *developer.Repository, recruiterRepo *recruiter.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			profileSlug := vars["slug"]
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil || profile == nil {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			dev, err := devRepo.DeveloperProfileBySlug(profileSlug)
			if err != nil {
				svr.Log(err, "unable to find developer profile by slug "+profileSlug)
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			if !profile.IsAdmin && !profile.IsRecruiter && profile.Email != dev.Email {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			if dev.CvID == nil || *dev.CvID == "" {
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			media, err := database.GetMediaByID(svr.Conn, *dev.CvID)
			if err != nil {
				svr.Log(err, "unable to retrieve developer cv media by ID")
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", fmt.Sprintf("%s-cv.pdf", dev.Slug)))
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(media.Bytes)))
			_, _ = w.Write(media.Bytes)
		},
	)
}
