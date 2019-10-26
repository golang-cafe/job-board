package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/0x13a/golang.cafe/pkg/config"
	"github.com/0x13a/golang.cafe/pkg/database"
)

const LastSentJobIDDailyKey = `last_sent_job_id_daily`
const LastSentJobIDWeeklyKey = `last_sent_job_id_weekly`
const MailerliteDailySegmentID = 326154
const MailerliteWeeklySegmentID = 326156

func main() {
	var frequency string
	flag.StringVar(&frequency, "frequency", "", "frequency which has to be used for newsletter")
	flag.Parse()
	jobsToSendStr := os.Getenv("NEWSLETTER_JOBS_TO_SEND")
	jobsToSend, err := strconv.Atoi(jobsToSendStr)
	if err != nil {
		log.Fatalf("unable to convert NEWSLETTER_JOBS_TO_SEND to int %v", err)
	}
	fmt.Printf("running newsletter script send %s jobs updates with %d jobs\n", frequency, jobsToSend)
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("unable to load config %v", err)
	}
	conn, err := database.GetDbConn(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("unable to connect to postgres: %v", err)
	}
	if frequency == "daily" {
		processDaily(conn, jobsToSend, cfg)
		return
	}
	if frequency == "weekly" {
		processWeekly(conn, jobsToSend, cfg)
		return
	}
	log.Fatalf("frequency is not supported %v", frequency)
}

func processDaily(conn *sql.DB, jobsToSend int, cfg config.Config) {
	lastJobIDStr, err := database.GetValue(conn, LastSentJobIDDailyKey)
	if err != nil {
		log.Fatalf("unable to retrieve last newsletter daily job id %v", err)
	}
	lastJobID, err := strconv.Atoi(lastJobIDStr)
	if err != nil {
		log.Fatalf("unable to convert job str to id %s", lastJobIDStr)
	}
	jobs, err := database.GetLastNJobsFromID(conn, jobsToSend, lastJobID)
	if len(jobs) < 1 {
		log.Printf("found 0 new jobs for daily newsletter. quitting")
		return
	}
	log.Printf("found %d/%d jobs for daily newsletter\n", len(jobs), jobsToSend)
	// create campaign
	jsonMailerliteRq := []byte(fmt.Sprintf(`{
		"segments": [%d],
		"type": "regular",
		"subject": "Golang Cafe Daily Job Alert",
		"from": "team@golang.cafe",
		"from_name": "Golang Cafe"
		}`, MailerliteDailySegmentID))
	client := &http.Client{}
	req, err := http.NewRequest("POST", "https://api.mailerlite.com/api/v2/campaigns", bytes.NewBuffer(jsonMailerliteRq))
	if err != nil {
		log.Printf("unable to create req for mailerlite %v: %v", jsonMailerliteRq, err)
		return
	}
	req.Header.Add("X-MailerLite-ApiKey", cfg.MailerLiteAPIKey)
	req.Header.Add("content-type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		log.Printf("unable to create campaign %s on mailerlite %v", jsonMailerliteRq, err)
		return
	}
	var campaignResponse struct {
		ID    int             `json:"id"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&campaignResponse); err != nil {
		log.Printf("unable to read json response campaign id from mailerlite api %v", err)
	}
	if res.StatusCode != http.StatusOK {
		log.Printf("unable to create daily campaign got status code %d and res %s - response %s", res.StatusCode, jsonMailerliteRq, campaignResponse.Error)
		return
	}
	res.Body.Close()
	log.Printf("created campaign for daily golang job alert with ID %d", campaignResponse.ID)
	// update campaign content
	var jobsHTMLArr []string
	var jobsTXTArr []string
	for _, j := range jobs {
		jobsHTMLArr = append(jobsHTMLArr, fmt.Sprintf("<p>%s with %s - %s | %s<br />https://golang.cafe/job/%s</p>", j.JobTitle, j.Company, j.Location, j.SalaryRange, j.Slug))
		jobsTXTArr = append(jobsTXTArr, fmt.Sprintf("%s with %s - %s | %s\nhttps://golang.cafe/job/%s\n", j.JobTitle, j.Company, j.Location, j.SalaryRange, j.Slug))
		lastJobID = j.ID
	}
	jobsTXT := strings.Join(jobsTXTArr, "\n")
	jobsHTML := strings.Join(jobsHTMLArr, " ")
	campaignContentHTML := `<p>Hey!</p>
	<p>It's been a busy day and we have got new Go (Golang) jobs being posted,</p>
	<p>Have a look at the latest Go jobs</p>
	` + jobsHTML + `
	<p>Check out more jobs at <a title="Golang Cafe" href="https://golang.cafe">https://golang.cafe</a></p>
	<p>Always Keep Coding!</p>
	<hr />
	<h6><strong> Golang Cafe</strong> | Bethnal Green Road, London<br />This email was sent to <a href="mailto:{$email}"><strong>{$email}</strong></a> | <a href="{$unsubscribe}">Unsubscribe</a> | <a href="{$forward}">Forward this email to a friend</a></h6>`
	campaignContentTxt := `Hey!
	It's been a busy day and we have got new Go (Golang) jobs being posted,
	Have a look at the latest Go jobs
	` + jobsTXT + `
	Check out more jobs at https://golang.cafe
	Always Keep Coding!
	
	Golang Cafe
	
	Unsubscribe here {$unsubscribe} | Golang Cafe Newsletter {$url}`
	updateCampaignRq := []byte(fmt.Sprintf(`{"html": %s, "plain": %s}`, jsonEscape(campaignContentHTML), jsonEscape(campaignContentTxt)))
	req, err = http.NewRequest("PUT", fmt.Sprintf("https://api.mailerlite.com/api/v2/campaigns/%d/content", campaignResponse.ID), bytes.NewBuffer(updateCampaignRq))
	if err != nil {
		log.Printf("unable to create req for mailerlite %v: %v", jsonMailerliteRq, err)
		return
	}
	req.Header.Add("X-MailerLite-ApiKey", cfg.MailerLiteAPIKey)
	req.Header.Add("content-type", "application/json")
	res, err = client.Do(req)
	if err != nil {
		log.Printf("unable to create daily campaign %s on mailerlite %v", jsonMailerliteRq, err)
		return
	}
	var campaignUpdateRes struct {
		OK    bool            `json:"success"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&campaignUpdateRes); err != nil {
		log.Printf("unable to update daily campaign content got error %v", err)
		return
	}
	if !campaignUpdateRes.OK {
		log.Printf("unable to update daily campaign content got res %s - req %s", campaignUpdateRes, updateCampaignRq)
		return
	}
	res.Body.Close()
	log.Printf("updated campaign with html content\n")
	// send campaign
	req, err = http.NewRequest("POST", fmt.Sprintf("http://api.mailerlite.com/api/v2/campaigns/%d/actions/send", campaignResponse.ID), bytes.NewBuffer([]byte{}))
	if err != nil {
		log.Printf("unable to send daily campaign id %d req for mailerlite: %v", campaignResponse.ID, err)
		return
	}
	req.Header.Add("X-MailerLite-ApiKey", cfg.MailerLiteAPIKey)
	req.Header.Add("content-type", "application/json")
	res, err = client.Do(req)
	if err != nil {
		log.Printf("unable to send daily campaign id %d on mailerlite %v", campaignResponse.ID, err)
		return
	}
	res.Body.Close()
	log.Printf("sent daily campaign with %d jobs via mailerlite api\n", len(jobsHTMLArr))
	lastJobIDStr = strconv.Itoa(lastJobID)
	err = database.SetValue(conn, LastSentJobIDDailyKey, lastJobIDStr)
	if err != nil {
		log.Fatalf("unable to save last daily newsletter job id to db %v", err)
	}
	log.Printf("updated last sent job id daily")
}

