package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/0x13a/golang.cafe/internal/company"
	"github.com/0x13a/golang.cafe/internal/developer"
	"github.com/0x13a/golang.cafe/internal/job"
	"github.com/0x13a/golang.cafe/internal/user"
	"github.com/0x13a/golang.cafe/internal/database"
	"github.com/0x13a/golang.cafe/internal/email"
	"github.com/0x13a/golang.cafe/internal/imagemeta"
	"github.com/0x13a/golang.cafe/internal/middleware"
	"github.com/0x13a/golang.cafe/internal/payment"
	"github.com/0x13a/golang.cafe/internal/seo"
	"github.com/0x13a/golang.cafe/internal/server"
	"github.com/ChimeraCoder/anaconda"
	"github.com/PuerkitoBio/goquery"
	"github.com/bot-api/telegram"
	jwt "github.com/dgrijalva/jwt-go"
	humanize "github.com/dustin/go-humanize"
	"github.com/gorilla/feeds"
	"github.com/gorilla/mux"
	"github.com/gosimple/slug"
	"github.com/machinebox/graphql"
	"github.com/microcosm-cc/bluemonday"
	"github.com/nfnt/resize"
	"github.com/segmentio/ksuid"
	"github.com/snabb/sitemap"
)

const (
	AuthStepVerifyDeveloperProfile = "1mCQFVDZTAx9VQa1lprjr0aLgoP"
	AuthStepLoginDeveloperProfile  = "1mEvrSr2G4e4iGeucwolKW6o64d"
)

type devGetter interface {
	DeveloperProfileByEmail(email string) (developer.Developer, error)
}

type devSaver interface {
	SaveDeveloperProfile(dev developer.Developer) error
}

type devGetSaver interface {
	devGetter
	devSaver
}

type tokenSaver interface {
	SaveTokenSignOn(email, token string) error
}

func GetAuthPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next := r.URL.Query().Get("next")
		svr.Render(w, http.StatusOK, "auth.html", map[string]interface{}{"Next": next})
	}
}

func CompaniesHandler(svr server.Server, companyRepo *company.Repository, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		location := vars["location"]
		page := r.URL.Query().Get("p")
		svr.RenderPageForCompanies(w, r, companyRepo, jobRepo, location, page, "companies.html")
	}
}

func DevelopersHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		location := vars["location"]
		tag := vars["tag"]
		page := r.URL.Query().Get("p")
		svr.RenderPageForDevelopers(w, r, devRepo, location, tag, page, "developers.html")
	}
}

func SubmitDeveloperProfileHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForDeveloperRegistration(w, r, devRepo, "submit-developer-profile.html")
	}
}

func SaveDeveloperProfileHandler(svr server.Server, devRepo devGetSaver, userRepo tokenSaver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req := &struct {
			Fullname        string  `json:"fullname"`
			LinkedinURL     string  `json:"linkedin_url"`
			GithubURL       *string `json:"github_url,omitempty"`
			TwitterURL      *string `json:"twitter_url,omitempty"`
			Bio             string  `json:"bio"`
			CurrentLocation string  `json:"current_location"`
			Tags            string  `json:"tags"`
			ProfileImageID  string  `json:"profile_image_id"`
			Email           string  `json:"email"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			svr.JSON(w, http.StatusBadRequest, "request is invalid")
			return
		}
		if !svr.IsEmail(req.Email) {
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
		existingDev, err := devRepo.DeveloperProfileByEmail(req.Email)
		if err != nil {
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		if existingDev.Email == req.Email {
			svr.JSON(w, http.StatusBadRequest, "developer profile with this email already exists")
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate token")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		t := time.Now().UTC()
		dev := developer.Developer{
			ID:          k.String(),
			Name:        req.Fullname,
			Location:    req.CurrentLocation,
			LinkedinURL: req.LinkedinURL,
			GithubURL:   req.GithubURL,
			TwitterURL:  req.TwitterURL,
			Bio:         req.Bio,
			Available:   true,
			CreatedAt:   t,
			UpdatedAt:   t,
			Email:       strings.ToLower(req.Email),
			ImageID:     req.ProfileImageID,
			Skills:      req.Tags,
		}
		err = userRepo.SaveTokenSignOn(strings.ToLower(req.Email), k.String())
		if err != nil {
			svr.Log(err, "unable to save sign on token")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		err = devRepo.SaveDeveloperProfile(dev)
		if err != nil {
			svr.Log(err, "unable to save developer profile")
			svr.JSON(w, http.StatusInternalServerError, nil)
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
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func TriggerFXRateUpdate(svr server.Server) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				log.Println("going through list of available currencies")
				for _, base := range svr.GetConfig().AvailableCurrencies {
					req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://freecurrencyapi.net/api/v2/latest?apikey=%s&base_currency=%s", svr.GetConfig().FXAPIKey, base), nil)
					if err != nil {
						svr.Log(err, "http.NewRequest")
						continue
					}
					res, err := http.DefaultClient.Do(req)
					if err != nil {
						svr.Log(err, "http.DefaultClient.Do")
						continue
					}
					var ratesResponse struct {
						Query struct {
							Base      string `json:"base_currency"`
							Timestamp int    `json:"timestamp"`
						} `json:"query"`
						Rates map[string]interface{} `json:"data"`
					}
					defer res.Body.Close()
					if err := json.NewDecoder(res.Body).Decode(&ratesResponse); err != nil {
						svr.Log(err, "json.NewDecoder(res.Body).Decode(ratesResponse)")
						continue
					}
					log.Printf("rate response for currency %s: %#v", base, ratesResponse)
					if ratesResponse.Query.Base != base {
						svr.Log(errors.New("got different base currency than requested"), "inconsistent reply from APIs")
						continue
					}
					for _, target := range svr.GetConfig().AvailableCurrencies {
						if target == base {
							continue
						}
						value, ok := ratesResponse.Rates[target]
						if !ok {
							svr.Log(errors.New("could not find target currency"), fmt.Sprintf("could not find target currency %s for base %s", target, base))
							continue
						}
						log.Println("updating fx rate pair ", base, target, value)
						valueFloat, ok := value.(float64)
						if !ok {
							svr.Log(errors.New("unable to cast to float"), "parsing value to float64")
							continue
						}
						fx := database.FXRate{
							Base:      base,
							UpdatedAt: time.Now(),
							Target:    target,
							Value:     valueFloat,
						}
						if err := database.AddFXRate(svr.Conn, fx); err != nil {
							svr.Log(err, "database.AddFxRate")
							continue
						}
					}
				}
			}()
		},
	)
}

func TriggerSitemapUpdate(svr server.Server, devRepo *developer.Repository, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				database.SaveSEOSkillFromCompany(svr.Conn)
				landingPages, err := seo.GenerateSearchSEOLandingPages(svr.Conn)
				if err != nil {
					svr.Log(err, "seo.GenerateSearchSEOLandingPages")
					return
				}
				postAJobLandingPages, err := seo.GeneratePostAJobSEOLandingPages(svr.Conn)
				if err != nil {
					svr.Log(err, "seo.GeneratePostAJobSEOLandingPages")
					return
				}
				salaryLandingPages, err := seo.GenerateSalarySEOLandingPages(svr.Conn)
				if err != nil {
					svr.Log(err, "seo.GenerateSalarySEOLandingPages")
					return
				}
				companyLandingPages, err := seo.GenerateCompaniesLandingPages(svr.Conn)
				if err != nil {
					svr.Log(err, "seo.GenerateCompaniesLandingPages")
					return
				}
				developerSkillsPages, err := seo.GenerateDevelopersSkillLandingPages(devRepo)
				if err != nil {
					svr.Log(err, "seo.GenerateDevelopersSkillLandingPages")
					return
				}
				developerProfilePages, err := seo.GenerateDevelopersProfileLandingPages(devRepo)
				if err != nil {
					svr.Log(err, "seo.GenerateDevelopersProfileLandingPages")
					return
				}
				companyProfilePages, err := seo.GenerateDevelopersProfileLandingPages(devRepo)
				if err != nil {
					svr.Log(err, "seo.GenerateDevelopersProfileLandingPages")
					return
				}
				developerLocationPages, err := seo.GenerateDevelopersLocationPages(svr.Conn)
				if err != nil {
					svr.Log(err, "seo.GenerateDevelopersLocationPages")
					return
				}
				blogPosts, err := seo.BlogPages("./static/blog")
				if err != nil {
					svr.Log(err, "seo.BlogPages")
					return
				}
				pages := seo.StaticPages()
				jobPosts, err := jobRepo.JobPostByCreatedAt()
				if err != nil {
					svr.Log(err, "database.JobPostByCreatedAt")
					return
				}
				n := time.Now().UTC()

				database.CreateTmpSitemapTable(svr.Conn)
				for _, j := range jobPosts {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`https://golang.cafe/job/%s`, j.Slug),
						LastMod:    time.Unix(j.CreatedAt, 0),
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", j.Slug))
					}
				}

				for _, b := range blogPosts {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`https://golang.cafe/blog/%s`, b.Path),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", b.Path))
					}
				}

				for _, p := range pages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for _, p := range postAJobLandingPages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for _, p := range salaryLandingPages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for _, p := range landingPages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p.URI),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for _, p := range companyLandingPages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for _, p := range developerSkillsPages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for _, p := range developerProfilePages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}
				for _, p := range companyProfilePages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for _, p := range developerLocationPages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}
				if err := database.SwapSitemapTable(svr.Conn); err != nil {
					svr.Log(err, "database.SwapSitemapTable")
				}
			}()
		})
}

