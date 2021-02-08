package main

import (
	"fmt"
	"log"
	"time"

	"github.com/0x13a/golang.cafe/pkg/config"
	"github.com/0x13a/golang.cafe/pkg/database"
	"github.com/0x13a/golang.cafe/pkg/email"
)

func main() {
	log.Println("cleaning up expired sponsored ads")
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("unable to load config %v", err)
	}
	conn, err := database.GetDbConn(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("unable to connect to postgres: %v", err)
	}
	defer database.CloseDbConn(conn)
	emailClient, err := email.NewClient(cfg.EmailAPIKey)
	if err != nil {
		log.Fatalf("unable to connect to sparkpost API: %v", err)
	}
	log.Printf("attempting to demote expired sponsored 30days pinned job ads\n")
	jobs, err := database.GetJobsOlderThan(conn, time.Now().AddDate(0, 0, -30), database.JobAdSponsoredPinnedFor30Days)
	if err != nil {
		log.Fatalf("unable to demote expired sponsored 30days pinned job ads %v", err)
	}
	for _, j := range jobs {
		jobToken, err := database.TokenByJobID(conn, j.ID)
		if err != nil {
			log.Fatalf("unable to retrieve token for job id %d for email %s: %v", j.ID, j.CompanyEmail, err)
		} else {
			err = emailClient.SendEmail("Diego from Golang Cafe <team@golang.cafe>", j.CompanyEmail, email.GolangCafeEmailAddress, "Your Job Ad on Golang Cafe Has Expired", fmt.Sprintf("Your Premium Job Ad has expired and it's no longer pinned to the front-page. If you want to keep your Job Ad on the front-page you can upgrade in a few clicks on the Job Edit Page by following this link https://golang.cafe/edit/%s?expired=1", jobToken))
			if err != nil {
				log.Fatalf("unable to send email while updating job ad type for job id %d: %v", j.ID, err)
			}
		}
		database.UpdateJobAdType(conn, database.JobAdBasic, j.ID)
		log.Printf("demoted job id %d expired sponsored 30days pinned job ads\n", j.ID)
	}

	log.Printf("attempting to demote expired sponsored 7days pinned job ads\n")
	jobs2, err := database.GetJobsOlderThan(conn, time.Now().AddDate(0, 0, -7), database.JobAdSponsoredPinnedFor7Days)
	if err != nil {
		log.Fatalf("unable to demote expired sponsored 7days pinned job ads %v", err)
	}
	for _, j := range jobs2 {
		jobToken, err := database.TokenByJobID(conn, j.ID)
		if err != nil {
			log.Fatalf("unable to retrieve token for job id %d for email %s: %v", j.ID, j.CompanyEmail, err)
		} else {
			err = emailClient.SendEmail("Diego from Golang Cafe <team@golang.cafe>", j.CompanyEmail, email.GolangCafeEmailAddress, "Your Job Ad on Golang Cafe Has Expired", fmt.Sprintf("Your Premium Job Ad has expired and it's no longer pinned to the front-page. If you want to keep your Job Ad on the front-page you can upgrade in a few clicks on the Job Edit Page by following this link https://golang.cafe/edit/%s?expired=1", jobToken))
			if err != nil {
				log.Fatalf("unable to send email while updating job ad type for job id %d: %v", j.ID, err)
			}
		}
		database.UpdateJobAdType(conn, database.JobAdBasic, j.ID)
		log.Printf("demoted job id %d expired sponsored 7days pinned job ads\n", j.ID)
	}

	log.Printf("also cleaning up expired apply tokens")
}