func jsonEscape(i string) string {
	r, err := json.Marshal(i)
	if err != nil {
		log.Fatalf("unable to escape json for html/txt template for campaign %v", err)
	}
	return string(r)
}

func processWeekly(conn *sql.DB, jobsToSend int, cfg config.Config) {
	lastJobIDStr, err := database.GetValue(conn, LastSentJobIDWeeklyKey)
	if err != nil {
		log.Fatalf("unable to retrieve last newsletter weekly job id %v", err)
	}
	lastJobID, err := strconv.Atoi(lastJobIDStr)
	if err != nil {
		log.Fatalf("unable to convert job str to id %s", lastJobIDStr)
	}
	jobs, err := database.GetLastNJobsFromID(conn, jobsToSend, lastJobID)
	if len(jobs) < 1 {
		log.Printf("found 0 new jobs for weekly newsletter. quitting")
		return
	}
	fmt.Printf("found %d/%d jobs for weekly newsletter\n", len(jobs), jobsToSend)
	// create campaign
	jsonMailerliteRq := []byte(fmt.Sprintf(`{
		"segments": [%d],
		"type": "regular",
		"subject": "Golang Cafe Weekly Job Alert",
		"from": "team@golang.cafe",
		"from_name": "Golang Cafe"
		}`, MailerliteWeeklySegmentID))
	client := &http.Client{}
	req, err := http.NewRequest("POST", "https://api.mailerlite.com/api/v2/campaigns", bytes.NewBuffer(jsonMailerliteRq))
	if err != nil {
		log.Printf("unable to create weekly req for mailerlite %v: %v", jsonMailerliteRq, err)
		return
	}
	req.Header.Add("X-MailerLite-ApiKey", cfg.MailerLiteAPIKey)
	req.Header.Add("content-type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		log.Printf("unable to create weekly campaign %s on mailerlite %v", jsonMailerliteRq, err)
		return
	}
	var campaignResponse struct {
		ID    int             `json:"id"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&campaignResponse); err != nil {
		log.Printf("unable to read json response weekly campaign id from mailerlite api %v", err)
	}
	res.Body.Close()
	log.Printf("created campaign for weekly golang job alert with ID %d", campaignResponse.ID)
	// update campaign content
	var jobsHTMLArr []string
	var jobsTXTArr []string
	for _, j := range jobs {
		jobsHTMLArr = append(jobsHTMLArr, fmt.Sprintf("<p>%s with %s - %s | %s<br />https://golang.cafe/job/%s</p>", j.JobTitle, j.Company, j.Location, j.SalaryRange, j.Slug))
		jobsTXTArr = append(jobsTXTArr, fmt.Sprintf("%s with %s - %s | %s\nhttps://golang.cafe/job/%s\n", j.JobTitle, j.Company, j.Location, j.SalaryRange, j.Slug))
		lastJobID = j.ID
	}
	jobsTXT := strings.Join(jobsTXTArr, "\n")
	jobsHTML := strings.Join(jobsHTMLArr, " ")
	campaignContentHTML := `<p>Hey!</p>
	<p>It's been a busy week and we have got new Go (Golang) jobs being posted,</p>
	<p>Have a look at the latest Go jobs</p>
	` + jobsHTML + `
	<p>Check out more jobs at <a title="Golang Cafe" href="https://golang.cafe">https://golang.cafe</a></p>
	<p>Always Keep Coding!</p>
	<hr />
	<h6><strong> Golang Cafe</strong> | Bethnal Green Road, London<br />This email was sent to <a href="mailto:{$email}"><strong>{$email}</strong></a> | <a href="{$unsubscribe}">Unsubscribe</a> | <a href="{$forward}">Forward this email to a friend</a></h6>`
	campaignContentTxt := `Hey!
	It's been a busy day and we have got new Go (Golang) jobs being posted,
	Have a look at the latest Go jobs
	` + jobsTXT + `
	Check out more jobs at https://golang.cafe
	Always Keep Coding!
	
	Golang Cafe
	
	Unsubscribe here {$unsubscribe} | Golang Cafe Newsletter {$url}`
	updateCampaignRq := []byte(fmt.Sprintf(`{"html": %s, "plain": %s}`, jsonEscape(campaignContentHTML), jsonEscape(campaignContentTxt)))
	req, err = http.NewRequest("PUT", fmt.Sprintf("https://api.mailerlite.com/api/v2/campaigns/%d/content", campaignResponse.ID), bytes.NewBuffer(updateCampaignRq))
	if err != nil {
		log.Printf("unable to create req for mailerlite %v: %v", jsonMailerliteRq, err)
		return
	}
	req.Header.Add("X-MailerLite-ApiKey", cfg.MailerLiteAPIKey)
	req.Header.Add("content-type", "application/json")
	res, err = client.Do(req)
	if err != nil {
		log.Printf("unable to create weekly campaign %s on mailerlite %v", jsonMailerliteRq, err)
		return
	}
	var campaignUpdateRes struct {
		OK bool `json:"success"`
	}
	if err := json.NewDecoder(res.Body).Decode(&campaignUpdateRes); err != nil {
		log.Printf("unable to update weekly campaign content got error %v", err)
		return
	}
	if !campaignUpdateRes.OK {
		log.Printf("unable to update weekly campaign content got res %s", campaignUpdateRes)
		return
	}
	res.Body.Close()
	log.Printf("updated weekly campaign with html content\n")
	// send campaign
	req, err = http.NewRequest("POST", fmt.Sprintf("http://api.mailerlite.com/api/v2/campaigns/%d/actions/send", campaignResponse.ID), bytes.NewBuffer([]byte{}))
	if err != nil {
		log.Printf("unable to send campaign id %d req for mailerlite: %v", campaignResponse.ID, err)
		return
	}
	req.Header.Add("X-MailerLite-ApiKey", cfg.MailerLiteAPIKey)
	req.Header.Add("content-type", "application/json")
	res, err = client.Do(req)
	if err != nil {
		log.Printf("unable to send weekly campaign id %d on mailerlite %v", campaignResponse.ID, err)
		return
	}
	res.Body.Close()
	log.Printf("sent weekly campaign with %d jobs via mailerlite api\n", len(jobsHTMLArr))
	lastJobIDStr = strconv.Itoa(lastJobID)
	err = database.SetValue(conn, LastSentJobIDWeeklyKey, lastJobIDStr)
	if err != nil {
		log.Fatalf("unable to save last weekly newsletter job id to db %v", err)
	}
	log.Printf("updated last sent job id weekly")
}
