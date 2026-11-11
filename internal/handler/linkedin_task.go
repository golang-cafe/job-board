package handler

import (
	"encoding/json"
	"net/http"

	"github.com/golang-cafe/job-board/internal/linkedin"
)

// TriggerLinkedInShareJobsTask handles POST /x/task/linkedin-share-jobs (#91)
func TriggerLinkedInShareJobsTask(
	client *linkedin.Client,
	fetchPendingJobs func(limit int) ([]linkedin.JobPostItem, error),
	markJobShared func(jobID int, postURN string) error,
	maxJobs int,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobs, err := fetchPendingJobs(maxJobs)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sharedIDs := make([]int, 0, len(jobs))
		for _, j := range jobs {
			urn, err := client.ShareJobPost(j)
			if err != nil {
				continue
			}
			_ = markJobShared(j.ID, urn)
			sharedIDs = append(sharedIDs, j.ID)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"route":       "/x/task/linkedin-share-jobs",
			"shared_jobs": sharedIDs,
			"count":       len(sharedIDs),
		})
	}
}
