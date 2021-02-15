package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/0x13a/golang.cafe/pkg/database"
	"github.com/0x13a/golang.cafe/pkg/email"
	"github.com/0x13a/golang.cafe/pkg/middleware"
	"github.com/0x13a/golang.cafe/pkg/payment"
	"github.com/0x13a/golang.cafe/pkg/server"
	"github.com/ChimeraCoder/anaconda"
	"github.com/PuerkitoBio/goquery"
	jwt "github.com/dgrijalva/jwt-go"
	humanize "github.com/dustin/go-humanize"
	"github.com/gorilla/feeds"
	"github.com/gorilla/mux"
	"github.com/machinebox/graphql"
	"github.com/microcosm-cc/bluemonday"
	"github.com/segmentio/ksuid"
)

const (
	AuthStepVerifyDeveloperProfile = "1mCQFVDZTAx9VQa1lprjr0aLgoP"
	AuthStepLoginDeveloperProfile  = "1mEvrSr2G4e4iGeucwolKW6o64d"
)

func GetAuthPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next := r.URL.Query().Get("next")
		svr.Render(w, http.StatusOK, "auth.html", map[string]interface{}{"Next": next})
	}
}

func CompaniesHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		location := vars["location"]
		page := r.URL.Query().Get("p")
		svr.RenderPageForCompanies(w, r, location, page, "companies.html")
	}
}

func DevelopersHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		location := vars["location"]
		tag := vars["tag"]
		page := r.URL.Query().Get("p")
		svr.RenderPageForDevelopers(w, r, location, tag, page, "developers.html")
	}
}

func SubmitDeveloperProfileHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(w, http.StatusOK, "submit-developer-profile.html", nil)
	}
}

func SaveDeveloperProfileHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req := &struct {
			Fullname        string `json:"fullname"`
			LinkedinURL     string `json:"linkedin_url"`
			Bio             string `json:"bio"`
			CurrentLocation string `json:"current_location"`
			Tags            string `json:"tags"`
			ProfileImageID  string `json:"profile_image_id"`
			Email           string `json:"email"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			svr.JSON(w, http.StatusBadRequest, "request is invalid")
			return
		}
		emailRe := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
		if !emailRe.MatchString(req.Email) {
			svr.JSON(w, http.StatusBadRequest, "email is invalid")
			return
		}
		linkedinRe := regexp.MustCompile(`^https:\/\/(?:[a-z]{2,3}\.)?linkedin\.com\/.*$`)
		if !linkedinRe.MatchString(req.LinkedinURL) {
			svr.JSON(w, http.StatusBadRequest, "linkedin url is invalid")
			return
		}
		req.Bio = bluemonday.StrictPolicy().Sanitize(req.Bio)
		req.Fullname = strings.Title(strings.ToLower(bluemonday.StrictPolicy().Sanitize(req.Fullname)))
		req.CurrentLocation = strings.Title(strings.ToLower(bluemonday.StrictPolicy().Sanitize(req.CurrentLocation)))
		req.Tags = bluemonday.StrictPolicy().Sanitize(req.Tags)
		if len(strings.Split(req.Tags, ",")) > 10 {
			svr.JSON(w, http.StatusBadRequest, "too many skills")
			return
		}
		existingDev, err := database.DeveloperProfileByEmail(svr.Conn, req.Email)
		if err != nil {
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		if existingDev.Email == req.Email {
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate token")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		t := time.Now().UTC()
		dev := database.Developer{
			ID:          k.String(),
			Name:        req.Fullname,
			Location:    req.CurrentLocation,
			LinkedinURL: req.LinkedinURL,
			Bio:         req.Bio,
			Available:   true,
			CreatedAt:   t,
			UpdatedAt:   t,
			Email:       req.Email,
			ImageID:     req.ProfileImageID,
			Skills:      req.Tags,
		}
		err = database.SaveTokenSignOn(svr.Conn, req.Email, k.String())
		if err != nil {
			svr.Log(err, "unable to save sign on token")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		err = database.SaveDeveloperProfile(svr.Conn, dev)
		if err != nil {
			svr.Log(err, "unable to save developer profile")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err = svr.GetEmail().SendEmail(
			"Diego from Golang Cafe <team@golang.cafe>",
			req.Email,
			email.GolangCafeEmailAddress,
			"Verify Your Developer Profile on Golang Cafe",
			fmt.Sprintf(
				"Verify Your Developer Profile on Golang Cafe https://golang.cafe/x/auth/%s?next=%s",
				k.String(),
				AuthStepVerifyDeveloperProfile,
			),
		)
		if err != nil {
			svr.Log(err, "unable to send email while submitting developer profile")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)

	}
}

func TriggerCloudflareStatsExport(svr server.Server) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				client := graphql.NewClient(svr.GetConfig().CloudflareAPIEndpoint)
				req := graphql.NewRequest(
					`query {
  viewer {
    zones(filter: {zoneTag: $zoneTag}) {
      httpRequests1dGroups(orderBy: [date_ASC]  filter: { date_gt: $fromDate } limit: 10000) {
        dimensions {
          date
        }
      sum {
        pageViews
        requests
        bytes
        cachedBytes
        threats
        countryMap {
          clientCountryName
          requests
          threats
        }
	browserMap {
          uaBrowserFamily
          pageViews
        }
        responseStatusMap {
          edgeResponseStatus
          requests
        }
      }
        uniq {
          uniques
        }
    }
  }
}
}`,
				)
				var err error
				var daysAgo int
				daysAgoStr := r.URL.Query().Get("days_ago")
				daysAgo, err = strconv.Atoi(daysAgoStr)
				if err != nil {
					daysAgo = 3
				}
				req.Var("zoneTag", svr.GetConfig().CloudflareZoneTag)
				req.Var("fromDate", time.Now().UTC().AddDate(0, 0, -daysAgo).Format("2006-01-02"))
				req.Header.Set("Cache-Control", "no-cache")
				req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", svr.GetConfig().CloudflareAPIToken))
				type cloudFlareStatsResponse struct {
					Viewer struct {
						Zones []struct {
							HttpRequests1dGroups []struct {
								Dimensions struct {
									Date string `json:"date"`
								} `json:"dimensions"`
								Sum struct {
									Bytes       uint64 `json:"bytes"`
									CachedBytes uint64 `json:"cachedBytes"`
									CountryMap  []struct {
										ClientCountryName string `json:"clientCountryName"`
										Requests          uint64 `json:"requests"`
										Threats           uint64 `json:"threats"`
									} `json:"countryMap"`
									BrowserMap []struct {
										UABrowserFamily string `json:"uaBrowserFamily"`
										PageViews       uint64 `json:"pageViews"`
									} `json:"browserMap"`
									PageViews         uint64 `json:"pageViews"`
									Requests          uint64 `json:"requests"`
									ResponseStatusMap []struct {
										EdgeResponseStatus int    `json:"edgeResponseStatus"`
										Requests           uint64 `json:"requests"`
									} `json:"responseStatusMap"`
									Threats uint64 `json:"threats"`
								} `json:"sum"`
								Uniq struct {
									Uniques uint64 `json:"uniques"`
								} `json:"uniq"`
							} `json:"httpRequests1dGroups"`
						} `json:"zones"`
					} `json:"viewer"`
				}
				var res cloudFlareStatsResponse
				if err := client.Run(context.Background(), req, &res); err != nil {
					svr.Log(err, "unable to complete graphql request to cloudflare")
					return
				}
				stat := database.CloudflareStat{}
				statusCodeStat := database.CloudflareStatusCodeStat{}
				countryStat := database.CloudflareCountryStat{}
				browserStat := database.CloudflareBrowserStat{}
				if len(res.Viewer.Zones) < 1 {
					svr.Log(errors.New("got empty response from cloudflare APIs"), "expecting 1 zone got none")
					return
				}
				log.Printf("retrieved %d cloudflare stat entries\n", len(res.Viewer.Zones[0].HttpRequests1dGroups))
				for _, d := range res.Viewer.Zones[0].HttpRequests1dGroups {
					stat.Date, err = time.Parse("2006-01-02", d.Dimensions.Date)
					if err != nil {
						svr.Log(err, "unable to parse date from cloudflare stat")
						return
					}
					stat.Bytes = d.Sum.Bytes
					stat.CachedBytes = d.Sum.CachedBytes
					stat.PageViews = d.Sum.PageViews
					stat.Requests = d.Sum.Requests
					stat.Threats = d.Sum.Threats
					stat.Uniques = d.Uniq.Uniques
					if err := database.SaveCloudflareStat(svr.Conn, stat); err != nil {
						svr.Log(err, "database.SaveCloudflareStat")
						return
					}
					// status code stat
					for _, v := range d.Sum.ResponseStatusMap {
						statusCodeStat.Date = stat.Date
						statusCodeStat.StatusCode = v.EdgeResponseStatus
						statusCodeStat.Requests = v.Requests
						if err := database.SaveCloudflareStatusCodeStat(svr.Conn, statusCodeStat); err != nil {
							svr.Log(err, "database.SaveCloudflareStatusCodeStat")
							return
						}
					}
					// country stat
					for _, v := range d.Sum.CountryMap {
						countryStat.Date = stat.Date
						countryStat.CountryCode = v.ClientCountryName
						countryStat.Requests = v.Requests
						countryStat.Threats = v.Threats
						if err := database.SaveCloudflareCountryStat(svr.Conn, countryStat); err != nil {
							svr.Log(err, "database.SaveCloudflareCountryStat")
							return
						}
					}
					// browser stat
					for _, v := range d.Sum.BrowserMap {
						browserStat.Date = stat.Date
						browserStat.PageViews = v.PageViews
						browserStat.UABrowserFamily = v.UABrowserFamily
						if err := database.SaveCloudflareBrowserStat(svr.Conn, browserStat); err != nil {
							svr.Log(err, "database.SaveCloudflareBrowserStat")
							return
						}
					}
				}
				log.Println("done exporting cloudflare stats")
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func TriggerWeeklyNewsletter(svr server.Server) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				lastJobIDStr, err := database.GetValue(svr.Conn, "last_sent_job_id_weekly")
				if err != nil {
					svr.Log(err, "unable to retrieve last newsletter weekly job id")
					return
				}
				lastJobID, err := strconv.Atoi(lastJobIDStr)
				if err != nil {
					svr.Log(err, fmt.Sprintf("unable to convert job str %s to id", lastJobIDStr))
					return
				}
				jobs, err := database.GetLastNJobsFromID(svr.Conn, svr.GetConfig().NewsletterJobsToSend, lastJobID)
				if len(jobs) < 1 {
					log.Printf("found 0 new jobs for weekly newsletter. quitting")
					return
				}
				fmt.Printf("found %d/%d jobs for weekly newsletter\n", len(jobs), svr.GetConfig().NewsletterJobsToSend)
				jsonMailerliteRq := []byte(fmt.Sprintf(`{
		"groups": [%d],
		"type": "regular",
		"subject": "Newest Go Jobs This Week",
		"from": "team@golang.cafe",
		"from_name": "Golang Cafe"
		}`, 103091230))
				client := &http.Client{}
				req, err := http.NewRequest(http.MethodPost, "https://api.mailerlite.com/api/v2/campaigns", bytes.NewBuffer(jsonMailerliteRq))
				if err != nil {
					svr.Log(err, "unable to create weekly req for mailerlite")
					return
				}
				req.Header.Add("X-MailerLite-ApiKey", svr.GetConfig().MailerLiteAPIKey)
				req.Header.Add("content-type", "application/json")
				res, err := client.Do(req)
				if err != nil {
					svr.Log(err, "unable to create weekly campaign on mailerlite")
					return
				}
				var campaignResponse struct {
					ID    int             `json:"id"`
					Error json.RawMessage `json:"error"`
				}
				if err := json.NewDecoder(res.Body).Decode(&campaignResponse); err != nil {
					svr.Log(err, "unable to read json response weekly campaign id from mailerlite")
					return
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
				campaignContentHtmlJSON, err := json.Marshal(campaignContentHTML)
				if err != nil {
					svr.Log(err, "unable to json marshal campaign content html")
					return
				}
				campaignContentTextJSON, err := json.Marshal(campaignContentTxt)
				if err != nil {
					svr.Log(err, "unable to json marshal campaign content txt")
					return
				}
				updateCampaignRq := []byte(fmt.Sprintf(`{"html": %s, "plain": %s}`, string(campaignContentHtmlJSON), string(campaignContentTextJSON)))
				req, err = http.NewRequest(http.MethodPut, fmt.Sprintf("https://api.mailerlite.com/api/v2/campaigns/%d/content", campaignResponse.ID), bytes.NewBuffer(updateCampaignRq))
				if err != nil {
					svr.Log(err, fmt.Sprintf("unable to create request for mailerlite %v", jsonMailerliteRq))
					return
				}
				req.Header.Add("X-MailerLite-ApiKey", svr.GetConfig().MailerLiteAPIKey)
				req.Header.Add("content-type", "application/json")
				res, err = client.Do(req)
				if err != nil {
					svr.Log(err, fmt.Sprintf("unable to create weekly campaign %v", jsonMailerliteRq))
					return
				}
				var campaignUpdateRes struct {
					OK bool `json:"success"`
				}
				if err := json.NewDecoder(res.Body).Decode(&campaignUpdateRes); err != nil {
					svr.Log(err, "unable to update weekly campaign content")
					return
				}
				if !campaignUpdateRes.OK {
					svr.Log(err, "unable to update weekly campaign content got non OK response")
					return
				}
				res.Body.Close()
				log.Printf("updated weekly campaign with html content\n")
				sendReqRaw := struct {
					Type int    `json:"type"`
					Date string `json:"date"`
				}{
					Type: 1,
					Date: time.Now().Format("2006-01-02 15:04"),
				}
				sendReq, err := json.Marshal(sendReqRaw)
				if err != nil {
					svr.Log(err, "unable to create send req json for campaign")
					return
				}
				req, err = http.NewRequest(http.MethodPost, fmt.Sprintf("https://api.mailerlite.com/api/v2/campaigns/%d/actions/send", campaignResponse.ID), bytes.NewBuffer(sendReq))
				if err != nil {
					svr.Log(err, fmt.Sprintf("unable to create request for campaign id req for mailerlite %s", campaignResponse.ID))
					return
				}
				req.Header.Add("X-MailerLite-ApiKey", svr.GetConfig().MailerLiteAPIKey)
				req.Header.Add("content-type", "application/json")
				res, err = client.Do(req)
				if err != nil {
					svr.Log(err, fmt.Sprintf("unable to send weeky campaign id %s", campaignResponse.ID))
					return
				}
				out, _ := ioutil.ReadAll(res.Body)
				log.Println(string(out))
				res.Body.Close()
				log.Printf("sent weekly campaign with %d jobs via mailerlite api\n", len(jobsHTMLArr))
				lastJobIDStr = strconv.Itoa(lastJobID)
				err = database.SetValue(svr.Conn, "last_sent_job_id_weekly", lastJobIDStr)
				if err != nil {
					svr.Log(err, "unable to save last weekly newsletter job id to db")
					return
				}
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func TriggerTwitterScheduler(svr server.Server) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				lastTwittedJobIDStr, err := database.GetValue(svr.Conn, "last_twitted_job_id")
				if err != nil {
					svr.Log(err, "unable to retrieve last twitter job id")
					return
				}
				lastTwittedJobID, err := strconv.Atoi(lastTwittedJobIDStr)
				if err != nil {
					svr.Log(err, "unable to convert job str to id")
					return
				}
				jobs, err := database.GetLastNJobsFromID(svr.Conn, svr.GetConfig().TwitterJobsToPost, lastTwittedJobID)
				log.Printf("found %d/%d jobs to post on twitter\n", len(jobs), svr.GetConfig().TwitterJobsToPost)
				if len(jobs) == 0 {
					return
				}
				lastJobID := lastTwittedJobID
				api := anaconda.NewTwitterApiWithCredentials(svr.GetConfig().TwitterAccessToken, svr.GetConfig().TwitterAccessTokenSecret, svr.GetConfig().TwitterClientKey, svr.GetConfig().TwitterClientSecret)
				for _, j := range jobs {
					_, err := api.PostTweet(fmt.Sprintf("%s with %s - %s | %s\n\n#golang #golangjobs\n\nhttps://golang.cafe/job/%s", j.JobTitle, j.Company, j.Location, j.SalaryRange, j.Slug), url.Values{})
					if err != nil {
						svr.Log(err, "unable to post tweet")
						continue
					}
					lastJobID = j.ID
				}
				lastJobIDStr := strconv.Itoa(lastJobID)
				err = database.SetValue(svr.Conn, "last_twitted_job_id", lastJobIDStr)
				if err != nil {
					svr.Log(err, fmt.Sprintf("unable to save last twitter job id to db as %s", lastJobIDStr))
					return
				}
				log.Printf("updated last twitted job id to %s\n", lastJobIDStr)
				log.Printf("posted last %d jobs to twitter", len(jobs))
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func TriggerCompanyUpdater(svr server.Server) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				since := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
				cs, err := database.InferCompaniesFromJobs(svr.Conn, since)
				if err != nil {
					svr.Log(err, "unable to infer companies from jobs")
					return
				}
				log.Printf("inferred %d companies...\n", len(cs))
				for _, c := range cs {
					res, err := http.Get(c.URL)
					if err != nil {
						svr.Log(err, fmt.Sprintf("http.Get(%s): unable to get url", c.URL))
						continue
					}
					defer res.Body.Close()
					if res.StatusCode != http.StatusOK {
						svr.Log(errors.New("non 200 status code"), fmt.Sprintf("GET %s: status code error: %d %s", c.URL, res.StatusCode, res.Status))
						continue
					}

					doc, err := goquery.NewDocumentFromReader(res.Body)
					if err != nil {
						svr.Log(err, "goquery.NewDocumentFromReader")
						continue
					}
					var description string
					doc.Find("meta").Each(func(i int, s *goquery.Selection) {
						if name, _ := s.Attr("name"); strings.EqualFold(name, "description") {
							var ok bool
							description, ok = s.Attr("content")
							if !ok {
								svr.Log(errors.New("s.Attr error"), fmt.Sprintf("unable to retrieve content for description tag for company url: %s", c.URL))
								return
							}
							log.Printf("%s: description: %s\n", c.URL, description)
						}
					})
					if description != "" {
						c.Description = &description
					}
					companyID, err := ksuid.NewRandom()
					if err != nil {
						svr.Log(err, "ksuid.NewRandom: unable to generate company id")
						continue
					}
					newIconID, err := ksuid.NewRandom()
					if err != nil {
						svr.Log(err, "ksuid.NewRandom: unable to generate new icon id")
						continue
					}
					if err := database.DuplicateImage(svr.Conn, c.IconImageID, newIconID.String()); err != nil {
						svr.Log(err, "database.DuplicateImage")
						continue
					}
					c.ID = companyID.String()
					c.IconImageID = newIconID.String()
					if err := database.SaveCompany(svr.Conn, c); err != nil {
						svr.Log(err, "database.SaveCompany")
						continue
					}
					log.Println(c.Name)
				}
				if err := database.DeleteStaleImages(svr.Conn); err != nil {
					svr.Log(err, "database.DeleteStaleImages")
					return
				}
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func TriggerAdsManager(svr server.Server) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				log.Printf("attempting to demote expired sponsored 30days pinned job ads\n")
				jobs, err := database.GetJobsOlderThan(svr.Conn, time.Now().AddDate(0, 0, -30), database.JobAdSponsoredPinnedFor30Days)
				if err != nil {
					svr.Log(err, "unable to demote expired sponsored 30 days pinned job ads")
					return
				}
				for _, j := range jobs {
					jobToken, err := database.TokenByJobID(svr.Conn, j.ID)
					if err != nil {
						svr.Log(err, fmt.Sprintf("unable to retrieve token for job id %d and email %s", j.ID, j.CompanyEmail))
						continue
					} else {
						err = svr.GetEmail().SendEmail(
							"Diego from Golang Cafe <team@golang.cafe>",
							j.CompanyEmail,
							email.GolangCafeEmailAddress,
							"Your Job Ad on Golang Cafe Has Expired",
							fmt.Sprintf(
								"Your Premium Job Ad has expired and it's no longer pinned to the front-page. If you want to keep your Job Ad on the front-page you can upgrade in a few clicks on the Job Edit Page by following this link https://golang.cafe/edit/%s?expired=1", jobToken,
							),
						)
						if err != nil {
							svr.Log(err, fmt.Sprintf("unable to send email while updating job ad type for job id %d", j.ID))
							continue
						}
					}
					database.UpdateJobAdType(svr.Conn, database.JobAdBasic, j.ID)
					log.Printf("demoted job id %d expired sponsored 30days pinned job ads\n", j.ID)
				}

				log.Printf("attempting to demote expired sponsored 7days pinned job ads\n")
				jobs2, err := database.GetJobsOlderThan(svr.Conn, time.Now().AddDate(0, 0, -7), database.JobAdSponsoredPinnedFor7Days)
				if err != nil {
					svr.Log(err, "unable to demote expired sponsored 7 days pinned job ads")
					return
				}
				for _, j := range jobs2 {
					jobToken, err := database.TokenByJobID(svr.Conn, j.ID)
					if err != nil {
						svr.Log(err, fmt.Sprintf("unable to retrieve toke for job id %d and email %s", j.ID, j.CompanyEmail))
						continue
					} else {
						err = svr.GetEmail().SendEmail(
							"Diego from Golang Cafe <team@golang.cafe>",
							j.CompanyEmail,
							email.GolangCafeEmailAddress,
							"Your Job Ad on Golang Cafe Has Expired",
							fmt.Sprintf(
								"Your Premium Job Ad has expired and it's no longer pinned to the front-page. If you want to keep your Job Ad on the front-page you can upgrade in a few clicks on the Job Edit Page by following this link https://golang.cafe/edit/%s?expired=1", jobToken,
							),
						)
						if err != nil {
							svr.Log(err, fmt.Sprintf("unable to send email while updating job ad type for job id %d", j.ID))
							continue
						}
					}
					database.UpdateJobAdType(svr.Conn, database.JobAdBasic, j.ID)
					log.Printf("demoted job id %d expired sponsored 7days pinned job ads\n", j.ID)
				}
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func UpdateDeveloperProfileHandler(svr server.Server) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			req := &struct {
				ID              string `json:"id"`
				Fullname        string `json:"fullname"`
				LinkedinURL     string `json:"linkedin_url"`
				Bio             string `json:"bio"`
				CurrentLocation string `json:"current_location"`
				Skills          string `json:"skills"`
				ImageID         string `json:"profile_image_id"`
				Email           string `json:"email"`
				Available       bool   `json:"available"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			emailRe := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
			if !emailRe.MatchString(req.Email) {
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			linkedinRe := regexp.MustCompile(`^https:\/\/(?:[a-z]{2,3}\.)?linkedin\.com\/.*$`)
			if !linkedinRe.MatchString(req.LinkedinURL) {
				svr.JSON(w, http.StatusBadRequest, "linkedin url is invalid")
				return
			}
			req.Bio = bluemonday.StrictPolicy().Sanitize(req.Bio)
			req.Fullname = strings.Title(strings.ToLower(bluemonday.StrictPolicy().Sanitize(req.Fullname)))
			req.CurrentLocation = strings.Title(strings.ToLower(bluemonday.StrictPolicy().Sanitize(req.CurrentLocation)))
			req.Skills = bluemonday.StrictPolicy().Sanitize(req.Skills)
			if len(strings.Split(req.Skills, ",")) > 10 {
				svr.JSON(w, http.StatusBadRequest, "too many skills")
				return
			}
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			if req.Email != profile.Email && !profile.IsAdmin {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			t := time.Now().UTC()
			dev := database.Developer{
				ID:          req.ID,
				Name:        req.Fullname,
				Location:    req.CurrentLocation,
				LinkedinURL: req.LinkedinURL,
				Bio:         req.Bio,
				Available:   req.Available,
				UpdatedAt:   t,
				Email:       req.Email,
				Skills:      req.Skills,
				ImageID:     req.ImageID,
			}
			err = database.UpdateDeveloperProfile(svr.Conn, dev)
			if err != nil {
				svr.Log(err, "unable to update developer profile")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func DeleteDeveloperProfileHandler(svr server.Server) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			req := &struct {
				ID      string `json:"id"`
				ImageID string `json:"image_id"`
				Email   string `json:"email"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			emailRe := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
			if !emailRe.MatchString(req.Email) {
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			if profile.Email != req.Email && !profile.IsAdmin {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			err = database.DeleteDeveloperProfile(svr.Conn, req.ID, req.Email)
			if err != nil {
				svr.Log(err, "unable to delete developer profile")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			if imageErr := database.DeleteImageByID(svr.Conn, req.ImageID); imageErr != nil {
				svr.Log(err, "unable to delete developer profile image id "+req.ImageID)
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			if userErr := database.DeleteUserByEmail(svr.Conn, req.Email); userErr != nil {
				svr.Log(err, "unable to delete user by email "+req.Email)
				svr.JSON(w, http.StatusInternalServerError, nil)
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func SendMessageDeveloperProfileHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		profileID := vars["id"]
		req := &struct {
			Content string `json:"content"`
			Email   string `json:"email"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		emailRe := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
		if !emailRe.MatchString(req.Email) {
			svr.JSON(w, http.StatusBadRequest, "invalid email provided")
			return
		}
		linkRe := regexp.MustCompile(`(?:(?:https?|ftp):\/\/)?[\w/\-?=%.]+\.[\w/\-&?=%.]+`)
		if linkRe.MatchString(req.Content) {
			svr.JSON(w, http.StatusUnprocessableEntity, "message should not contain links")
			return
		}
		dev, err := database.DeveloperProfileByID(svr.Conn, profileID)
		if err != nil {
			svr.Log(err, "unable to find developer profile by id "+profileID)
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate message ID")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		message := database.DeveloperMessage{
			ID:        k.String(),
			Email:     req.Email,
			Content:   req.Content,
			ProfileID: dev.ID,
		}
		err = database.SendMessageDeveloperProfile(svr.Conn, message)
		if err != nil {
			svr.Log(err, "unable to send message to developer profile")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		err = svr.GetEmail().SendEmail(
			"Diego from Golang Cafe <team@golang.cafe>",
			dev.Email,
			req.Email,
			"Confirm Your Message on Golang Cafe",
			fmt.Sprintf(
				"You have sent a message through Golang Cafe: \n\nMessage: %s\n\nPlease follow this link to confirm and deliver your message: %s\n\nIf this was not requested by you, you can ignore this email.",
				req.Content,
				fmt.Sprintf("https://golang.cafe/x/auth/message/%s", k.String()),
			),
		)
		if err != nil {
			svr.Log(err, "unable to send email while submitting message")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func DeliverMessageDeveloperProfileHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		messageID := vars["id"]
		message, email, err := database.MessageForDeliveryByID(svr.Conn, messageID)
		if err != nil {
			svr.JSON(w, http.StatusBadRequest, "Your link may be invalid or expired")
			return
		}
		err = svr.GetEmail().SendEmail(
			"Diego from Golang Cafe <team@golang.cafe>",
			email,
			message.Email,
			"New Message from Golang Cafe on Golang Cafe",
			fmt.Sprintf(
				"You received a new message from Golang Cafe: \n\nMessage: %s\n\nFrom: %s",
				message.Content,
				message.Email,
			),
		)
		if err != nil {
			svr.Log(err, "unable to send email to developer profile")
			svr.JSON(w, http.StatusBadRequest, "There was a problem while sending the email")
			return
		}
		if err := database.MarkDeveloperMessageAsSent(svr.Conn, messageID); err != nil {
			svr.Log(err, "unable to mark developer message as sent "+messageID)
		}
		svr.JSON(w, http.StatusOK, "Message Sent Successfully")
	}
}

func EditDeveloperProfileHandler(svr server.Server) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			profileID := vars["id"]
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			dev, err := database.DeveloperProfileByID(svr.Conn, profileID)
			if err != nil {
				svr.Log(err, "unable to find developer profile by id "+profileID)
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			if dev.Email != profile.Email && !profile.IsAdmin {
				svr.JSON(w, http.StatusForbidden, "forbidden")
				return
			}
			svr.Render(w, http.StatusOK, "edit-developer-profile.html", map[string]interface{}{
				"DeveloperProfile": dev,
			})
		},
	)
}

func ViewDeveloperProfileHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		profileSlug := vars["slug"]
		dev, err := database.DeveloperProfileBySlug(svr.Conn, profileSlug)
		if err != nil {
			svr.Log(err, "unable to find developer profile by slug "+profileSlug)
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		dev.UpdatedAtHumanized = dev.UpdatedAt.UTC().Format("January 2006")
		dev.SkillsArray = strings.Split(dev.Skills, ",")
		svr.Render(w, http.StatusOK, "view-developer-profile.html", map[string]interface{}{
			"DeveloperProfile": dev,
			"MonthAndYear":     time.Now().UTC().Format("January 2006"),
		})
	}
}

func CompaniesForLocationHandler(svr server.Server, loc string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("p")
		svr.RenderPageForCompanies(w, r, loc, page, "companies.html")
	}
}

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

		svr.RenderPageForLocationAndTag(w, r, "", "", page, "landing.html")
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

func ShowPaymentPage(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := r.URL.Query().Get("email")
		currency := r.URL.Query().Get("currency")
		amount, err := strconv.Atoi(r.URL.Query().Get("amount"))
		if err != nil {
			svr.JSON(w, http.StatusBadRequest, "invalid amount")
			return
		}
		if amount < 1900 || amount > 9900 {
			svr.JSON(w, http.StatusBadRequest, "invalid amount")
			return
		}
		if currency != "EUR" && currency != "GBP" && currency != "USD" {
			svr.JSON(w, http.StatusBadRequest, "invalid currency")
			return
		}
		if email == "" {
			svr.JSON(w, http.StatusBadRequest, "invalid email")
		}
		curSymb := map[string]string{"USD": "$", "GBP": "£", "EUR": "€"}
		svr.Render(w, http.StatusOK, "payment.html", map[string]interface{}{
			"Currency":             currency,
			"CurrencySymbol":       curSymb[currency],
			"StripePublishableKey": svr.GetConfig().StripePublishableKey,
			"Email":                email,
			"Amount":               amount / 100,
			"AmountPence":          amount,
		})
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

func SendFeedbackMessage(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req := &struct {
			Email   string `json:"email"`
			Message string `json:"message"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		emailRe := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
		if !emailRe.MatchString(req.Email) {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		if svr.SeenSince(r, time.Duration(1*time.Hour)) {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err := svr.
			GetEmail().
			SendEmail(
				"Diego from Golang Cafe <team@golang.cafe>",
				email.GolangCafeEmailAddress,
				req.Email,
				"New Feedback Message",
				fmt.Sprintf("From: %s\nMessage: %s", req.Email, req.Message),
			)
		if err != nil {
			svr.Log(err, "unable to send email for feedback message")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func RequestTokenSignOn(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next := r.URL.Query().Get("next")
		req := &struct {
			Email string `json:"email"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		emailRe := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
		if !emailRe.MatchString(req.Email) {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err = database.SaveTokenSignOn(svr.Conn, req.Email, k.String())
		if err != nil {
			svr.Log(err, "unable to save sign on token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		token := k.String()
		if next != "" {
			token += "?next=" + next
		}
		err = svr.GetEmail().SendEmail("Diego from Golang Cafe <team@golang.cafe>", req.Email, email.GolangCafeEmailAddress, "Sign On on Golang Cafe", fmt.Sprintf("Sign On on Golang Cafe https://golang.cafe/x/auth/%s", token))
		if err != nil {
			svr.Log(err, "unable to send email while applying to job")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func VerifyTokenSignOn(svr server.Server, adminEmail string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		token := vars["token"]
		user, err := database.ValidateSignOnToken(svr.Conn, token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to validate signon token %s", token))
			svr.TEXT(w, http.StatusBadRequest, "Invalid or expired token")
			return
		}
		sess, err := svr.SessionStore.Get(r, "____gc")
		if err != nil {
			svr.TEXT(w, http.StatusInternalServerError, "Invalid or expired token")
			return
		}
		stdClaims := &jwt.StandardClaims{
			ExpiresAt: time.Now().Add(30 * 24 * time.Hour).UTC().Unix(),
			IssuedAt:  time.Now().UTC().Unix(),
			Issuer:    "https://golang.cafe",
		}
		claims := middleware.UserJWT{
			UserID:         user.ID,
			Email:          user.Email,
			IsAdmin:        user.Email == adminEmail,
			StandardClaims: *stdClaims,
		}
		tkn := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		ss, err := tkn.SignedString(svr.GetJWTSigningKey())
		sess.Values["jwt"] = ss
		err = sess.Save(r, w)
		if err != nil {
			svr.Log(err, "unable to save jwt into session cookie")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		next := r.URL.Query().Get("next")
		switch {
		case AuthStepVerifyDeveloperProfile == next:
			if activateDevProfileErr := database.ActivateDeveloperProfile(svr.Conn, user.Email); activateDevProfileErr != nil {
				svr.Log(err, "unable to activate developer profile")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			dev, err := database.DeveloperProfileByEmail(svr.Conn, user.Email)
			if err != nil {
				svr.Log(err, "unable to find developer profile by email")
				svr.JSON(w, http.StatusNotFound, "unable to find developer profile by email")
				return
			}
			svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("/edit/profile/%s", dev.ID))
			return
		case AuthStepLoginDeveloperProfile == next:
			dev, err := database.DeveloperProfileByEmail(svr.Conn, user.Email)
			if err != nil {
				svr.Log(err, "unable to find developer profile by email")
				svr.JSON(w, http.StatusNotFound, "unable to find developer profile by email")
				return
			}
			svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("/edit/profile/%s", dev.ID))
			return
		case claims.IsAdmin:
			svr.Redirect(w, r, http.StatusMovedPermanently, "/manage/list")
			return
		}
		svr.Log(errors.New("unable to find next step in token verification flow"), fmt.Sprintf("unable to know next step for %s token %s and next %s", user.Email, token, next))
		svr.Redirect(w, r, http.StatusMovedPermanently, "/")
	}
}

func ListJobsAsAdminPageHandler(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			loc := r.URL.Query().Get("l")
			skill := r.URL.Query().Get("s")
			page := r.URL.Query().Get("p")
			svr.RenderPageForLocationAndTagAdmin(w, loc, skill, page, "list-jobs-admin.html")
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
		jobLocations := strings.Split(job.Location, "/")
		var isQuickApply bool
		emailRe := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
		if emailRe.MatchString(job.HowToApply) {
			isQuickApply = true
		}
		job.SalaryRange = fmt.Sprintf("%s%s to %s%s", job.SalaryCurrency, humanize.Comma(job.SalaryMin), job.SalaryCurrency, humanize.Comma(job.SalaryMax))
		svr.Render(w, http.StatusOK, "job.html", map[string]interface{}{
			"Job":                     job,
			"JobURIEncoded":           url.QueryEscape(job.Slug),
			"IsQuickApply":            isQuickApply,
			"HTMLJobDescription":      svr.MarkdownToHTML(job.JobDescription),
			"HTMLJobPerks":            svr.MarkdownToHTML(job.Perks),
			"HTMLJobInterviewProcess": svr.MarkdownToHTML(job.InterviewProcess),
			"LocationFilter":          location,
			"ExternalJobId":           job.ExternalID,
			"MonthAndYear":            time.Unix(job.CreatedAt, 0).UTC().Format("January 2006"),
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
		svr.RenderPageForLocationAndTag(w, r, location, "", page, "landing.html")
	}
}

func LandingPageForLocationAndSkillPlaceholderHandler(svr server.Server, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, location, skill, page, "landing.html")
	}
}

func LandingPageForLocationPlaceholderHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		loc := strings.ReplaceAll(vars["location"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, loc, "", page, "landing.html")
	}
}

func LandingPageForSkillPlaceholderHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, "", skill, page, "landing.html")
	}
}

func LandingPageForSkillAndLocationPlaceholderHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		loc := strings.ReplaceAll(vars["location"], "-", " ")
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, loc, skill, page, "landing.html")
	}
}

