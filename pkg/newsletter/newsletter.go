package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/0x13a/golang.cafe/pkg/config"
	"github.com/0x13a/golang.cafe/pkg/database"
)

const LastSentJobIDWeeklyKey = `last_sent_job_id_weekly`
const MailerliteWeeklySegmentID = 326156
const EveryoneGroup = 103091230

func main() {
	jobsToSendStr := os.Getenv("NEWSLETTER_JOBS_TO_SEND")
	jobsToSend, err := strconv.Atoi(jobsToSendStr)
	if err != nil {
		log.Fatalf("unable to convert NEWSLETTER_JOBS_TO_SEND to int %v", err)
	}
	fmt.Printf("running newsletter script send jobs updates with %d jobs\n", jobsToSend)
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("unable to load config %v", err)
	}
	conn, err := database.GetDbConn(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("unable to connect to postgres: %v", err)
	}
	processWeekly(conn, jobsToSend, cfg)
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
		"groups": [%d],
		"type": "regular",
		"subject": "Newest Go Jobs This Week",
		"from": "team@golang.cafe",
		"from_name": "Golang Cafe"
		}`, EveryoneGroup))
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
		jobsHTMLArr = append(jobsHTMLArr, fmt.Sprintf(`<p><b>Job Title:</b> %s<br /><b>Company:</b> %s<br /><b>Location:</b> %s<br /><b>Salary:</b> %s<br /><b>Detail:</b> <a href="https://golang.cafe/job/%s">https://golang.cafe/job/%s</a></p>`, j.JobTitle, j.Company, j.Location, j.SalaryRange, j.Slug, j.Slug))
		jobsTXTArr = append(jobsTXTArr, fmt.Sprintf("Job Title: %s\nCompany: %s\nLocation: %s\nSalary: %s\nDetail: https://golang.cafe/job/%s\n\n", j.JobTitle, j.Company, j.Location, j.SalaryRange, j.Slug))
		lastJobID = j.ID
	}
	jobsTXT := strings.Join(jobsTXTArr, "\n")
	jobsHTML := strings.Join(jobsHTMLArr, " ")
	campaignContentHTML := `<p>Hello! Here's a list of the newest Go jobs this week on Golang Cafe!</p>
` + jobsHTML + `
	<p>Check out more jobs at <a title="Golang Cafe" href="https://golang.cafe">https://golang.cafe</a></p>
	<p>Diego from Golang Cafe</p>
	<hr />
	<h6><strong> Golang Cafe</strong> | London, United Kingdom<br />This email was sent to <a href="mailto:{$email}"><strong>{$email}</strong></a> | <a href="{$unsubscribe}">Unsubscribe</a> | <a href="{$forward}">Forward this email to a friend</a></h6>`
	campaignContentTxt := `Hello! Here's a list of the newest Go jobs this week on Golang Cafe!

` + jobsTXT + `
Check out more jobs at https://golang.cafe
	
Diego from Golang Cafe
	
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
		log.Printf("unable to update weekly campaign content got res %v", campaignUpdateRes)
		return
	}
	res.Body.Close()
	log.Printf("updated weekly campaign with html content\n")
	// send campaign
	sendReqRaw := struct {
		Type int    `json:"type"`
		Date string `json:"date"`
	}{
		Type: 1,
		Date: time.Now().Format("2006-01-02 15:04"),
	}
	sendReq, err := json.Marshal(sendReqRaw)
	if err != nil {
		log.Printf("unable create send req json for campaing id %d: %v", campaignResponse.ID, err)
		return
	}
	req, err = http.NewRequest("POST", fmt.Sprintf("https://api.mailerlite.com/api/v2/campaigns/%d/actions/send", campaignResponse.ID), bytes.NewBuffer(sendReq))
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
	out, _ := ioutil.ReadAll(res.Body)
	log.Println(string(out))
	res.Body.Close()
	log.Printf("sent weekly campaign with %d jobs via mailerlite api\n", len(jobsHTMLArr))
	lastJobIDStr = strconv.Itoa(lastJobID)
	err = database.SetValue(conn, LastSentJobIDWeeklyKey, lastJobIDStr)
	if err != nil {
		log.Fatalf("unable to save last weekly newsletter job id to db %v", err)
	}
	log.Printf("updated last sent job id weekly")
}