func TriggerExpiredJobsTask(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				jobURLs, err := jobRepo.GetJobApplyURLs()
				if err != nil {
					svr.Log(err, "unable to get job apply URL for cleanup")
					return
				}
				for _, jobURL := range jobURLs {
					if svr.IsEmail(jobURL.URL) {
						continue
					}
					res, err := http.Get(jobURL.URL)
					if err != nil {
						svr.Log(err, fmt.Sprintf("error while checking expired apply URL for job %d %s", jobURL.ID, jobURL.URL))
						continue
					}
					if res.StatusCode == http.StatusNotFound {
						fmt.Printf("found expired job %d URL %s returned 404\n", jobURL.ID, jobURL.URL)
						if err := jobRepo.MarkJobAsExpired(jobURL.ID); err != nil {
							svr.Log(err, fmt.Sprintf("unable to mark job %d %s as expired", jobURL.ID, jobURL.URL))
						}
					}
				}
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func TriggerUpdateLastWeekClickouts(svr server.Server) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				err := database.UpdateLastWeekClickouts(svr.Conn)
				if err != nil {
					svr.Log(err, "unable to update last week clickouts")
					return
				}
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
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

func TriggerWeeklyNewsletter(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				lastJobIDStr, err := jobRepo.GetValue("last_sent_job_id_weekly")
				if err != nil {
					svr.Log(err, "unable to retrieve last newsletter weekly job id")
					return
				}
				lastJobID, err := strconv.Atoi(lastJobIDStr)
				if err != nil {
					svr.Log(err, fmt.Sprintf("unable to convert job str %s to id", lastJobIDStr))
					return
				}
				jobPosts, err := jobRepo.GetLastNJobsFromID(svr.GetConfig().NewsletterJobsToSend, lastJobID)
				if len(jobPosts) < 1 {
					log.Printf("found 0 new jobs for weekly newsletter. quitting")
					return
				}
				fmt.Printf("found %d/%d jobs for weekly newsletter\n", len(jobPosts), svr.GetConfig().NewsletterJobsToSend)
				subscribers, err := database.GetEmailSubscribers(svr.Conn)
				if err != nil {
					svr.Log(err, fmt.Sprintf("unable to retrieve subscribers"))
					return
				}
				var jobsHTMLArr []string
				for _, j := range jobPosts {
					jobsHTMLArr = append(jobsHTMLArr, `<p><b>Job Title:</b> `+j.JobTitle+`<br /><b>Company:</b> `+j.Company+`<br /><b>Location:</b> `+j.Location+`<br /><b>Salary:</b> `+j.SalaryRange+`<br /><b>Detail:</b> <a href="https://golang.cafe/job/`+j.Slug+`">https://golang.cafe/job/`+j.Slug+`</a></p>`)
					lastJobID = j.ID
				}
				jobsHTML := strings.Join(jobsHTMLArr, " ")
				campaignContentHTML := `<p>Here's a list of the newest ` + fmt.Sprintf("%d", len(jobPosts)) + ` Go jobs this week on Golang Cafe</p>
` + jobsHTML + `
	<p>Check out more jobs at <a title="Golang Cafe" href="https://golang.cafe">https://golang.cafe</a></p>
	<p>Diego from Golang Cafe</p>
	<hr />`
				unsubscribeLink := `
	<h6><strong> Golang Cafe</strong> | London, United Kingdom<br />This email was sent to <strong>%s</strong> | <a href="https://golang.cafe/x/email/unsubscribe?token=%s">Unsubscribe</a></h6>`

				for _, s := range subscribers {
					err = svr.GetEmail().SendHTMLEmail(
						"Diego from Golang Cafe <team@golang.cafe>",
						s.Email,
						email.GolangCafeEmailAddress,
						fmt.Sprintf("Go Jobs This Week (%d New)", len(jobPosts)),
						campaignContentHTML+fmt.Sprintf(unsubscribeLink, s.Email, s.Token),
					)
					if err != nil {
						svr.Log(err, fmt.Sprintf("unable to send email for newsletter email %s", s.Email))
						continue
					}
				}
				lastJobIDStr = strconv.Itoa(lastJobID)
				err = jobRepo.SetValue("last_sent_job_id_weekly", lastJobIDStr)
				if err != nil {
					svr.Log(err, "unable to save last weekly newsletter job id to db")
					return
				}
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func TriggerTelegramScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				lastTelegramJobIDStr, err := jobRepo.GetValue("last_telegram_job_id")
				if err != nil {
					svr.Log(err, "unable to retrieve last telegram job id")
					return
				}
				lastTelegramJobID, err := strconv.Atoi(lastTelegramJobIDStr)
				if err != nil {
					svr.Log(err, "unable to convert job str to id")
					return
				}
				jobPosts, err := jobRepo.GetLastNJobsFromID(svr.GetConfig().TwitterJobsToPost, lastTelegramJobID)
				log.Printf("found %d/%d jobs to post on telegram\n", len(jobPosts), svr.GetConfig().TwitterJobsToPost)
				if len(jobPosts) == 0 {
					return
				}
				lastJobID := lastTelegramJobID
				api := telegram.New(svr.GetConfig().TelegramAPIToken)
				ctx := context.Background()
				for _, j := range jobPosts {
					_, err := api.SendMessage(ctx, telegram.NewMessage(svr.GetConfig().TelegramChannelID, fmt.Sprintf("%s with %s - %s | %s\n\n#golang #golangjobs\n\nhttps://golang.cafe/job/%s", j.JobTitle, j.Company, j.Location, j.SalaryRange, j.Slug)))
					if err != nil {
						svr.Log(err, "unable to post on telegram")
						continue
					}
					lastJobID = j.ID
				}
				lastJobIDStr := strconv.Itoa(lastJobID)
				err = jobRepo.SetValue("last_telegram_job_id", lastJobIDStr)
				if err != nil {
					svr.Log(err, fmt.Sprintf("unable to save last telegram job id to db as %s", lastJobIDStr))
					return
				}
				log.Printf("updated last telegram job id to %s\n", lastJobIDStr)
				log.Printf("posted last %d jobs to telegram", len(jobPosts))
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func TriggerMonthlyHighlights(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				pageviewsLast30Days, err := database.GetWebsitePageViewsLast30Days(svr.Conn)
				if err != nil {
					svr.Log(err, "could not retrieve pageviews for last 30 days")
					return
				}
				jobPageviewsLast30Days, err := database.GetJobPageViewsLast30Days(svr.Conn)
				if err != nil {
					svr.Log(err, "could not retrieve job pageviews for last 30 days")
					return
				}
				jobApplicantsLast30Days, err := database.GetJobClickoutsLast30Days(svr.Conn)
				if err != nil {
					svr.Log(err, "could not retrieve job clickouts for last 30 days")
					return
				}
				_, newJobsLastMonth, err := jobRepo.NewJobsLastWeekOrMonth()
				if err != nil {
					svr.Log(err, "unable to retrieve new jobs last week last month")
					return
				}
				pageviewsLast30DaysText := humanize.Comma(int64(pageviewsLast30Days))
				jobPageviewsLast30DaysText := humanize.Comma(int64(jobPageviewsLast30Days))
				jobApplicantsLast30DaysText := humanize.Comma(int64(jobApplicantsLast30Days))
				newJobsLastMonthText := humanize.Comma(int64(newJobsLastMonth))
				api := anaconda.NewTwitterApiWithCredentials(svr.GetConfig().TwitterAccessToken, svr.GetConfig().TwitterAccessTokenSecret, svr.GetConfig().TwitterClientKey, svr.GetConfig().TwitterClientSecret)
				highlights := fmt.Sprintf(`This months highlight ✨ 

📣 %s new jobs posted last month
✉️  %s applicants last month
🌎 %s pageviews last month
💼 %s jobs viewed last month

Find your next job on Golang Cafe ⏩ https://golang.cafe 

#go #golang #gojobs`, newJobsLastMonthText, jobApplicantsLast30DaysText, pageviewsLast30DaysText, jobPageviewsLast30DaysText)
				_, err = api.PostTweet(highlights, url.Values{})
				if err != nil {
					svr.Log(err, "unable to post monthly highlight tweet")
					return
				}
				telegramApi := telegram.New(svr.GetConfig().TelegramAPIToken)
				_, err = telegramApi.SendMessage(context.Background(), telegram.NewMessage(svr.GetConfig().TelegramChannelID, highlights))
				if err != nil {
					svr.Log(err, "unable to post on telegram monthly highlights")
					return
				}
				err = svr.GetEmail().SendEmail(
					"Diego from Golang Cafe <team@golang.cafe>",
					email.GolangCafeEmailAddress,
					email.GolangCafeEmailAddress,
					"Golang Cafe Monthly Highlights",
					highlights,
				)
				if err != nil {
					svr.Log(err, "unable to send monthtly highlights email")
					return
				}
			}()
		},
	)
}

