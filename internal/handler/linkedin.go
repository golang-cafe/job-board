package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/linkedin"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
)

// TriggerLinkedInShareJobs posts the newest approved jobs to LinkedIn as
// article shares, mirroring TriggerTelegramScheduler and
// TriggerTwitterScheduler. It is triggered externally (e.g. by a cron job)
// with a POST to /x/task/linkedin-share-jobs authenticated with the
// x-machine-token header.
//
// Jobs are picked up from where the previous run left off, tracked by the
// last_linkedin_job_id key in the meta table, and shared in ascending id
// order. Each share links back to the job page and carries the same
// title/company/location/salary text as the Telegram and Twitter shares.
//
// LinkedIn is configured with environment variables (see README):
//   - LINKEDIN_ACCESS_TOKEN: access token with the w_member_social scope.
//   - LINKEDIN_AUTHOR_URN: author URN, e.g. urn:li:organization:1234567 or
//     urn:li:person:8675309. Falls back to LINKEDIN_PAGE_URL.
//   - LINKEDIN_PAGE_URL: LinkedIn company page or profile URL, e.g.
//     https://www.linkedin.com/company/1234567/.
//   - LINKEDIN_JOBS_TO_POST: max jobs per run (defaults to
//     TWITTER_JOBS_TO_POST).
//
// When no token or author is configured the task logs a message and posts
// nothing, so the endpoint is safe to trigger before LinkedIn is set up.
func TriggerLinkedInShareJobs(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				cfg := svr.GetConfig()
				if cfg.LinkedinAccessToken == "" {
					log.Println("linkedin share skipped: LINKEDIN_ACCESS_TOKEN is not set")
					return
				}
				authorURN, err := linkedin.ResolveAuthorURN(cfg.LinkedinPageURL, cfg.LinkedinAuthorURN)
				if err != nil {
					svr.Log(err, "linkedin share skipped: unable to resolve linkedin author")
					return
				}
				lastLinkedInJobIDStr, err := jobRepo.GetValue("last_linkedin_job_id")
				lastLinkedInJobID := 0
				if err != nil {
					if err != sql.ErrNoRows {
						svr.Log(err, "unable to retrieve last linkedin job id")
						return
					}
					// No cursor yet: start from the beginning.
					log.Println("no last linkedin job id found, starting from the beginning")
				} else if lastLinkedInJobID, err = strconv.Atoi(lastLinkedInJobIDStr); err != nil {
					svr.Log(err, "unable to convert linkedin job str to id")
					return
				}
				jobPosts, err := jobRepo.GetLastNJobsFromID(cfg.LinkedinJobsToPost, lastLinkedInJobID)
				if err != nil {
					svr.Log(err, "unable to retrieve jobs to post on linkedin")
					return
				}
				log.Printf("found %d/%d jobs to post on linkedin\n", len(jobPosts), cfg.LinkedinJobsToPost)
				if len(jobPosts) == 0 {
					return
				}
				lastJobID := lastLinkedInJobID
				client := linkedin.NewClient(cfg.LinkedinAccessToken, authorURN)
				ctx := context.Background()
				for _, j := range jobPosts {
					jobURL := fmt.Sprintf("%s%s/job/%s", cfg.URLProtocol, cfg.SiteHost, j.Slug)
					commentary := fmt.Sprintf("%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s", j.JobTitle, j.Company, j.Location, j.SalaryRange, cfg.SiteJobCategory, cfg.SiteJobCategory, jobURL)
					description := fmt.Sprintf("%s - %s | %s", j.Company, j.Location, j.SalaryRange)
					shareID, err := client.ShareArticle(ctx, commentary, jobURL, j.JobTitle, description)
					if err != nil {
						svr.Log(err, "unable to post on linkedin")
						continue
					}
					log.Printf("posted job %d to linkedin as %s\n", j.ID, shareID)
					lastJobID = j.ID
				}
				lastJobIDStr := strconv.Itoa(lastJobID)
				if err := jobRepo.SetValue("last_linkedin_job_id", lastJobIDStr); err != nil {
					svr.Log(err, fmt.Sprintf("unable to save last linkedin job id to db as %s", lastJobIDStr))
					return
				}
				log.Printf("updated last linkedin job id to %s\n", lastJobIDStr)
				log.Printf("posted last %d jobs to linkedin", len(jobPosts))
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}
