package handler

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/0x13a/golang.cafe/pkg/database"
	"github.com/0x13a/golang.cafe/pkg/middleware"
	"github.com/0x13a/golang.cafe/pkg/obfuscator"
	"github.com/0x13a/golang.cafe/pkg/server"
	"github.com/gorilla/mux"
)

func IndexPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		location := r.URL.Query().Get("l")
		tag := r.URL.Query().Get("t")
		page := r.URL.Query().Get("p")

		var dst string
		if location != "" && tag != "" {
			dst = fmt.Sprintf("/Golang-%s-Jobs-In-%s", tag, location)
		} else if location != "" {
			dst = fmt.Sprintf("/Golang-Jobs-In-%s", location)
		} else if tag != "" {
			dst = fmt.Sprintf("/Golang-%s-Jobs", tag)
		}
		if dst != "" && page != "" {
			dst += fmt.Sprintf("?p=%s", page)
		}
		if dst != "" {
			svr.Redirect(w, r, http.StatusMovedPermanently, dst)
		}

		svr.RenderPageForLocationAndTag(w, "", "", page, "landing.html")
	}
}

func PermanentRedirectHandler(svr server.Server, dst string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("https://golang.cafe/%s", dst))
	}
}

func PostAJobPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPostAJobForLocation(w, r, "")
	}
}

func PostAJobWithoutPaymentPageHandler(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			svr.Render(w, http.StatusOK, "post-a-job-without-payment.html", nil)
		},
	)
}

func ListJobsAsAdminPageHandler(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			loc := r.URL.Query().Get("l")
			skill := r.URL.Query().Get("s")
			page := r.URL.Query().Get("p")
			svr.RenderPageForLocationAndTag(w, loc, skill, page, "list-jobs-admin.html")
		},
	)
}

func PostAJobForLocationPageHandler(svr server.Server, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPostAJobForLocation(w, r, location)
	}
}

func PostAJobForLocationFromURLPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		location := vars["location"]
		location = strings.ReplaceAll(location, "-", " ")
		reg, err := regexp.Compile("[^a-zA-Z0-9\\s]+")
		if err != nil {
			log.Fatal(err)
		}
		location = reg.ReplaceAllString(location, "")
		svr.RenderPostAJobForLocation(w, r, location)
	}
}

func JobByTimestampIDPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		rawID := vars["id"]
		jobID, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to parse job id %s", rawID))
			svr.JSON(w, http.StatusOK, fmt.Sprintf("Job golang.cafe/j/%s not found", rawID))
			return
		}
		job, err := database.JobPostByURLID(svr.Conn, jobID)
		if err != nil || job == nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job %s", rawID))
			svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job golang.cafe/j/%s not found", rawID))
			return
		}
		svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("https://golang.cafe/job/%s", job.Slug))
	}
}

func JobBySlugPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		slug := vars["slug"]
		location := vars["l"]
		job, err := database.JobPostBySlug(svr.Conn, slug)
		if err != nil || job == nil {
			svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job golang.cafe/job/%s not found", slug))
			return
		}
		if err := database.TrackJobView(svr.Conn, job); err != nil {
			svr.Log(err, fmt.Sprintf("unable to track job view for %s: %v", slug, err))
		}
		encryptedID, err := obfuscator.ObfuscateInt(job.ID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to generate encrypted id for %s: %v", slug, err))
		}
		jobLocations := strings.Split(job.Location, "/")
		svr.Render(w, http.StatusOK, "job.html", map[string]interface{}{
			"Job":                     job,
			"HTMLJobDescription":      svr.MarkdownToHTML(job.JobDescription),
			"HTMLJobPerks":            svr.MarkdownToHTML(job.Perks),
			"HTMLJobInterviewProcess": svr.MarkdownToHTML(job.InterviewProcess),
			"LocationFilter":          location,
			"ExternalJobId":           encryptedID,
			"GoogleJobCreatedAt":      time.Unix(job.CreatedAt, 0).Format(time.RFC3339),
			"GoogleJobValidThrough":   time.Unix(job.CreatedAt, 0).AddDate(0, 5, 0),
			"GoogleJobLocation":       jobLocations[0],
			"GoogleJobDescription":    strconv.Quote(strings.ReplaceAll(string(svr.MarkdownToHTML(job.JobDescription)), "\n", "")),
		})
	}
}

func LandingPageForLocationHandler(svr server.Server, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, location, "", page, "landing.html")
	}
}

func LandingPageForLocationAndSkillPlaceholderHandler(svr server.Server, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, location, skill, page, "landing.html")
	}
}

func LandingPageForLocationPlaceholderHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		loc := strings.ReplaceAll(vars["location"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, loc, "", page, "landing.html")
	}
}

func LandingPageForSkillPlaceholderHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, "", skill, page, "landing.html")
	}
}

func LandingPageForSkillAndLocationPlaceholderHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		loc := strings.ReplaceAll(vars["location"], "-", " ")
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, loc, skill, page, "landing.html")
	}
}