func TriggerTwitterScheduler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				lastTwittedJobIDStr, err := jobRepo.GetValue("last_twitted_job_id")
				if err != nil {
					svr.Log(err, "unable to retrieve last twitter job id")
					return
				}
				lastTwittedJobID, err := strconv.Atoi(lastTwittedJobIDStr)
				if err != nil {
					svr.Log(err, "unable to convert job str to id")
					return
				}
				jobPosts, err := jobRepo.GetLastNJobsFromID(svr.GetConfig().TwitterJobsToPost, lastTwittedJobID)
				log.Printf("found %d/%d jobs to post on twitter\n", len(jobPosts), svr.GetConfig().TwitterJobsToPost)
				if len(jobPosts) == 0 {
					return
				}
				lastJobID := lastTwittedJobID
				api := anaconda.NewTwitterApiWithCredentials(svr.GetConfig().TwitterAccessToken, svr.GetConfig().TwitterAccessTokenSecret, svr.GetConfig().TwitterClientKey, svr.GetConfig().TwitterClientSecret)
				for _, j := range jobPosts {
					_, err := api.PostTweet(fmt.Sprintf("%s with %s - %s | %s\n\n#golang #golangjobs\n\nhttps://golang.cafe/job/%s", j.JobTitle, j.Company, j.Location, j.SalaryRange, j.Slug), url.Values{})
					if err != nil {
						svr.Log(err, "unable to post tweet")
						continue
					}
					lastJobID = j.ID
				}
				lastJobIDStr := strconv.Itoa(lastJobID)
				err = jobRepo.SetValue("last_twitted_job_id", lastJobIDStr)
				if err != nil {
					svr.Log(err, fmt.Sprintf("unable to save last twitter job id to db as %s", lastJobIDStr))
					return
				}
				log.Printf("updated last twitted job id to %s\n", lastJobIDStr)
				log.Printf("posted last %d jobs to twitter", len(jobPosts))
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func TriggerCompanyUpdate(svr server.Server, companyRepo *company.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				since := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
				cs, err := companyRepo.InferCompaniesFromJobs(since)
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
					description := doc.Find("title").Text()
					twitter := ""
					doc.Find("meta").Each(func(i int, s *goquery.Selection) {
						if name, _ := s.Attr("name"); strings.EqualFold(name, "description") {
							var ok bool
							desc, ok := s.Attr("content")
							if !ok {
								log.Println("unable to retrieve content for description tag for companyURL ", c.URL)
								return
							}
							if desc != "" {
								description = desc
							}
							log.Printf("description: %s\n", description)
						}
						if name, _ := s.Attr("name"); strings.EqualFold(name, "twitter:site") {
							var ok bool
							twtr, ok := s.Attr("content")
							if !ok {
								log.Println("unable to retrieve content for twitter:site")
								return
							}
							if twtr != "" {
								twitter = "https://twitter.com/" + strings.Trim(twtr, "@")
							}
							log.Printf("twitter: %s\n", twitter)
						}
					})
					github := ""
					linkedin := ""
					doc.Find("a").Each(func(i int, s *goquery.Selection) {
						if href, ok := s.Attr("href"); ok && strings.Contains(href, "github.com/") {
							github = href
							log.Printf("github: %s\n", github)
						}
						if href, ok := s.Attr("href"); ok && strings.Contains(href, "linkedin.com/") {
							linkedin = href
							log.Printf("linkedin: %s\n", linkedin)
						}
						if twitter == "" {
							if href, ok := s.Attr("href"); ok && strings.Contains(href, "twitter.com/") {
								twitter = href
								log.Printf("twitter: %s\n", twitter)
							}
						}
					})
					if description != "" {
						c.Description = &description
					}
					if twitter != "" {
						c.Twitter = &twitter
					}
					if github != "" {
						c.Github = &github
					}
					if linkedin != "" {
						c.Linkedin = &linkedin
					}
					companyID, err := ksuid.NewRandom()
					if err != nil {
						svr.Log(err, "ksuid.NewRandom: companyID")
						continue
					}
					newIconID, err := ksuid.NewRandom()
					if err != nil {
						svr.Log(err, "ksuid.NewRandom: newIconID")
						continue
					}
					if err := database.DuplicateImage(svr.Conn, c.IconImageID, newIconID.String()); err != nil {
						svr.Log(err, "database.DuplicateImage")
						continue
					}
					c.ID = companyID.String()
					c.Slug = slug.Make(c.Name)
					c.IconImageID = newIconID.String()
					if err := companyRepo.SaveCompany(c); err != nil {
						svr.Log(err, "companyRepo.SaveCompany")
						continue
					}
					log.Println(c.Name)
				}
				if err := companyRepo.DeleteStaleImages(); err != nil {
					svr.Log(err, "companyRepo.DeleteStaleImages")
					return
				}
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func TriggerAdsManager(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				log.Printf("attempting to demote expired sponsored 30days pinned job ads\n")
				jobPosts, err := jobRepo.GetJobsOlderThan(time.Now().AddDate(0, 0, -30), job.JobAdSponsoredPinnedFor30Days)
				if err != nil {
					svr.Log(err, "unable to demote expired sponsored 30 days pinned job ads")
					return
				}
				for _, j := range jobPosts {
					jobToken, err := jobRepo.TokenByJobID(j.ID)
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
					jobRepo.UpdateJobAdType(job.JobAdBasic, j.ID)
					log.Printf("demoted job id %d expired sponsored 30days pinned job ads\n", j.ID)
				}

				log.Printf("attempting to demote expired sponsored 7days pinned job ads\n")
				jobPosts2, err := jobRepo.GetJobsOlderThan(time.Now().AddDate(0, 0, -7), job.JobAdSponsoredPinnedFor7Days)
				if err != nil {
					svr.Log(err, "unable to demote expired sponsored 7 days pinned job ads")
					return
				}
				for _, j := range jobPosts2 {
					jobToken, err := jobRepo.TokenByJobID(j.ID)
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
					jobRepo.UpdateJobAdType(job.JobAdBasic, j.ID)
					log.Printf("demoted job id %d expired sponsored 7days pinned job ads\n", j.ID)
				}
				log.Printf("attempting to demote expired sponsored 60days pinned job ads\n")
				jobPosts3, err := jobRepo.GetJobsOlderThan(time.Now().AddDate(0, 0, -60), job.JobAdSponsoredPinnedFor60Days)
				if err != nil {
					svr.Log(err, "unable to demote expired sponsored 7 days pinned job ads")
					return
				}
				for _, j := range jobPosts3 {
					jobToken, err := jobRepo.TokenByJobID(j.ID)
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
					jobRepo.UpdateJobAdType(job.JobAdBasic, j.ID)
					log.Printf("demoted job id %d expired sponsored 60days pinned job ads\n", j.ID)
				}
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func UpdateDeveloperProfileHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
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
			if !svr.IsEmail(req.Email) {
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
			dev := developer.Developer{
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
			err = devRepo.UpdateDeveloperProfile(dev)
			if err != nil {
				svr.Log(err, "unable to update developer profile")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func DeleteDeveloperProfileHandler(svr server.Server, devRepo *developer.Repository, userRepo *user.Repository) http.HandlerFunc {
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
			if !svr.IsEmail(req.Email) {
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
			err = devRepo.DeleteDeveloperProfile(req.ID, req.Email)
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
			if userErr := userRepo.DeleteUserByEmail(req.Email); userErr != nil {
				svr.Log(err, "unable to delete user by email "+req.Email)
				svr.JSON(w, http.StatusInternalServerError, nil)
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func ConfirmEmailSubscriberHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		token := vars["token"]
		err := database.ConfirmEmailSubscriber(svr.Conn, token)
		if err != nil {
			svr.Log(err, "unable to confirm subscriber using token "+token)
			svr.TEXT(w, http.StatusInternalServerError, "There was an error with your request. Please try again later.")
			return
		}
		svr.TEXT(w, http.StatusOK, "Your email subscription has been confirmed successfully.")
	}
}

func RemoveEmailSubscriberHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := database.RemoveEmailSubscriber(svr.Conn, r.URL.Query().Get("token"))
		if err != nil {
			svr.Log(err, "unable to add email subscriber to db")
			svr.TEXT(w, http.StatusInternalServerError, "")
			return
		}
		svr.TEXT(w, http.StatusOK, "Your email has been successfully removed.")
	}
}

func AddEmailSubscriberHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := strings.ToLower(r.URL.Query().Get("email"))
		if !svr.IsEmail(email) {
			svr.Log(errors.New("invalid email"), "request email is not a valid email")
			svr.JSON(w, http.StatusBadRequest, "invalid email provided")
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate email subscriber token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err = database.AddEmailSubscriber(svr.Conn, email, k.String())
		if err != nil {
			svr.Log(err, "unable to add email subscriber to db")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		err = svr.GetEmail().SendEmail(
			"Diego from Golang Cafe <team@golang.cafe>",
			email,
			"",
			"Confirm Your Email Subscription on Golang Cafe",
			fmt.Sprintf(
				"Please click on the link below to confirm your subscription to receive weekly emails from Golang Cafe\n\n%s\n\nIf this was not requested by you, please ignore this email.",
				fmt.Sprintf("https://golang.cafe/x/email/confirm/%s", k.String()),
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

func SendMessageDeveloperProfileHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		profileID := vars["id"]
		req := &struct {
			Content string `json:"content"`
			Email   string `json:"email"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			reqData, ioErr := ioutil.ReadAll(r.Body)
			if ioErr != nil {
				svr.Log(ioErr, "unable to read request body data for developer profile message")
			}
			svr.Log(err, fmt.Sprintf("unable to decode request body from developer profile message %+v", string(reqData)))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		if !svr.IsEmail(req.Email) {
			svr.Log(errors.New("invalid email"), "request email is not a valid email")
			svr.JSON(w, http.StatusBadRequest, "invalid email provided")
			return
		}
		dev, err := devRepo.DeveloperProfileByID(profileID)
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
		message := developer.DeveloperMessage{
			ID:        k.String(),
			Email:     req.Email,
			Content:   req.Content,
			ProfileID: dev.ID,
		}
		err = devRepo.SendMessageDeveloperProfile(message)
		if err != nil {
			svr.Log(err, "unable to send message to developer profile")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		err = svr.GetEmail().SendEmail(
			"Diego from Golang Cafe <team@golang.cafe>",
			req.Email,
			dev.Email,
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
		if err := devRepo.TrackDeveloperProfileMessageSent(dev); err != nil {
			svr.Log(err, "unable to track message sent to developer profile")
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func AutocompleteLocation(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("k")
		locs, err := database.LocationsByPrefix(svr.Conn, prefix)
		if err != nil {
			svr.Log(err, "unable to retrieve locations by prefix")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.JSON(w, http.StatusOK, locs)
	}
}

func AutocompleteSkill(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := r.URL.Query().Get("k")
		skills, err := database.SkillsByPrefix(svr.Conn, prefix)
		if err != nil {
			svr.Log(err, "unable to retrieve skills by prefix")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.JSON(w, http.StatusOK, skills)
	}
}

func DeliverMessageDeveloperProfileHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		messageID := vars["id"]
		message, email, err := devRepo.MessageForDeliveryByID(messageID)
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
		if err := devRepo.MarkDeveloperMessageAsSent(messageID); err != nil {
			svr.Log(err, "unable to mark developer message as sent "+messageID)
		}
		svr.JSON(w, http.StatusOK, "Message Sent Successfully")
	}
}

func EditDeveloperProfileHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
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
			dev, err := devRepo.DeveloperProfileByID(profileID)
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

func ViewDeveloperProfileHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		profileSlug := vars["slug"]
		dev, err := devRepo.DeveloperProfileBySlug(profileSlug)
		if err != nil {
			svr.Log(err, "unable to find developer profile by slug "+profileSlug)
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		if err := devRepo.TrackDeveloperProfileView(dev); err != nil {
			svr.Log(err, "unable to track developer profile view")
		}
		dev.UpdatedAtHumanized = dev.UpdatedAt.UTC().Format("January 2006")
		dev.SkillsArray = strings.Split(dev.Skills, ",")
		svr.Render(w, http.StatusOK, "view-developer-profile.html", map[string]interface{}{
			"DeveloperProfile": dev,
			"MonthAndYear":     time.Now().UTC().Format("January 2006"),
		})
	}
}

func CompaniesForLocationHandler(svr server.Server, companyRepo *company.Repository, jobRepo *job.Repository, loc string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("p")
		svr.RenderPageForCompanies(w, r, companyRepo, jobRepo, loc, page, "companies.html")
	}
}

func IndexPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
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
			return
		}
		vars := mux.Vars(r)
		salary := vars["salary"]
		currency := vars["currency"]
		location = vars["location"]
		tag = vars["tag"]
		var validSalary bool
		for _, band := range svr.GetConfig().AvailableSalaryBands {
			if fmt.Sprintf("%d", band) == salary {
				validSalary = true
				break
			}
		}
		dst = "/"
		if location != "" && tag != "" {
			dst = fmt.Sprintf("/Golang-%s-Jobs-In-%s", tag, location)
		} else if location != "" {
			dst = fmt.Sprintf("/Golang-Jobs-In-%s", location)
		} else if tag != "" {
			dst = fmt.Sprintf("/Golang-%s-Jobs", tag)
		}
		if page != "" {
			dst += fmt.Sprintf("?p=%s", page)
		}
		if (salary != "" && !validSalary) || (currency != "" && currency != "USD") {
			svr.Redirect(w, r, http.StatusMovedPermanently, dst)
			return
		}

		svr.RenderPageForLocationAndTag(w, r, jobRepo, "", "", page, salary, currency, "landing.html")
	}
}

func PermanentRedirectHandler(svr server.Server, dst string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("https://golang.cafe/%s", dst))
	}
}

func PermanentExternalRedirectHandler(svr server.Server, dst string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Redirect(w, r, http.StatusMovedPermanently, dst)
	}
}

func PostAJobPageHandler(svr server.Server, companyRepo *company.Repository, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPostAJobForLocation(w, r, companyRepo, jobRepo, "")
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
		if amount < 900 || amount > 19900 {
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
		if !svr.IsEmail(req.Email) {
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

func RequestTokenSignOn(svr server.Server, userRepo *user.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next := r.URL.Query().Get("next")
		req := &struct {
			Email string `json:"email"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		if !svr.IsEmail(req.Email) {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err = userRepo.SaveTokenSignOn(req.Email, k.String())
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

func VerifyTokenSignOn(svr server.Server, userRepo *user.Repository, devRepo *developer.Repository, adminEmail string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		token := vars["token"]
		user, _, err := userRepo.GetOrCreateUserFromToken(token)
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
			if activateDevProfileErr := devRepo.ActivateDeveloperProfile(user.Email); activateDevProfileErr != nil {
				svr.Log(err, "unable to activate developer profile")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			dev, err := devRepo.DeveloperProfileByEmail(user.Email)
			if err != nil {
				svr.Log(err, "unable to find developer profile by email")
				svr.JSON(w, http.StatusNotFound, "unable to find developer profile by email")
				return
			}
			svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("/edit/profile/%s", dev.ID))
			return
		case AuthStepLoginDeveloperProfile == next:
			dev, err := devRepo.DeveloperProfileByEmail(user.Email)
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

func ListJobsAsAdminPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			loc := r.URL.Query().Get("l")
			skill := r.URL.Query().Get("s")
			page := r.URL.Query().Get("p")
			salary := ""
			currency := "USD"
			svr.RenderPageForLocationAndTagAdmin(w, jobRepo, loc, skill, page, salary, currency, "list-jobs-admin.html")
		},
	)
}

func PostAJobForLocationPageHandler(svr server.Server, companyRepo *company.Repository, jobRepo *job.Repository, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPostAJobForLocation(w, r, companyRepo, jobRepo, location)
	}
}

func PostAJobForLocationFromURLPageHandler(svr server.Server, companyRepo *company.Repository, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		location := vars["location"]
		location = strings.ReplaceAll(location, "-", " ")
		reg, err := regexp.Compile("[^a-zA-Z0-9\\s]+")
		if err != nil {
			log.Fatal(err)
		}
		location = reg.ReplaceAllString(location, "")
		svr.RenderPostAJobForLocation(w, r, companyRepo, jobRepo, location)
	}
}

func JobBySlugPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		slug := vars["slug"]
		location := vars["l"]
		jobPost, err := jobRepo.JobPostBySlug(slug)
		if err != nil || jobPost == nil {
			svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job golang.cafe/job/%s not found", slug))
			return
		}
		if err := jobRepo.TrackJobView(jobPost); err != nil {
			svr.Log(err, fmt.Sprintf("unable to track job view for %s: %v", slug, err))
		}
		jobLocations := strings.Split(jobPost.Location, "/")
		var isQuickApply bool
		if svr.IsEmail(jobPost.HowToApply) {
			isQuickApply = true
		}
		jobPost.SalaryRange = fmt.Sprintf("%s%s to %s%s", jobPost.SalaryCurrency, humanize.Comma(jobPost.SalaryMin), jobPost.SalaryCurrency, humanize.Comma(jobPost.SalaryMax))

		relevantJobs, err := jobRepo.GetRelevantJobs(jobPost.Location, jobPost.ID, 3)
		if err != nil {
			svr.Log(err, "unable to get relevant jobs")
		}
		for i, j := range relevantJobs {
			relevantJobs[i].CompanyURLEnc = url.PathEscape(j.Company)
			relevantJobs[i].JobDescription = string(svr.MarkdownToHTML(j.JobDescription))
			relevantJobs[i].Perks = string(svr.MarkdownToHTML(j.Perks))
			relevantJobs[i].SalaryRange = fmt.Sprintf("%s%s to %s%s", j.SalaryCurrency, humanize.Comma(j.SalaryMin), j.SalaryCurrency, humanize.Comma(j.SalaryMax))
			relevantJobs[i].InterviewProcess = string(svr.MarkdownToHTML(j.InterviewProcess))
			if svr.IsEmail(j.HowToApply) {
				relevantJobs[i].IsQuickApply = true
			}
		}
		svr.Render(w, http.StatusOK, "job.html", map[string]interface{}{
			"Job":                     jobPost,
			"JobURIEncoded":           url.QueryEscape(jobPost.Slug),
			"IsQuickApply":            isQuickApply,
			"HTMLJobDescription":      svr.MarkdownToHTML(jobPost.JobDescription),
			"HTMLJobPerks":            svr.MarkdownToHTML(jobPost.Perks),
			"HTMLJobInterviewProcess": svr.MarkdownToHTML(jobPost.InterviewProcess),
			"LocationFilter":          location,
			"ExternalJobId":           jobPost.ExternalID,
			"MonthAndYear":            time.Unix(jobPost.CreatedAt, 0).UTC().Format("January 2006"),
			"GoogleJobCreatedAt":      time.Unix(jobPost.CreatedAt, 0).Format(time.RFC3339),
			"GoogleJobValidThrough":   time.Unix(jobPost.CreatedAt, 0).AddDate(0, 5, 0),
			"GoogleJobLocation":       jobLocations[0],
			"GoogleJobDescription":    strconv.Quote(strings.ReplaceAll(string(svr.MarkdownToHTML(jobPost.JobDescription)), "\n", "")),
			"RelevantJobs":            relevantJobs,
		})
	}
}

func CompanyBySlugPageHandler(svr server.Server, companyRepo *company.Repository, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		slug := vars["slug"]
		company, err := companyRepo.CompanyBySlug(slug)
		if err != nil || company == nil {
			svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Company golang.cafe/job/%s not found", slug))
			return
		}
		if err := companyRepo.TrackCompanyView(company); err != nil {
			svr.Log(err, fmt.Sprintf("unable to track company view for %s: %v", slug, err))
		}
		companyJobs, err := jobRepo.GetCompanyJobs(company.Name, 3)
		if err != nil {
			svr.Log(err, "unable to get company jobs")
		}
		for i, j := range companyJobs {
			companyJobs[i].CompanyURLEnc = url.PathEscape(j.Company)
			companyJobs[i].JobDescription = string(svr.MarkdownToHTML(j.JobDescription))
			companyJobs[i].Perks = string(svr.MarkdownToHTML(j.Perks))
			companyJobs[i].SalaryRange = fmt.Sprintf("%s%s to %s%s", j.SalaryCurrency, humanize.Comma(j.SalaryMin), j.SalaryCurrency, humanize.Comma(j.SalaryMax))
			companyJobs[i].InterviewProcess = string(svr.MarkdownToHTML(j.InterviewProcess))
			if svr.IsEmail(j.HowToApply) {
				companyJobs[i].IsQuickApply = true
			}
		}
		if err := svr.Render(w, http.StatusOK, "company.html", map[string]interface{}{
			"Company":      company,
			"MonthAndYear": time.Now().UTC().Format("January 2006"),
			"CompanyJobs":  companyJobs,
		}); err != nil {
			svr.Log(err, "unable to render template")
		}
	}
}

func LandingPageForLocationHandler(svr server.Server, jobRepo *job.Repository, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		salary := vars["salary"]
		currency := vars["currency"]
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, jobRepo, location, "", page, salary, currency, "landing.html")
	}
}

