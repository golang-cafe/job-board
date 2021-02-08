package main

import (
	"fmt"
	"log"
	"net/url"
	"strconv"

	"github.com/0x13a/golang.cafe/pkg/database"
	"github.com/ChimeraCoder/anaconda"
)

const LastTwittedJobIDKey = `last_twitted_job_id`

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("unable to load config: %+v", err)
	}
	fmt.Printf("running twitter script to post last %d jobs\n", cfg.JobsToPost)
	conn, err := database.GetDbConn(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("unable to connect to postgres: %v", err)
	}
	lastTwittedJobIDStr, err := database.GetValue(conn, LastTwittedJobIDKey)
	if err != nil {
		log.Fatalf("unable to retrieve last twitted job id %v", err)
	}
	lastTwittedJobID, err := strconv.Atoi(lastTwittedJobIDStr)
	if err != nil {
		log.Fatalf("unable to convert job str to id %s", lastTwittedJobIDStr)
	}
	jobs, err := database.GetLastNJobsFromID(conn, cfg.JobsToPost, lastTwittedJobID)
	fmt.Printf("found %d/%d jobs to post on twitter\n", len(jobs), cfg.JobsToPost)
	if len(jobs) == 0 {
		return
	}
	lastJobID := lastTwittedJobID
	api := anaconda.NewTwitterApiWithCredentials(cfg.AccessToken, cfg.AccessTokenSecret, cfg.ClientKey, cfg.ClientSecret)
	fmt.Printf("initialised twitter client\n")
	for _, j := range jobs {
		_, err := api.PostTweet(fmt.Sprintf("%s with %s - %s | %s\n\n#golang #golangjobs\n\nhttps://golang.cafe/job/%s", j.JobTitle, j.Company, j.Location, j.SalaryRange, j.Slug), url.Values{})
		if err != nil {
			// TODO: add some warning to email/sentry
			log.Fatalf("unable to post tweet got error %v", err)
		}
		fmt.Printf(fmt.Sprintf("%s with %s - %s | %s\n\n#golang #golangjobs\n\nhttps://golang.cafe/job/%s\n", j.JobTitle, j.Company, j.Location, j.SalaryRange, j.Slug))
		lastJobID = j.ID
	}
	lastJobIDStr := strconv.Itoa(lastJobID)
	err = database.SetValue(conn, LastTwittedJobIDKey, lastJobIDStr)
	if err != nil {
		log.Fatalf("unable to save last twitted job id to db %v", err)
	}
	fmt.Printf("updated last twitted job id to %s\n", lastJobIDStr)
	fmt.Printf("posted last %d jobs to twitter", len(jobs))
}
