package main

import (
	"fmt"
	"log"
	"strconv"

	"github.com/0x13a/golang.cafe/pkg/database"
	"github.com/turnage/graw/reddit"
)

const LastRedditJobIDKey = `last_reddit_job_id`
const SubReddit = `golangjobofferings`
const JobsToPost = 2

func main() {
	fmt.Printf("running reddit script to post last %d jobs\n", JobsToPost)
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("unable to load config %v", err)
	}
	redditCfg := reddit.BotConfig{
		Agent: "alpine:github.com/golang-cafe/golang.cafe:0.1 (by /u/hidiegomariani)",
		App: reddit.App{
			ID:       cfg.AppID,
			Secret:   cfg.AppKey,
			Username: cfg.Username,
			Password: cfg.Password,
		},
	}
	bot, err := reddit.NewBot(redditCfg)
	if err != nil {
		log.Fatalf("unable to start reddit agent %v", err)
	}
	fmt.Printf("initialised reddit client\n")
	conn, err := database.GetDbConn(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("unable to connect to postgres: %v", err)
	}
	lastRedditJobIDStr, err := database.GetValue(conn, LastRedditJobIDKey)
	if err != nil {
		log.Fatalf("unable to retrieve last reddit job id %v", err)
	}
	lastRedditJobID, err := strconv.Atoi(lastRedditJobIDStr)
	if err != nil {
		log.Fatalf("unable to convert job str to id %s", lastRedditJobIDStr)
	}
	jobs, err := database.GetLastNJobsFromID(conn, JobsToPost, lastRedditJobID)
	fmt.Printf("found %d/%d jobs to post on reddit /r/%s\n", len(jobs), JobsToPost, SubReddit)
	var lastJobID int
	for _, j := range jobs {
		err = bot.PostLink(SubReddit, fmt.Sprintf("%s with %s - %s | %s | Golang Cafe", j.JobTitle, j.Company, j.Location, j.SalaryRange), fmt.Sprintf("https://golang.cafe/job/%s", j.Slug))
		if err != nil {
			log.Fatalf("unable to post link to reddit got error %v", err)
		}
		fmt.Printf("%s with %s - %s | %s | Golang Cafe\n", j.JobTitle, j.Company, j.Location, j.SalaryRange)
		fmt.Printf("https://golang.cafe/job/%s\n", j.Slug)
		lastJobID = j.ID
	}
	lastJobIDStr := strconv.Itoa(lastJobID)
	err = database.SetValue(conn, LastRedditJobIDKey, lastJobIDStr)
	if err != nil {
		log.Fatalf("unable to save last reddit job id to db %v", err)
	}
	fmt.Printf("posted last %d jobs to reddit", len(jobs))
}