func LandingPageForLocationAndSkillPlaceholderHandler(svr server.Server, jobRepo *job.Repository, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		salary := vars["salary"]
		currency := vars["currency"]
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, jobRepo, location, skill, page, salary, currency, "landing.html")
	}
}

func LandingPageForLocationPlaceholderHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		salary := vars["salary"]
		currency := vars["currency"]
		loc := strings.ReplaceAll(vars["location"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, jobRepo, loc, "", page, salary, currency, "landing.html")
	}
}

func LandingPageForSkillPlaceholderHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		salary := vars["salary"]
		currency := vars["currency"]
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, jobRepo, "", skill, page, salary, currency, "landing.html")
	}
}

func LandingPageForSkillAndLocationPlaceholderHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		salary := vars["salary"]
		currency := vars["currency"]
		loc := strings.ReplaceAll(vars["location"], "-", " ")
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, jobRepo, loc, skill, page, salary, currency, "landing.html")
	}
}

func ServeRSSFeed(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobPosts, err := jobRepo.GetLastNJobs(20, r.URL.Query().Get("l"))
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

		for _, j := range jobPosts {
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

func StripePaymentConfirmationWebookHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
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
			jobPost, err := jobRepo.GetJobByStripeSessionID(sess.ID)
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
			jobToken, err := jobRepo.TokenByJobID(jobPost.ID)
			if err != nil {
				svr.Log(errors.New("unable to find token for job id"), fmt.Sprintf("session id %s job id %d", sess.ID, jobPost.ID))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			if jobPost.ApprovedAt != nil && jobPost.AdType != job.JobAdSponsoredPinnedFor30Days && jobPost.AdType != job.JobAdSponsoredPinnedFor7Days && (purchaseEvent.AdType == job.JobAdSponsoredPinnedFor7Days || jobPost.AdType != job.JobAdSponsoredPinnedFor30Days) {
				err := jobRepo.UpdateJobAdType(purchaseEvent.AdType, jobPost.ID)
				if err != nil {
					svr.Log(errors.New("unable to update job to new ad type"), fmt.Sprintf("unable to update job id %d to new ad type %d for session id %s", jobPost.ID, purchaseEvent.AdType, sess.ID))
					svr.JSON(w, http.StatusBadRequest, nil)
					return
				}
				err = svr.GetEmail().SendEmail("Diego from Golang Cafe <team@golang.cafe>", purchaseEvent.Email, email.GolangCafeEmailAddress, "Your Job Ad on Golang Cafe", fmt.Sprintf("Your Job Ad has been upgraded successfully and it's now pinned to the home page. You can edit the Job Ad at any time and check page views and clickouts by following this link https://golang.cafe/edit/%s", jobToken))
				if err != nil {
					svr.Log(err, "unable to send email while upgrading job ad")
				}
				if err := svr.CacheDelete(server.CacheKeyPinnedJobs); err != nil {
					svr.Log(err, "unable to cleanup cache after approving job")
				}
			}
			svr.JSON(w, http.StatusOK, nil)
			return
		}

		svr.JSON(w, http.StatusOK, nil)
	}
}