func ServeRSSFeed(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobs, err := database.GetLastNJobs(svr.Conn, 20, r.URL.Query().Get("l"))
		if err != nil {
			svr.Log(err, "unable to retrieve jobs for RSS Feed")
			svr.XML(w, http.StatusInternalServerError, []byte{})
			return
		}
		now := time.Now()
		feed := &feeds.Feed{
			Title:       "Golang Cafe Jobs",
			Link:        &feeds.Link{Href: "https://golang.cafe"},
			Description: "Golang Cafe Jobs",
			Author:      &feeds.Author{Name: "Golang Cafe", Email: "team@golang.cafe"},
			Created:     now,
		}

		for _, j := range jobs {
			if j.CompanyIconID != "" {
				feed.Items = append(feed.Items, &feeds.Item{
					Title:       fmt.Sprintf("%s with %s - %s", j.JobTitle, j.Company, j.Location),
					Link:        &feeds.Link{Href: fmt.Sprintf("https://golang.cafe/job/%s", j.Slug)},
					Description: string(svr.MarkdownToHTML(j.JobDescription + "\n\n**Salary Range:** " + j.SalaryRange)),
					Author:      &feeds.Author{Name: "Golang Cafe", Email: "team@golang.cafe"},
					Enclosure:   &feeds.Enclosure{Length: "not implemented", Type: "image", Url: fmt.Sprintf("https://golang.cafe/x/s/m/%s", j.CompanyIconID)},
					Created:     *j.ApprovedAt,
				})
			} else {
				feed.Items = append(feed.Items, &feeds.Item{
					Title:       fmt.Sprintf("%s with %s - %s", j.JobTitle, j.Company, j.Location),
					Link:        &feeds.Link{Href: fmt.Sprintf("https://golang.cafe/job/%s", j.Slug)},
					Description: string(svr.MarkdownToHTML(j.JobDescription + "\n\n**Salary Range:** " + j.SalaryRange)),
					Author:      &feeds.Author{Name: "Golang Cafe", Email: "team@golang.cafe"},
					Created:     *j.ApprovedAt,
				})
			}
		}
		rssFeed, err := feed.ToRss()
		if err != nil {
			svr.Log(err, "unable to convert rss feed to xml")
			svr.XML(w, http.StatusInternalServerError, []byte{})
			return
		}
		svr.XML(w, http.StatusOK, []byte(rssFeed))
	}
}

func StripePaymentConfirmationWebookHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		const MaxBodyBytes = int64(65536)
		req.Body = http.MaxBytesReader(w, req.Body, MaxBodyBytes)
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			svr.Log(err, "error reading request body from stripe")
			svr.JSON(w, http.StatusServiceUnavailable, nil)
			return
		}

		stripeSig := req.Header.Get("Stripe-Signature")
		sess, err := payment.HandleCheckoutSessionComplete(body, svr.GetConfig().StripeEndpointSecret, stripeSig)
		if err != nil {
			svr.Log(err, "error while handling checkout session complete")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		if sess != nil {
			affectedRows, err := database.SaveSuccessfulPayment(svr.Conn, sess.ID)
			if err != nil {
				svr.Log(err, "error while saving successful payment")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			if affectedRows != 1 {
				svr.Log(errors.New("invalid number of rows affected when saving payment"), fmt.Sprintf("got %d expected 1", affectedRows))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			job, err := database.GetJobByStripeSessionID(svr.Conn, sess.ID)
			if err != nil {
				svr.Log(errors.New("unable to find job by stripe session id"), fmt.Sprintf("session id %s", sess.ID))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			purchaseEvent, err := database.GetPurchaseEventBySessionID(svr.Conn, sess.ID)
			if err != nil {
				svr.Log(errors.New("unable to find purchase event by stripe session id"), fmt.Sprintf("session id %s", sess.ID))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			jobToken, err := database.TokenByJobID(svr.Conn, job.ID)
			if err != nil {
				svr.Log(errors.New("unable to find token for job id"), fmt.Sprintf("session id %s job id %d", sess.ID, job.ID))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			if job.ApprovedAt != nil && job.AdType != database.JobAdSponsoredPinnedFor30Days && job.AdType != database.JobAdSponsoredPinnedFor7Days && (purchaseEvent.AdType == database.JobAdSponsoredPinnedFor7Days || job.AdType != database.JobAdSponsoredPinnedFor30Days) {
				err := database.UpdateJobAdType(svr.Conn, purchaseEvent.AdType, job.ID)
				if err != nil {
					svr.Log(errors.New("unable to update job to new ad type"), fmt.Sprintf("unable to update job id %d to new ad type %d for session id %s", job.ID, purchaseEvent.AdType, sess.ID))
					svr.JSON(w, http.StatusBadRequest, nil)
					return
				}
				err = svr.GetEmail().SendEmail("Diego from Golang Cafe <team@golang.cafe>", purchaseEvent.Email, email.GolangCafeEmailAddress, "Your Job Ad on Golang Cafe", fmt.Sprintf("Your Job Ad has been upgraded successfully and it's now pinned to the home page. You can edit the Job Ad at any time and check page views and clickouts by following this link https://golang.cafe/edit/%s", jobToken))
				if err != nil {
					svr.Log(err, "unable to send email while upgrading job ad")
				}
			}
			svr.JSON(w, http.StatusOK, nil)
			return
		}

		svr.JSON(w, http.StatusOK, nil)
	}
}