func BlogListHandler(svr server.Server, blogDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		posts, err := seo.BlogPages(blogDir)
		if err != nil {
			svr.TEXT(w, http.StatusInternalServerError, "internal error. please try again later")
			return
		}
		svr.Render(w, http.StatusOK, "blog.html", map[string]interface{}{
			"Posts":        posts,
			"MonthAndYear": time.Now().UTC().Format("January 2006"),
		})
	}
}

func SitemapIndexHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		index := sitemap.NewSitemapIndex()
		entries, err := database.GetSitemapIndex(svr.Conn)
		if err != nil {
			svr.Log(err, "database.GetSitemapIndex")
			svr.TEXT(w, http.StatusInternalServerError, "unable to fetch sitemap")
			return
		}
		for _, e := range entries {
			index.Add(&sitemap.URL{
				Loc:     e.Loc,
				LastMod: &e.LastMod,
			})
		}
		buf := new(bytes.Buffer)
		if _, err := index.WriteTo(buf); err != nil {
			svr.Log(err, "sitemapIndex.WriteTo")
			svr.TEXT(w, http.StatusInternalServerError, "unable to save sitemap index")
			return
		}
		svr.XML(w, http.StatusOK, buf.Bytes())
	}
}

func SitemapHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		sitemapNo := vars["number"]
		number, err := strconv.Atoi(sitemapNo)
		if err != nil || number < 1 {
			svr.Log(err, fmt.Sprintf("unable to parse sitemap number %s", sitemapNo))
			svr.TEXT(w, http.StatusBadRequest, "invalid sitemap number")
			return
		}
		entries, err := database.GetSitemapNo(svr.Conn, number)
		if err != nil {
			svr.Log(err, fmt.Sprintf("database.GetSitemapNo %d", number))
			svr.TEXT(w, http.StatusInternalServerError, "unable to fetch sitemap")
			return
		}
		sitemapFile := sitemap.New()
		for _, e := range entries {
			sitemapFile.Add(&sitemap.URL{
				Loc:        e.Loc,
				LastMod:    &e.LastMod,
				ChangeFreq: sitemap.ChangeFreq(e.ChangeFreq),
			})
		}
		buf := new(bytes.Buffer)
		if _, err := sitemapFile.WriteTo(buf); err != nil {
			svr.Log(err, fmt.Sprintf("sitemapFile.WriteTo %d", number))
			svr.TEXT(w, http.StatusInternalServerError, "unable to save sitemap file")
			return
		}
		svr.XML(w, http.StatusOK, buf.Bytes())
	}
}

func RobotsTxtHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/robots.txt")
}

func WellKnownSecurityHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/security.txt")
}

func AboutPageHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/views/about.html")
}

func PrivacyPolicyPageHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/views/privacy-policy.html")
}

func TermsOfServicePageHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/views/terms-of-service.html")
}

func SalaryLandingPageLocationPlaceholderHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		location := strings.ReplaceAll(vars["location"], "-", " ")
		svr.RenderSalaryForLocation(w, r, jobRepo, location)
	}
}

func SalaryLandingPageLocationHandler(svr server.Server, jobRepo *job.Repository, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderSalaryForLocation(w, r, jobRepo, location)
	}
}

func ViewNewsletterPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, r, jobRepo, "", "", "", "", "", "newsletter.html")
	}
}

func ViewCommunityNewsletterPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, r, jobRepo, "", "", "", "", "", "news.html")
	}
}

func DisableDirListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ViewSupportPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, r, jobRepo, "", "", "", "", "", "support.html")
	}
}

var allowedMediaTypes = []string{"image/png", "image/jpeg", "image/jpg"}

func GenerateKsuIDPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := ksuid.NewRandom()
		if err != nil {
			svr.Render(w, http.StatusInternalServerError, "ksuid.html", map[string]string{"KSUID": ""})
			return
		}
		svr.Render(w, http.StatusOK, "ksuid.html", map[string]string{"KSUID": id.String()})
	}
}

func IPAddressLookup(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ipAddress := strings.Split(r.Header.Get("x-forwarded-for"), ", ")
		if len(ipAddress) < 1 {
			svr.Render(w, http.StatusInternalServerError, "whats-my-ip.html", map[string]string{"IPAddress": ""})
			return
		}
		svr.Render(w, http.StatusOK, "whats-my-ip.html", map[string]string{"IPAddress": ipAddress[0]})
	}
}

func DNSCheckerPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(w, http.StatusOK, "dns-checker.html", nil)
	}
}

func DNSChecker(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dnsType := r.URL.Query().Get("t")
		dnsHost := r.URL.Query().Get("h")
		switch dnsType {
		case "A":
			res, err := net.LookupIP(dnsHost)
			if err != nil || len(res) == 0 {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve A record")
				return
			}
			var buffer bytes.Buffer
			for _, ip := range res {
				buffer.WriteString(fmt.Sprintf("%s\n", ip.String()))
			}
			svr.TEXT(w, http.StatusOK, buffer.String())
		case "PTR":
			res, err := net.LookupAddr(dnsHost)
			if err != nil || len(res) == 0 {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve PTR record")
				return
			}
			var buffer bytes.Buffer
			for _, ptr := range res {
				buffer.WriteString(fmt.Sprintf("%s\n", ptr))
			}
			svr.TEXT(w, http.StatusOK, buffer.String())
		case "MX":
			res, err := net.LookupMX(dnsHost)
			if err != nil {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve CNAME record")
				return
			}
			var buffer bytes.Buffer
			for _, m := range res {
				buffer.WriteString(fmt.Sprintf("%s %v\n", m.Host, m.Pref))
			}
			svr.TEXT(w, http.StatusOK, buffer.String())
		case "CNAME":
			res, err := net.LookupCNAME(dnsHost)
			if err != nil {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve CNAME record")
				return
			}
			svr.TEXT(w, http.StatusOK, res)
		case "NS":
			res, err := net.LookupNS(dnsHost)
			if err != nil || len(res) == 0 {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve NS record")
				return
			}
			var buffer bytes.Buffer
			for _, ns := range res {
				buffer.WriteString(fmt.Sprintf("%s\n", ns.Host))
			}
			svr.TEXT(w, http.StatusOK, buffer.String())
		case "TXT":
			res, err := net.LookupTXT(dnsHost)
			if err != nil || len(res) == 0 {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve TXT record")
				return
			}
			var buffer bytes.Buffer
			for _, t := range res {
				buffer.WriteString(fmt.Sprintf("%s\n", t))
			}
			svr.TEXT(w, http.StatusOK, buffer.String())
		default:
			svr.TEXT(w, http.StatusInternalServerError, "invalid dns record type")
		}

	}
}

func PostAJobSuccessPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(w, http.StatusOK, "post-a-job-success.html", nil)
	}
}

func PostAJobFailurePageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(w, http.StatusOK, "post-a-job-error.html", nil)
	}
}

func ApplyForJobPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// limits upload form size to 5mb
		maxPdfSize := 5 * 1024 * 1024
		r.Body = http.MaxBytesReader(w, r.Body, int64(maxPdfSize))
		cv, header, err := r.FormFile("cv")
		if err != nil {
			svr.Log(err, "unable to read cv file")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		defer cv.Close()
		fileBytes, err := ioutil.ReadAll(cv)
		if err != nil {
			svr.Log(err, "unable to read cv file content")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		contentType := http.DetectContentType(fileBytes)
		if contentType != "application/pdf" {
			svr.Log(errors.New("PDF file is not application/pdf"), fmt.Sprintf("PDF file is not application/pdf got %s", contentType))
			svr.JSON(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		if header.Size > int64(maxPdfSize) {
			svr.Log(errors.New("PDF file is too large"), fmt.Sprintf("PDF file too large: %d > %d", header.Size, maxPdfSize))
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		externalID := r.FormValue("job-id")
		emailAddr := r.FormValue("email")
		jobPost, err := jobRepo.JobPostByExternalIDForEdit(externalID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job by externalId %d, %v", externalID, err))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		randomToken, err := k.Value()
		if err != nil {
			svr.Log(err, "unable to get token value")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		randomTokenStr, ok := randomToken.(string)
		if !ok {
			svr.Log(err, "unable to assert token value as string")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err = jobRepo.ApplyToJob(jobPost.ID, fileBytes, emailAddr, randomTokenStr)
		if err != nil {
			svr.Log(err, "unable to apply for job while saving to db")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err = svr.GetEmail().SendEmail("Diego from Golang Cafe <team@golang.cafe>", emailAddr, email.GolangCafeEmailAddress, fmt.Sprintf("Confirm your job application with %s", jobPost.Company), fmt.Sprintf("Thanks for applying for the position %s with %s - %s (https://golang.cafe/job/%s). You application request, your email and your CV will expire in 72 hours and will be permanently deleted from the system. Please confirm your application now by following this link https://golang.cafe/apply/%s", jobPost.JobTitle, jobPost.Company, jobPost.Location, jobPost.Slug, randomTokenStr))
		if err != nil {
			svr.Log(err, "unable to send email while applying to job")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		if r.FormValue("notify-jobs") == "true" {
			k, err := ksuid.NewRandom()
			if err != nil {
				svr.Log(err, "unable to generate email subscriber token")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			err = database.AddEmailSubscriber(svr.Conn, emailAddr, k.String())
			if err != nil {
				svr.Log(err, "unable to add email subscriber to db")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			err = svr.GetEmail().SendEmail(
				"Diego from Golang Cafe <team@golang.cafe>",
				emailAddr,
				"",
				"Confirm Your Email Subscription on Golang Cafe",
				fmt.Sprintf(
					"Please click on the link below to confirm your subscription to receive weekly emails from Golang Cafe\n\n%s\n\nIf this was not requested by you, please ignore this email.",
					fmt.Sprintf("https://golang.cafe/x/email/confirm/%s", k.String()),
				),
			)
			if err != nil {
				svr.Log(err, "unable to send email while submitting message")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func ApplyToJobConfirmation(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		token := vars["token"]
		jobPost, applicant, err := jobRepo.GetJobByApplyToken(token)
		if err != nil {
			svr.Render(w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Invalid Job Application",
				"Description": "Oops, seems like the application you are trying to complete is no longer valid. Your application request may be expired or simply the company may not be longer accepting applications.",
			})
			return
		}
		err = svr.GetEmail().SendEmailWithPDFAttachment("Diego from Golang Cafe <team@golang.cafe>", jobPost.HowToApply, applicant.Email, "New Applicant from Golang Cafe", fmt.Sprintf("Hi, there is a new applicant for your position on Golang Cafe: %s with %s - %s (https://golang.cafe/job/%s). Applicant's Email: %s. Please find applicant's CV attached below", jobPost.JobTitle, jobPost.Company, jobPost.Location, jobPost.Slug, applicant.Email), applicant.Cv, "cv.pdf")
		if err != nil {
			svr.Log(err, "unable to send email while applying to job")
			svr.Render(w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Job Application Failure",
				"Description": "Oops, there was a problem while completing yuor application. Please try again later. If the problem persists, please contact team@golang.cafe",
			})
			return
		}
		err = jobRepo.ConfirmApplyToJob(token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to update apply_token with successfull application for token %s", token))
			svr.Render(w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Job Application Failure",
				"Description": "Oops, there was a problem while completing yuor application. Please try again later. If the problem persists, please contact team@golang.cafe",
			})
			return
		}
		svr.Render(w, http.StatusOK, "apply-message.html", map[string]interface{}{
			"Title":       "Job Application Successfull",
			"Description": svr.StringToHTML(fmt.Sprintf("Thank you for applying for <b>%s with %s - %s</b><br /><a href=\"https://golang.cafe/job/%s\">https://golang.cafe/job/%s</a>. <br /><br />Your CV has been forwarded to company HR. If you have further questions please reach out to <code>%s</code>. Please note, your email and CV have been permanently deleted from our systems.", jobPost.JobTitle, jobPost.Company, jobPost.Location, jobPost.Slug, jobPost.Slug, jobPost.HowToApply)),
		})
	}
}

func SubmitJobPostWithoutPaymentHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			jobRq := &job.JobRq{}
			if err := decoder.Decode(&jobRq); err != nil {
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			jobID, err := jobRepo.SaveDraft(jobRq)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to save job request: %#v", jobRq))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			k, err := ksuid.NewRandom()
			if err != nil {
				svr.Log(err, "unable to generate unique token")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			randomToken, err := k.Value()
			if err != nil {
				svr.Log(err, "unable to get token value")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			randomTokenStr, ok := randomToken.(string)
			if !ok {
				svr.Log(err, "unbale to assert token value as string")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			err = jobRepo.SaveTokenForJob(randomTokenStr, jobID)
			if err != nil {
				svr.Log(err, "unable to generate token")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			svr.JSON(w, http.StatusOK, map[string]interface{}{"token": randomTokenStr})
			return
		},
	)
}

func SubmitJobPostPaymentUpsellPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		jobRq := &job.JobRqUpsell{}
		if err := decoder.Decode(&jobRq); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		// validate currency
		if jobRq.CurrencyCode != "USD" && jobRq.CurrencyCode != "EUR" && jobRq.CurrencyCode != "GBP" {
			jobRq.CurrencyCode = "USD"
		}
		jobID, err := jobRepo.JobPostIDByToken(jobRq.Token)
		if err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		sess, err := payment.CreateSession(svr.GetConfig().StripeKey, &job.JobRq{AdType: jobRq.AdType, CurrencyCode: jobRq.CurrencyCode, Email: jobRq.Email}, jobRq.Token)
		if err != nil {
			svr.Log(err, "unable to create payment session")
		}

		err = svr.GetEmail().SendEmail("Diego from Golang Cafe <team@golang.cafe>", email.GolangCafeEmailAddress, jobRq.Email, "New Upgrade on Golang Cafe", fmt.Sprintf("Hey! There is a new ad upgrade on Golang Cafe. Please check https://golang.cafe/manage/%s", jobRq.Token))
		if err != nil {
			svr.Log(err, "unable to send email to admin while upgrading job ad")
		}
		if sess != nil {
			err = database.InitiatePaymentEvent(svr.Conn, sess.ID, payment.AdTypeToAmount(jobRq.AdType), jobRq.CurrencyCode, payment.AdTypeToDescription(jobRq.AdType), jobRq.AdType, jobRq.Email, jobID)
			if err != nil {
				svr.Log(err, "unable to save payment initiated event")
			}
			svr.JSON(w, http.StatusOK, map[string]string{"s_id": sess.ID})
			return
		}
		svr.JSON(w, http.StatusOK, nil)
		return
	}
}

func GeneratePaymentIntent(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		req := struct {
			Email    string
			Currency string
			Amount   int
		}{}
		if err := dec.Decode(&req); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			fmt.Println("invalid req")
			return
		}
		if req.Currency != "USD" && req.Currency != "EUR" && req.Currency != "GBP" {
			fmt.Println("invalid cur")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		sess, err := payment.CreateGenericSession(svr.GetConfig().StripeKey, req.Email, req.Currency, req.Amount)
		if err != nil {
			fmt.Println("invalid sess")
			svr.Log(err, "unable to create payment session")
		}
		if sess != nil {
			fmt.Println("invalid req")
			svr.JSON(w, http.StatusOK, map[string]string{"s_id": sess.ID})
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func SubmitJobPostPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		jobRq := &job.JobRq{}
		if err := decoder.Decode(&jobRq); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		// validate currency
		if jobRq.CurrencyCode != "USD" && jobRq.CurrencyCode != "EUR" && jobRq.CurrencyCode != "GBP" {
			jobRq.CurrencyCode = "USD"
		}
		jobID, err := jobRepo.SaveDraft(jobRq)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to save job request: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate unique token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		randomToken, err := k.Value()
		if err != nil {
			svr.Log(err, "unable to get token value")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		randomTokenStr, ok := randomToken.(string)
		if !ok {
			svr.Log(err, "unbale to assert token value as string")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err = jobRepo.SaveTokenForJob(randomTokenStr, jobID)
		if err != nil {
			svr.Log(err, "unbale to generate token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		sess, err := payment.CreateSession(svr.GetConfig().StripeKey, jobRq, randomTokenStr)
		if err != nil {
			svr.Log(err, "unable to create payment session")
		}
		approveMsg := fmt.Sprintf("Hey! There is a new Ad on Golang Cafe. Please approve https://golang.cafe/manage/%s", randomTokenStr)
		err = svr.GetEmail().SendEmail("Diego from Golang Cafe <team@golang.cafe>", email.GolangCafeEmailAddress, jobRq.Email, "New Job Ad on Golang Cafe", approveMsg)
		if err != nil {
			svr.Log(err, "unable to send email to admin while posting job ad")
		}
		if sess != nil {
			err = database.InitiatePaymentEvent(svr.Conn, sess.ID, payment.AdTypeToAmount(jobRq.AdType), jobRq.CurrencyCode, payment.AdTypeToDescription(jobRq.AdType), jobRq.AdType, jobRq.Email, jobID)
			if err != nil {
				svr.Log(err, "unable to save payment initiated event")
			}
			svr.JSON(w, http.StatusOK, map[string]string{"s_id": sess.ID})
			return
		}
		svr.JSON(w, http.StatusOK, nil)
		return
	}
}

func RetrieveMediaPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		mediaID := vars["id"]
		media, err := database.GetMediaByID(svr.Conn, mediaID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve media by ID: '%s'", mediaID))
			svr.MEDIA(w, http.StatusNotFound, media.Bytes, media.MediaType)
			return
		}
		height := r.URL.Query().Get("h")
		width := r.URL.Query().Get("w")
		if height == "" && width == "" {
			svr.MEDIA(w, http.StatusOK, media.Bytes, media.MediaType)
			return
		}
		he, err := strconv.Atoi(height)
		if err != nil {
			svr.MEDIA(w, http.StatusOK, media.Bytes, media.MediaType)
			return
		}
		wi, err := strconv.Atoi(width)
		if err != nil {
			svr.MEDIA(w, http.StatusOK, media.Bytes, media.MediaType)
			return
		}
		contentTypeInvalid := true
		for _, allowedMedia := range allowedMediaTypes {
			if allowedMedia == media.MediaType {
				contentTypeInvalid = false
			}
		}
		if contentTypeInvalid {
			svr.Log(errors.New("invalid media content type"), fmt.Sprintf("media file %s is not one of the allowed media types: %+v", media.MediaType, allowedMediaTypes))
			svr.JSON(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		decImage, _, err := image.Decode(bytes.NewReader(media.Bytes))
		if err != nil {
			svr.Log(err, "unable to decode image from bytes")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		m := resize.Resize(uint(wi), uint(he), decImage, resize.Lanczos3)
		resizeImageBuf := new(bytes.Buffer)
		switch media.MediaType {
		case "image/jpg", "image/jpeg":
			if err := jpeg.Encode(resizeImageBuf, m, nil); err != nil {
				svr.Log(err, "unable to encode resizeImage into jpeg")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		case "image/png":
			if err := png.Encode(resizeImageBuf, m); err != nil {
				svr.Log(err, "unable to encode resizeImage into png")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		default:
			svr.MEDIA(w, http.StatusOK, media.Bytes, media.MediaType)
			return
		}
		svr.MEDIA(w, http.StatusOK, resizeImageBuf.Bytes(), media.MediaType)
	}
}

func RetrieveMediaMetaPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		jobID := vars["id"]
		job, err := jobRepo.GetJobByExternalID(jobID)
		if err != nil {
			svr.Log(err, "unable to retrieve job by external ID")
			svr.MEDIA(w, http.StatusNotFound, []byte{}, "image/png")
			return
		}
		media, err := imagemeta.GenerateImageForJob(job)
		mediaBytes, err := ioutil.ReadAll(media)
		if err != nil {
			svr.Log(err, "unable to generate media for job ID")
			svr.MEDIA(w, http.StatusNotFound, mediaBytes, "image/png")
			return
		}
		svr.MEDIA(w, http.StatusOK, mediaBytes, "image/png")
	}
}

func UpdateMediaPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var x, y, wi, he int
		var err error
		x, err = strconv.Atoi(r.URL.Query().Get("x"))
		if err != nil {
			x = 0
		}
		y, err = strconv.Atoi(r.URL.Query().Get("y"))
		if err != nil {
			y = 0
		}
		wi, err = strconv.Atoi(r.URL.Query().Get("w"))
		if err != nil {
			wi = 0
		}
		he, err = strconv.Atoi(r.URL.Query().Get("h"))
		if err != nil {
			he = 0
		}
		vars := mux.Vars(r)
		mediaID := vars["id"]
		// limits upload form size to 5mb
		maxMediaFileSize := 5 * 1024 * 1024
		r.Body = http.MaxBytesReader(w, r.Body, int64(maxMediaFileSize))
		imageFile, header, err := r.FormFile("image")
		if err != nil {
			svr.Log(err, "unable to read media file")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		defer imageFile.Close()
		fileBytes, err := ioutil.ReadAll(imageFile)
		if err != nil {
			svr.Log(err, "unable to read imageFile file content")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		contentType := http.DetectContentType(fileBytes)
		contentTypeInvalid := true
		for _, allowedMedia := range allowedMediaTypes {
			if allowedMedia == contentType {
				contentTypeInvalid = false
			}
		}
		if contentTypeInvalid {
			svr.Log(errors.New("invalid media content type"), fmt.Sprintf("media file %s is not one of the allowed media types: %+v", contentType, allowedMediaTypes))
			svr.JSON(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		if header.Size > int64(maxMediaFileSize) {
			svr.Log(errors.New("media file is too large"), fmt.Sprintf("media file too large: %d > %d", header.Size, maxMediaFileSize))
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		decImage, _, err := image.Decode(bytes.NewReader(fileBytes))
		if err != nil {
			svr.Log(err, "unable to decode image from bytes")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		min := decImage.Bounds().Dy()
		if decImage.Bounds().Dx() < min {
			min = decImage.Bounds().Dx()
		}
		if he == 0 || wi == 0 || he != wi {
			he = min
			wi = min
		}
		cutImage := decImage.(interface {
			SubImage(r image.Rectangle) image.Image
		}).SubImage(image.Rect(x, y, wi+x, he+y))
		cutImageBytes := new(bytes.Buffer)
		switch contentType {
		case "image/jpg", "image/jpeg":
			if err := jpeg.Encode(cutImageBytes, cutImage, nil); err != nil {
				svr.Log(err, "unable to encode cutImage into jpeg")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		case "image/png":
			if err := png.Encode(cutImageBytes, cutImage); err != nil {
				svr.Log(err, "unable to encode cutImage into png")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		default:
			svr.Log(errors.New("content type not supported for encoding"), fmt.Sprintf("content type %s not supported for encoding", contentType))
			svr.JSON(w, http.StatusInternalServerError, nil)
		}
		err = database.UpdateMedia(svr.Conn, database.Media{cutImageBytes.Bytes(), contentType}, mediaID)
		if err != nil {
			svr.Log(err, "unable to update media image to db")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func SaveMediaPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var x, y, wi, he int
		var err error
		x, err = strconv.Atoi(r.URL.Query().Get("x"))
		if err != nil {
			x = 0
		}
		y, err = strconv.Atoi(r.URL.Query().Get("y"))
		if err != nil {
			y = 0
		}
		wi, err = strconv.Atoi(r.URL.Query().Get("w"))
		if err != nil {
			wi = 0
		}
		he, err = strconv.Atoi(r.URL.Query().Get("h"))
		if err != nil {
			he = 0
		}
		// limits upload form size to 5mb
		maxMediaFileSize := 5 * 1024 * 1024
		allowedMediaTypes := []string{"image/png", "image/jpeg", "image/jpg"}
		r.Body = http.MaxBytesReader(w, r.Body, int64(maxMediaFileSize))
		cv, header, err := r.FormFile("image")
		if err != nil {
			svr.Log(err, "unable to read media file")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		defer cv.Close()
		fileBytes, err := ioutil.ReadAll(cv)
		if err != nil {
			svr.Log(err, "unable to read cv file content")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		contentType := http.DetectContentType(fileBytes)
		contentTypeInvalid := true
		for _, allowedMedia := range allowedMediaTypes {
			if allowedMedia == contentType {
				contentTypeInvalid = false
			}
		}
		if contentTypeInvalid {
			svr.Log(errors.New("invalid media content type"), fmt.Sprintf("media file %s is not one of the allowed media types: %+v", contentType, allowedMediaTypes))
			svr.JSON(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		if header.Size > int64(maxMediaFileSize) {
			svr.Log(errors.New("media file is too large"), fmt.Sprintf("media file too large: %d > %d", header.Size, maxMediaFileSize))
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		decImage, _, err := image.Decode(bytes.NewReader(fileBytes))
		if err != nil {
			svr.Log(err, "unable to decode image from bytes")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		min := decImage.Bounds().Dy()
		if decImage.Bounds().Dx() < min {
			min = decImage.Bounds().Dx()
		}
		if he == 0 || wi == 0 || wi != he {
			he = min
			wi = min
		}
		cutImage := decImage.(interface {
			SubImage(r image.Rectangle) image.Image
		}).SubImage(image.Rect(x, y, wi, he))
		cutImageBytes := new(bytes.Buffer)
		switch contentType {
		case "image/jpg", "image/jpeg":
			if err := jpeg.Encode(cutImageBytes, cutImage, nil); err != nil {
				svr.Log(err, "unable to encode cutImage into jpeg")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		case "image/png":
			if err := png.Encode(cutImageBytes, cutImage); err != nil {
				svr.Log(err, "unable to encode cutImage into png")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		default:
			svr.Log(errors.New("content type not supported for encoding"), fmt.Sprintf("content type %s not supported for encoding", contentType))
			svr.JSON(w, http.StatusInternalServerError, nil)
		}
		id, err := database.SaveMedia(svr.Conn, database.Media{cutImageBytes.Bytes(), contentType})
		if err != nil {
			svr.Log(err, "unable to save media image to db")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.JSON(w, http.StatusOK, map[string]interface{}{"id": id})
	}
}

func UpdateJobPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		jobRq := &job.JobRqUpdate{}
		if err := decoder.Decode(&jobRq); err != nil {
			svr.Log(err, fmt.Sprintf("unable to parse job request for update: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		jobID, err := jobRepo.JobPostIDByToken(jobRq.Token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", jobRq.Token))
			svr.JSON(w, http.StatusNotFound, nil)
			return
		}
		err = jobRepo.UpdateJob(jobRq, jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to update job request: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		if err := svr.CacheDelete(server.CacheKeyPinnedJobs); err != nil {
			svr.Log(err, "unable to cleanup cache after approving job")
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}
func PermanentlyDeleteJobByToken(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			jobRq := &job.JobRqUpdate{}
			if err := decoder.Decode(&jobRq); err != nil {
				svr.Log(err, fmt.Sprintf("unable to parse job request for delete: %#v", jobRq))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			jobID, err := jobRepo.JobPostIDByToken(jobRq.Token)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", jobRq.Token))
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			err = jobRepo.DeleteJobCascade(jobID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to permanently delete job: %#v", jobRq))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func ApproveJobPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			jobRq := &job.JobRqUpdate{}
			if err := decoder.Decode(&jobRq); err != nil {
				svr.Log(err, fmt.Sprintf("unable to parse job request for update: %#v", jobRq))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			jobID, err := jobRepo.JobPostIDByToken(jobRq.Token)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", jobRq.Token))
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			err = jobRepo.ApproveJob(jobID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to update job request: %#v", jobRq))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			err = svr.GetEmail().SendEmail("Diego from Golang Cafe <team@golang.cafe>", jobRq.Email, email.GolangCafeEmailAddress, "Your Job Ad on Golang Cafe", fmt.Sprintf("Thanks for using Golang Cafe,\n\nYour Job Ad has been approved and it's currently live on Golang Cafe: https://golang.cafe.\n\nYour Job Dashboard: https://golang.cafe/edit/%s\n\nThe ad expires in 90 days and does not automatically renew. If you wish to sponsor or pin again the job ad you can do so by following the edit link.\n\nI am always available to answer any questions you may have,\n\nBest,\n\nDiego\n%s\n%s", jobRq.Token, svr.GetConfig().AdminEmail, svr.GetConfig().PhoneNumber))
			if err != nil {
				svr.Log(err, "unable to send email while approving job ad")
			}
			if err := svr.CacheDelete(server.CacheKeyPinnedJobs); err != nil {
				svr.Log(err, "unable to cleanup cache after approving job")
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func DisapproveJobPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		jobRq := &job.JobRqUpdate{}
		if err := decoder.Decode(&jobRq); err != nil {
			svr.Log(err, fmt.Sprintf("unable to parse job request for update: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		jobID, err := jobRepo.JobPostIDByToken(jobRq.Token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", jobRq.Token))
			svr.JSON(w, http.StatusNotFound, nil)
			return
		}
		err = jobRepo.DisapproveJob(jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to update job request: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func TrackJobClickoutPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		externalID := vars["id"]
		if externalID == "" {
			svr.Log(errors.New("got empty id for tracking job"), "got empty externalID for tracking")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		jobPost, err := jobRepo.GetJobByExternalID(externalID)
		if err != nil {
			svr.Log(err, "unable to get JobID from externalID")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		if err := jobRepo.TrackJobClickout(jobPost.ID); err != nil {
			svr.Log(err, fmt.Sprintf("unable to save job clickout for job id %d. %v", jobPost.ID, err))
			svr.JSON(w, http.StatusOK, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func TrackJobClickoutAndRedirectToJobPage(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		externalID := r.URL.Query().Get("j")
		if externalID == "" {
			svr.Log(errors.New("TrackJobClickoutAndRedirectToJobPage: got empty id for tracking job"), "got empty externalID for tracking")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		reg, _ := regexp.Compile("[^a-zA-Z0-9 ]+")
		jobPost, err := jobRepo.GetJobByExternalID(reg.ReplaceAllString(externalID, ""))
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to get HowToApply from externalID %s", externalID))
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		if err := jobRepo.TrackJobClickout(jobPost.ID); err != nil {
			svr.Log(err, fmt.Sprintf("unable to save job clickout for job id %d. %v", jobPost.ID, err))
			svr.JSON(w, http.StatusOK, nil)
			return
		}
		svr.Redirect(w, r, http.StatusTemporaryRedirect, jobPost.HowToApply)
	}
}

func EditJobViewPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		token := vars["token"]
		isCallback := r.URL.Query().Get("callback")
		paymentSuccess := r.URL.Query().Get("payment")
		expiredUpsell := r.URL.Query().Get("expired")
		jobID, err := jobRepo.JobPostIDByToken(token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", token))
			svr.JSON(w, http.StatusNotFound, nil)
			return
		}
		jobPost, err := jobRepo.JobPostByIDForEdit(jobID)
		if err != nil || jobPost == nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job by ID %d", jobID))
			svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job for golang.cafe/edit/%s not found", token))
			return
		}
		clickoutCount, err := jobRepo.GetClickoutCountForJob(jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job clickout count for job id %d", jobID))
		}
		viewCount, err := jobRepo.GetViewCountForJob(jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job view count for job id %d", jobID))
		}
		conversionRate := ""
		if clickoutCount > 0 && viewCount > 0 {
			conversionRate = fmt.Sprintf("%.2f", float64(float64(clickoutCount)/float64(viewCount)*100))
		}
		purchaseEvents, err := database.GetPurchaseEvents(svr.Conn, jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job payment events for job id %d", jobID))
		}
		stats, err := jobRepo.GetStatsForJob(jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve stats for job id %d", jobID))
		}
		statsSet, err := json.Marshal(stats)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to marshal stats for job id %d", jobID))
		}
		currency, err := svr.GetCurrencyFromRequest(r)
		if err != nil {
			svr.Log(err, "could not find ip address in x-forwarded-for, defaulting currency to USD")
		}
		svr.Render(w, http.StatusOK, "edit.html", map[string]interface{}{
			"Job":                        jobPost,
			"Stats":                      string(statsSet),
			"Purchases":                  purchaseEvents,
			"JobPerksEscaped":            svr.JSEscapeString(jobPost.Perks),
			"JobInterviewProcessEscaped": svr.JSEscapeString(jobPost.InterviewProcess),
			"JobDescriptionEscaped":      svr.JSEscapeString(jobPost.JobDescription),
			"Token":                      token,
			"ViewCount":                  viewCount,
			"ClickoutCount":              clickoutCount,
			"ConversionRate":             conversionRate,
			"IsCallback":                 isCallback,
			"PaymentSuccess":             paymentSuccess,
			"IsUpsell":                   len(expiredUpsell) > 0,
			"Currency":                   currency,
			"StripePublishableKey":       svr.GetConfig().StripePublishableKey,
			"IsUnpinned":                 jobPost.AdType < 1,
		})
	}
}

func ManageJobBySlugViewPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			slug := vars["slug"]
			jobPost, err := jobRepo.JobPostBySlugAdmin(slug)
			if err != nil {
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			jobPostToken, err := jobRepo.TokenByJobID(jobPost.ID)
			if err != nil {
				svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job for golang.cafe/manage/job/%s not found", slug))
				return
			}
			svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("/manage/%s", jobPostToken))
		},
	)
}

func ManageJobViewPageHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			token := vars["token"]
			jobID, err := jobRepo.JobPostIDByToken(token)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", token))
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			jobPost, err := jobRepo.JobPostByIDForEdit(jobID)
			if err != nil || jobPost == nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve job by ID %d", jobID))
				svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job for golang.cafe/edit/%s not found", token))
				return
			}
			clickoutCount, err := jobRepo.GetClickoutCountForJob(jobID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve job clickout count for job id %d", jobID))
			}
			viewCount, err := jobRepo.GetViewCountForJob(jobID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve job view count for job id %d", jobID))
			}
			conversionRate := ""
			if clickoutCount > 0 && viewCount > 0 {
				conversionRate = fmt.Sprintf("%.2f", float64(float64(clickoutCount)/float64(viewCount)*100))
			}
			svr.Render(w, http.StatusOK, "manage.html", map[string]interface{}{
				"Job":                        jobPost,
				"JobPerksEscaped":            svr.JSEscapeString(jobPost.Perks),
				"JobInterviewProcessEscaped": svr.JSEscapeString(jobPost.InterviewProcess),
				"JobDescriptionEscaped":      svr.JSEscapeString(jobPost.JobDescription),
				"Token":                      token,
				"ViewCount":                  viewCount,
				"ClickoutCount":              clickoutCount,
				"ConversionRate":             conversionRate,
			})
		},
	)
}
