package server

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	stdtemplate "html/template"

	"github.com/0x13a/golang.cafe/internal/company"
	"github.com/0x13a/golang.cafe/internal/config"
	"github.com/0x13a/golang.cafe/internal/database"
	"github.com/0x13a/golang.cafe/internal/developer"
	"github.com/0x13a/golang.cafe/internal/email"
	"github.com/0x13a/golang.cafe/internal/ipgeolocation"
	"github.com/0x13a/golang.cafe/internal/job"
	"github.com/0x13a/golang.cafe/internal/middleware"
	"github.com/0x13a/golang.cafe/internal/template"
	"github.com/aclements/go-moremath/stats"
	"github.com/dustin/go-humanize"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"

	"github.com/allegro/bigcache/v3"
	"github.com/getsentry/raven-go"
)

const (
	CacheKeyPinnedJobs       = "pinnedJobs"
	CacheKeyNewJobsLastWeek  = "newJobsLastWeek"
	CacheKeyNewJobsLastMonth = "newJobsLastMonth"
)

type Server struct {
	cfg           config.Config
	Conn          *sql.DB
	router        *mux.Router
	tmpl          *template.Template
	emailClient   email.Client
	ipGeoLocation ipgeolocation.IPGeoLocation
	SessionStore  *sessions.CookieStore
	bigCache      *bigcache.BigCache
	emailRe       *regexp.Regexp
}

func NewServer(
	cfg config.Config,
	conn *sql.DB,
	r *mux.Router,
	t *template.Template,
	emailClient email.Client,
	ipGeoLocation ipgeolocation.IPGeoLocation,
	sessionStore *sessions.CookieStore,
) Server {
	// todo: move somewhere else
	raven.SetDSN(cfg.SentryDSN)

	bigCache, err := bigcache.NewBigCache(bigcache.DefaultConfig(12 * time.Hour))
	svr := Server{
		cfg:           cfg,
		Conn:          conn,
		router:        r,
		tmpl:          t,
		emailClient:   emailClient,
		ipGeoLocation: ipGeoLocation,
		SessionStore:  sessionStore,
		bigCache:      bigCache,
		emailRe:       regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$"),
	}
	if err != nil {
		svr.Log(err, "unable to initialise big cache")
	}

	return svr
}

func (s Server) RegisterRoute(path string, handler func(w http.ResponseWriter, r *http.Request), methods []string) {
	s.router.HandleFunc(path, handler).Methods(methods...)
}

func (s Server) RegisterPathPrefix(path string, handler http.Handler, methods []string) {
	s.router.PathPrefix(path).Handler(handler).Methods(methods...)
}

func (s Server) StringToHTML(str string) stdtemplate.HTML {
	return s.tmpl.StringToHTML(str)
}

func (s Server) JSEscapeString(str string) string {
	return s.tmpl.JSEscapeString(str)
}

func (s Server) MarkdownToHTML(str string) stdtemplate.HTML {
	return s.tmpl.MarkdownToHTML(str)
}

func (s Server) GetConfig() config.Config {
	return s.cfg
}

func (s Server) RenderSalaryForLocation(w http.ResponseWriter, r *http.Request, jobRepo *job.Repository, location string) {
	loc, err := database.GetLocation(s.Conn, location)
	complimentaryRemote := false
	if err != nil {
		complimentaryRemote = true
		loc.Name = "Remote"
		loc.Currency = "$"
	}
	set, err := database.GetSalaryDataForLocationAndCurrency(s.Conn, loc.Name, loc.Currency)
	if err != nil {
		s.Log(err, fmt.Sprintf("unable to retrieve salary stats for location %s and currency %s, err: %#v", location, loc.Currency, err))
		s.JSON(w, http.StatusInternalServerError, map[string]string{"status": "error"})
		return
	}
	trendSet, err := database.GetSalaryTrendsForLocationAndCurrency(s.Conn, loc.Name, loc.Currency)
	if err != nil {
		s.Log(err, fmt.Sprintf("unable to retrieve salary trends for location %s and currency %s, err: %#v", location, loc.Currency, err))
		s.JSON(w, http.StatusInternalServerError, map[string]string{"status": "error"})
		return
	}
	if len(set) < 1 {
		complimentaryRemote = true
		set, err = database.GetSalaryDataForLocationAndCurrency(s.Conn, "Remote", "$")
		if err != nil {
			s.Log(err, fmt.Sprintf("unable to retrieve salary stats for location %s and currency %s, err: %#v", location, loc.Currency, err))
			s.JSON(w, http.StatusInternalServerError, map[string]string{"status": "error"})
			return
		}
		trendSet, err = database.GetSalaryTrendsForLocationAndCurrency(s.Conn, "Remote", "$")
		if err != nil {
			s.Log(err, fmt.Sprintf("unable to retrieve salary stats for location %s and currency %s, err: %#v", location, loc.Currency, err))
			s.JSON(w, http.StatusInternalServerError, map[string]string{"status": "error"})
			return
		}
	}
	jsonRes, err := json.Marshal(set)
	if err != nil {
		s.Log(err, fmt.Sprintf("unable to marshal data set %v, err: %#v", set, err))
		s.JSON(w, http.StatusInternalServerError, map[string]string{"status": "error"})
		return
	}
	jsonTrendRes, err := json.Marshal(trendSet)
	if err != nil {
		s.Log(err, fmt.Sprintf("unable to marshal data set trneds %v, err: %#v", trendSet, err))
		s.JSON(w, http.StatusInternalServerError, map[string]string{"status": "error"})
		return
	}
	var sampleMin, sampleMax stats.Sample
	for _, x := range set {
		sampleMin.Xs = append(sampleMin.Xs, float64(x.Min))
		sampleMax.Xs = append(sampleMax.Xs, float64(x.Max))
	}
	min, _ := sampleMin.Bounds()
	_, max := sampleMax.Bounds()
	min = min - 30000
	max = max + 30000
	if min < 0 {
		min = 0
	}
	ua := r.Header.Get("user-agent")
	ref := r.Header.Get("referer")
	ips := strings.Split(r.Header.Get("x-forwarded-for"), ", ")
	if len(ips) > 0 && strings.Contains(ref, "golang.cafe") {
		hashedIP := sha256.Sum256([]byte(ips[0]))
		go func() {
			if err := database.TrackSearchEvent(s.Conn, ua, hex.EncodeToString(hashedIP[:]), location, "", len(set), job.SearchTypeSalary); err != nil {
				fmt.Printf("err while saving loc: %s\n", err)
			}
		}()
	}
	jobPosts, err := jobRepo.TopNJobsByCurrencyAndLocation(loc.Currency, loc.Name, 3)
	if err != nil {
		s.Log(err, "jobRepo.TopNJobsByCurrencyAndLocation")
	}
	if len(jobPosts) == 0 {
		jobPosts, err = jobRepo.TopNJobsByCurrencyAndLocation("$", "Remote", 3)
		if err != nil {
			s.Log(err, "jobRepo.TopNJobsByCurrencyAndLocation")
		}
	}
	lastJobPosted, err := jobRepo.LastJobPosted()
	if err != nil {
		s.Log(err, "could not retrieve last job posted at")
		lastJobPosted = time.Now().AddDate(0, 0, -1)
	}

	emailSubscribersCount, err := database.CountEmailSubscribers(s.Conn)
	if err != nil {
		s.Log(err, "database.CountEmailSubscribers")
	}

	s.Render(w, http.StatusOK, "salary-explorer.html", map[string]interface{}{
		"Location":                 strings.ReplaceAll(location, "-", " "),
		"LocationURLEncoded":       url.PathEscape(strings.ReplaceAll(location, "-", " ")),
		"Currency":                 loc.Currency,
		"DataSet":                  string(jsonRes),
		"DataSetTrends":            string(jsonTrendRes),
		"TextCompanies":            textifyCompanies(loc.Name, jobPosts, jobPosts),
		"TextJobTitles":            textifyJobTitles(jobPosts),
		"P10Max":                   humanize.Comma(int64(math.Round(sampleMax.Quantile(0.1)))),
		"P10Min":                   humanize.Comma(int64(math.Round(sampleMin.Quantile(0.1)))),
		"P50Max":                   humanize.Comma(int64(math.Round(sampleMax.Quantile(0.5)))),
		"P50Min":                   humanize.Comma(int64(math.Round(sampleMin.Quantile(0.5)))),
		"P90Max":                   humanize.Comma(int64(math.Round(sampleMax.Quantile(0.9)))),
		"P90Min":                   humanize.Comma(int64(math.Round(sampleMin.Quantile(0.9)))),
		"MeanMin":                  humanize.Comma(int64(math.Round(sampleMin.Mean()))),
		"MeanMax":                  humanize.Comma(int64(math.Round(sampleMax.Mean()))),
		"StdDevMin":                humanize.Comma(int64(math.Round(sampleMin.StdDev()))),
		"StdDevMax":                humanize.Comma(int64(math.Round(sampleMax.StdDev()))),
		"Count":                    len(set),
		"Country":                  loc.Country,
		"Region":                   loc.Region,
		"Population":               loc.Population,
		"Min":                      int64(math.Round(min)),
		"Max":                      int64(math.Round(max)),
		"ComplimentaryRemote":      complimentaryRemote,
		"LastJobPostedAt":          lastJobPosted.Format(time.RFC3339),
		"LastJobPostedAtHumanized": humanize.Time(lastJobPosted),
		"MonthAndYear":             time.Now().UTC().Format("January 2006"),
		"EmailSubscribersCount":    humanize.Comma(int64(emailSubscribersCount)),
	})
}

func (s Server) RenderPageForLocationAndTag(w http.ResponseWriter, r *http.Request, jobRepo *job.Repository, location, tag, page, salary, currency, htmlView string) {
	var validSalary bool
	for _, band := range s.GetConfig().AvailableSalaryBands {
		if fmt.Sprintf("%d", band) == salary {
			validSalary = true
			break
		}
	}
	var validCurrency bool
	for _, availableCurrency := range s.GetConfig().AvailableCurrencies {
		if availableCurrency == currency {
			validCurrency = true
			break
		}
	}
	if (salary != "" && !validSalary) || (currency != "" && !validCurrency) {
		s.Redirect(w, r, http.StatusMovedPermanently, "/")
		return
	}
	showPage := true
	if page == "" {
		page = "1"
		showPage = false
	}
	salaryInt, err := strconv.Atoi(salary)
	if err != nil {
		salaryInt = 0
	}
	salaryInt = int(salaryInt)
	tag = strings.TrimSpace(tag)
	location = strings.TrimSpace(location)
	reg, err := regexp.Compile("[^a-zA-Z0-9\\s]+")
	if err != nil {
		s.Log(err, "unable to compile regex (this should never happen)")
	}
	tag = reg.ReplaceAllString(tag, "")
	location = reg.ReplaceAllString(location, "")
	pageID, err := strconv.Atoi(page)
	if err != nil {
		pageID = 1
		showPage = false
	}
	isLandingPage := tag == "" && location == "" && page == "1" && salary == ""
	var newJobsLastWeek, newJobsLastMonth int
	newJobsLastWeekCached, okWeek := s.CacheGet(CacheKeyNewJobsLastWeek)
	newJobsLastMonthCached, okMonth := s.CacheGet(CacheKeyNewJobsLastMonth)
	if !okMonth || !okWeek {
		// load and cache last jobs count
		var err error
		newJobsLastWeek, newJobsLastMonth, err = jobRepo.NewJobsLastWeekOrMonth()
		if err != nil {
			s.Log(err, "unable to retrieve new jobs last week last month")
		}
		buf := &bytes.Buffer{}
		enc := gob.NewEncoder(buf)
		if err := enc.Encode(newJobsLastWeek); err != nil {
			s.Log(err, "unable to encode new jobs last week")
		}
		if err := s.CacheSet(CacheKeyNewJobsLastWeek, buf.Bytes()); err != nil {
			s.Log(err, "unable to cache set new jobs lat week")
		}
		buf.Reset()
		if err := enc.Encode(newJobsLastMonth); err != nil {
			s.Log(err, "unable to encode new jobs last month")
		}
		if err := s.CacheSet(CacheKeyNewJobsLastMonth, buf.Bytes()); err != nil {
			s.Log(err, "unable to cache set new jobs lat month")
		}
	} else {
		dec := gob.NewDecoder(bytes.NewReader(newJobsLastWeekCached))
		if err := dec.Decode(&newJobsLastWeek); err != nil {
			s.Log(err, "unable to decode cached new jobs last week")
		}
		dec = gob.NewDecoder(bytes.NewReader(newJobsLastMonthCached))
		if err := dec.Decode(&newJobsLastMonth); err != nil {
			s.Log(err, "unable to decode cached new jobs last month")
		}
	}
	var pinnedJobs []*job.JobPost
	// only load pinned jobs for main landing page
	if isLandingPage {
		pinnedJobsCached, ok := s.CacheGet(CacheKeyPinnedJobs)
		if !ok {
			// load and cache jobs
			pinnedJobs, err = jobRepo.GetPinnedJobs()
			if err != nil {
				s.Log(err, "unable to get pinned jobs")
			}
			for i, j := range pinnedJobs {
				pinnedJobs[i].CompanyURLEnc = url.PathEscape(j.Company)
				pinnedJobs[i].JobDescription = string(s.tmpl.MarkdownToHTML(j.JobDescription))
				pinnedJobs[i].Perks = string(s.tmpl.MarkdownToHTML(j.Perks))
				pinnedJobs[i].SalaryRange = fmt.Sprintf("%s%s to %s%s", j.SalaryCurrency, humanize.Comma(j.SalaryMin), j.SalaryCurrency, humanize.Comma(j.SalaryMax))
				pinnedJobs[i].InterviewProcess = string(s.tmpl.MarkdownToHTML(j.InterviewProcess))
				if s.IsEmail(j.HowToApply) {
					pinnedJobs[i].IsQuickApply = true
				}
			}
			buf := &bytes.Buffer{}
			enc := gob.NewEncoder(buf)
			if err := enc.Encode(pinnedJobs); err != nil {
				s.Log(err, "unable to encode pinned jobs")
			}
			if err := s.CacheSet(CacheKeyPinnedJobs, buf.Bytes()); err != nil {
				s.Log(err, "unable to set pinnedJobs cache")
			}
		} else {
			// pinned jobs are cached
			dec := gob.NewDecoder(bytes.NewReader(pinnedJobsCached))
			if err := dec.Decode(&pinnedJobs); err != nil {
				s.Log(err, "unable to decode pinned jobs")
			}
		}
	}
	jobsForPage, totalJobCount, err := jobRepo.JobsByQuery(location, tag, pageID, salaryInt, currency, s.cfg.JobsPerPage, !isLandingPage)
	if err != nil {
		s.Log(err, "unable to get jobs by query")
		s.JSON(w, http.StatusInternalServerError, "Oops! An internal error has occurred")
		return
	}
	var complementaryRemote bool
	if len(jobsForPage) == 0 {
		complementaryRemote = true
		jobsForPage, totalJobCount, err = jobRepo.JobsByQuery("Remote", tag, pageID, salaryInt, currency, s.cfg.JobsPerPage, !isLandingPage)
		if len(jobsForPage) == 0 {
			jobsForPage, totalJobCount, err = jobRepo.JobsByQuery("Remote", "", pageID, salaryInt, currency, s.cfg.JobsPerPage, !isLandingPage)
		}
	}
	if err != nil {
		s.Log(err, "unable to retrieve jobs by query")
		s.JSON(w, http.StatusInternalServerError, "Oops! An internal error has occurred")
		return
	}
	pages := []int{}
	pageLinksPerPage := 8
	pageLinkShift := ((pageLinksPerPage / 2) + 1)
	firstPage := 1
	if pageID-pageLinkShift > 0 {
		firstPage = pageID - pageLinkShift
	}
	for i, j := firstPage, 1; i <= totalJobCount/s.cfg.JobsPerPage+1 && j <= pageLinksPerPage; i, j = i+1, j+1 {
		pages = append(pages, i)
	}

	locFromDB := database.Location{}
	locFromDB.Name = "Remote"
	locFromDB.Currency = "$"
	if location != "" && !strings.EqualFold(location, "remote") {
		locFromDB, err = database.GetLocation(s.Conn, location)
		if err != nil {
			locFromDB.Name = "Remote"
			locFromDB.Currency = "$"
		}
	}
	var minSalary int64 = 1<<63 - 1
	var maxSalary int64 = 0
	for i, j := range jobsForPage {
		jobsForPage[i].CompanyURLEnc = url.PathEscape(j.Company)
		jobsForPage[i].JobDescription = string(s.tmpl.MarkdownToHTML(j.JobDescription))
		jobsForPage[i].SalaryRange = fmt.Sprintf("%s%s to %s%s", j.SalaryCurrency, humanize.Comma(j.SalaryMin), j.SalaryCurrency, humanize.Comma(j.SalaryMax))
		jobsForPage[i].Perks = string(s.tmpl.MarkdownToHTML(j.Perks))
		jobsForPage[i].InterviewProcess = string(s.tmpl.MarkdownToHTML(j.InterviewProcess))
		if s.IsEmail(j.HowToApply) {
			jobsForPage[i].IsQuickApply = true
		}
		if j.SalaryPeriod == "year" && j.SalaryCurrency == locFromDB.Currency && minSalary > j.SalaryMin {
			minSalary = j.SalaryMin
		}
		if j.SalaryPeriod == "year" && j.SalaryCurrency == locFromDB.Currency && maxSalary < j.SalaryMax {
			maxSalary = j.SalaryMax
		}
	}

	ua := r.Header.Get("user-agent")
	ref := r.Header.Get("referer")
	ips := strings.Split(r.Header.Get("x-forwarded-for"), ", ")
	if len(ips) > 0 && strings.Contains(ref, "golang.cafe") {
		hashedIP := sha256.Sum256([]byte(ips[0]))
		go func() {
			if err := database.TrackSearchEvent(s.Conn, ua, hex.EncodeToString(hashedIP[:]), location, tag, len(jobsForPage), job.SearchTypeJob); err != nil {
				fmt.Printf("err while saving event: %s\n", err)
			}
		}()
	}

	locationWithCountry := strings.Title(location)
	relatedLocations := make([]string, 0)
	if locFromDB.Name != "Remote" {
		locationWithCountry = fmt.Sprintf("%s", locFromDB.Name)
		if locFromDB.Country != "" {
			locationWithCountry = fmt.Sprintf("%s, %s", locFromDB.Name, locFromDB.Country)
		}
		if locFromDB.Region != "" {
			locationWithCountry = fmt.Sprintf("%s, %s, %s", locFromDB.Name, locFromDB.Region, locFromDB.Country)
		}
		relatedLocations, err = database.GetRandomLocationsForCountry(s.Conn, locFromDB.Country, 6)
		if err != nil {
			s.Log(err, fmt.Sprintf("unable to get random locations for country %s", locFromDB.Country))
		}
	}
	if currency == "" {
		currency = "USD"
	}
	lastJobPosted, err := jobRepo.LastJobPosted()
	if err != nil {
		s.Log(err, "could not retrieve last job posted at")
		lastJobPosted = time.Now().AddDate(0, 0, -1)
	}

	emailSubscribersCount, err := database.CountEmailSubscribers(s.Conn)
	if err != nil {
		s.Log(err, "database.CountEmailSubscribers")
	}

	s.Render(w, http.StatusOK, htmlView, map[string]interface{}{
		"Jobs":                      jobsForPage,
		"PinnedJobs":                pinnedJobs,
		"JobsMinusOne":              len(jobsForPage) - 1,
		"LocationFilter":            strings.Title(location),
		"LocationFilterWithCountry": locationWithCountry,
		"LocationFilterURLEnc":      url.PathEscape(strings.Title(location)),
		"TagFilter":                 tag,
		"SalaryFilter":              salaryInt,
		"CurrencyFilter":            currency,
		"AvailableCurrencies":       s.GetConfig().AvailableCurrencies,
		"AvailableSalaryBands":      s.GetConfig().AvailableSalaryBands,
		"TagFilterURLEnc":           url.PathEscape(tag),
		"CurrentPage":               pageID,
		"ShowPage":                  showPage,
		"PageSize":                  s.cfg.JobsPerPage,
		"PageIndexes":               pages,
		"TotalJobCount":             totalJobCount,
		"TextJobCount":              textifyJobCount(totalJobCount),
		"TextCompanies":             textifyCompanies(location, pinnedJobs, jobsForPage),
		"TextJobTitles":             textifyJobTitles(jobsForPage),
		"LastJobPostedAt":           lastJobPosted.Format(time.RFC3339),
		"LastJobPostedAtHumanized":  humanize.Time(lastJobPosted),
		"HasSalaryInfo":             maxSalary > 0,
		"MinSalary":                 fmt.Sprintf("%s%s", locFromDB.Currency, humanize.Comma(minSalary)),
		"MaxSalary":                 fmt.Sprintf("%s%s", locFromDB.Currency, humanize.Comma(maxSalary)),
		"LocationFromDB":            locFromDB.Name,
		"CountryFromDB":             locFromDB.Country,
		"RegionFromDB":              locFromDB.Region,
		"PopulationFromDB":          locFromDB.Population,
		"LocationEmojiFromDB":       locFromDB.Emoji,
		"RelatedLocations":          relatedLocations,
		"ComplementaryRemote":       complementaryRemote,
		"MonthAndYear":              time.Now().UTC().Format("January 2006"),
		"NewJobsLastWeek":           newJobsLastWeek,
		"NewJobsLastMonth":          newJobsLastMonth,
		"EmailSubscribersCount":     humanize.Comma(int64(emailSubscribersCount)),
	})
}

func textifyJobCount(n int) string {
	if n <= 50 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d+", (n/50)*50)
}

func textifyCompanies(location string, pinnedJobs, jobs []*job.JobPost) string {
	if len(pinnedJobs) > 2 && location == "" {
		jobs = pinnedJobs
	}
	switch {
	case len(jobs) == 1:
		return jobs[0].Company
	case len(jobs) == 2:
		return fmt.Sprintf("%s and %s", jobs[0].Company, jobs[1].Company)
	case len(jobs) > 2:
		return fmt.Sprintf("%s, %s and %s", jobs[0].Company, jobs[1].Company, jobs[2].Company)
	}

	return ""
}

func textifyGeneric(items []string) string {
	switch {
	case len(items) == 1:
		return items[0]
	case len(items) == 2:
		return fmt.Sprintf("%s and %s", items[0], items[1])
	case len(items) > 2:
		return fmt.Sprintf("%s and %s", strings.Join(items[:len(items)-1], ", "), items[len(items)-1])
	}

	return ""
}

func textifyCompanyNames(companies []company.Company, max int) string {
	switch {
	case len(companies) == 1:
		return companies[0].Name
	case len(companies) == 2:
		return fmt.Sprintf("%s and %s", companies[0].Name, companies[1].Name)
	case len(companies) > 2:
		names := make([]string, 0, len(companies))
		if max >= len(companies)-1 {
			max = len(companies) - 1
		}
		for i := 0; i < max; i++ {
			names = append(names, companies[i].Name)
		}
		return fmt.Sprintf("%s and many others", strings.Join(names, ", "))
	}

	return ""
}

func textifyJobTitles(jobs []*job.JobPost) string {
	switch {
	case len(jobs) == 1:
		return jobs[0].JobTitle
	case len(jobs) == 2:
		return fmt.Sprintf("%s and %s", jobs[0].JobTitle, jobs[1].JobTitle)
	case len(jobs) > 2:
		return fmt.Sprintf("%s, %s and %s", jobs[0].JobTitle, jobs[1].JobTitle, jobs[2].JobTitle)
	}

	return ""
}

func (s Server) RenderPageForDeveloperRegistration(w http.ResponseWriter, r *http.Request, devRepo *developer.Repository, htmlView string) {
	topDevelopers, err := devRepo.GetTopDevelopers(10)
	if err != nil {
		s.Log(err, "unable to get top developers")
	}
	topDeveloperSkills, err := devRepo.GetTopDeveloperSkills(7)
	if err != nil {
		s.Log(err, "unable to get top developer skills")
	}
	lastDevUpdatedAt, err := devRepo.GetLastDevUpdatedAt()
	if err != nil {
		s.Log(err, "unable to retrieve last developer joined at")
	}
	topDeveloperNames := make([]string, 0, len(topDevelopers))
	for _, d := range topDevelopers {
		topDeveloperNames = append(topDeveloperNames, strings.Split(d.Name, " ")[0])
	}
	messagesSentLastMonth, err := devRepo.GetDeveloperMessagesSentLastMonth()
	if err != nil {
		s.Log(err, "GetDeveloperMessagesSentLastMonth")
	}
	devsRegisteredLastMonth, err := devRepo.GetDevelopersRegisteredLastMonth()
	if err != nil {
		s.Log(err, "GetDevelopersRegisteredLastMonth")
	}
	devPageViewsLastMonth, err := devRepo.GetDeveloperProfilePageViewsLastMonth()
	if err != nil {
		s.Log(err, "GetDeveloperProfilePageViewsLastMonth")
	}
	s.Render(w, http.StatusOK, htmlView, map[string]interface{}{
		"TopDevelopers":                      topDevelopers,
		"TopDeveloperNames":                  textifyGeneric(topDeveloperNames),
		"TopDeveloperSkills":                 textifyGeneric(topDeveloperSkills),
		"DeveloperMessagesSentLastMonth":     messagesSentLastMonth,
		"DevelopersRegisteredLastMonth":      devsRegisteredLastMonth,
		"DeveloperProfilePageViewsLastMonth": devPageViewsLastMonth,
		"MonthAndYear":                       time.Now().UTC().Format("January 2006"),
		"LastDevCreatedAt":                   lastDevUpdatedAt.Format(time.RFC3339),
		"LastDevCreatedAtHumanized":          humanize.Time(lastDevUpdatedAt),
	})
}

func (s Server) RenderPageForDevelopers(w http.ResponseWriter, r *http.Request, devRepo *developer.Repository, location, tag, page, htmlView string) {
	showPage := true
	if page == "" {
		page = "1"
		showPage = false
	}
	location = strings.TrimSpace(location)
	tag = strings.TrimSpace(tag)
	reg, err := regexp.Compile("[^a-zA-Z0-9\\s]+")
	if err != nil {
		s.Log(err, "unable to compile regex (this should never happen)")
	}
	location = reg.ReplaceAllString(location, "")
	tag = reg.ReplaceAllString(tag, "")
	pageID, err := strconv.Atoi(page)
	if err != nil {
		pageID = 1
		showPage = false
	}
	var complementaryRemote bool
	locSearch := location
	if strings.EqualFold(location, "remote") {
		locSearch = ""
	}
	developersForPage, totalDevelopersCount, err := devRepo.DevelopersByLocationAndTag(locSearch, tag, pageID, s.cfg.DevelopersPerPage)
	if err != nil {
		s.Log(err, "unable to get developers by location and tag")
		s.JSON(w, http.StatusInternalServerError, "Oops! An internal error has occurred")
		return
	}
	if len(developersForPage) == 0 {
		complementaryRemote = true
		developersForPage, totalDevelopersCount, err = devRepo.DevelopersByLocationAndTag("", "", pageID, s.cfg.DevelopersPerPage)
	}
	pages := []int{}
	pageLinksPerPage := 8
	pageLinkShift := ((pageLinksPerPage / 2) + 1)
	firstPage := 1
	if pageID-pageLinkShift > 0 {
		firstPage = pageID - pageLinkShift
	}
	for i, j := firstPage, 1; i <= totalDevelopersCount/s.cfg.DevelopersPerPage+1 && j <= pageLinksPerPage; i, j = i+1, j+1 {
		pages = append(pages, i)
	}
	for i, j := range developersForPage {
		developersForPage[i].CreatedAtHumanized = humanize.Time(j.CreatedAt.UTC())
		developersForPage[i].UpdatedAtHumanized = j.CreatedAt.UTC().Format("January 2006")
		developersForPage[i].SkillsArray = strings.Split(j.Skills, ",")
	}
	ua := r.Header.Get("user-agent")
	ref := r.Header.Get("referer")
	ips := strings.Split(r.Header.Get("x-forwarded-for"), ", ")
	if len(ips) > 0 && strings.Contains(ref, "golang.cafe") {
		hashedIP := sha256.Sum256([]byte(ips[0]))
		go func() {
			if err := database.TrackSearchEvent(s.Conn, ua, hex.EncodeToString(hashedIP[:]), location, "", len(developersForPage), developer.SearchTypeDeveloper); err != nil {
				fmt.Printf("err while saving event: %s\n", err)
			}
		}()
	}
	loc, err := database.GetLocation(s.Conn, location)
	if err != nil {
		loc.Name = "Remote"
		loc.Currency = "$"
	}
	topDevelopers, err := devRepo.GetTopDevelopers(5)
	if err != nil {
		s.Log(err, "unable to get top developer names")
	}
	topDeveloperSkills, err := devRepo.GetTopDeveloperSkills(7)
	if err != nil {
		s.Log(err, "unable to get top developer skills")
	}
	topDeveloperNames := make([]string, 0, len(topDevelopers))
	for _, d := range topDevelopers {
		topDeveloperNames = append(topDeveloperNames, strings.Split(d.Name, " ")[0])
	}

	var lastDevCreatedAt, lastDevCreatedAtHumanized string
	if len(developersForPage) > 0 {
		lastDevCreatedAt = developersForPage[0].UpdatedAt.Format(time.RFC3339)
		lastDevCreatedAtHumanized = humanize.Time(developersForPage[0].UpdatedAt)
	}

	emailSubscribersCount, err := database.CountEmailSubscribers(s.Conn)
	if err != nil {
		s.Log(err, "database.CountEmailSubscribers")
	}

	s.Render(w, http.StatusOK, htmlView, map[string]interface{}{
		"Developers":                developersForPage,
		"TopDeveloperNames":         textifyGeneric(topDeveloperNames),
		"TopDeveloperSkills":        textifyGeneric(topDeveloperSkills),
		"DevelopersMinusOne":        len(developersForPage) - 1,
		"LocationFilter":            strings.Title(location),
		"LocationURLEncoded":        url.PathEscape(strings.ReplaceAll(location, "-", " ")),
		"TextCount":                 textifyJobCount(totalDevelopersCount),
		"TagFilter":                 tag,
		"TagFilterURLEncoded":       url.PathEscape(tag),
		"CurrentPage":               pageID,
		"ShowPage":                  showPage,
		"PageSize":                  s.cfg.DevelopersPerPage,
		"Country":                   loc.Country,
		"Region":                    loc.Region,
		"PageIndexes":               pages,
		"TotalDevelopersCount":      totalDevelopersCount,
		"ComplementaryRemote":       complementaryRemote,
		"MonthAndYear":              time.Now().UTC().Format("January 2006"),
		"LastDevCreatedAt":          lastDevCreatedAt,
		"LastDevCreatedAtHumanized": lastDevCreatedAtHumanized,
		"EmailSubscribersCount":     humanize.Comma(int64(emailSubscribersCount)),
	})

}

func (s Server) RenderPageForCompanies(w http.ResponseWriter, r *http.Request, companyRepo *company.Repository, jobRepo *job.Repository, location, page, htmlView string) {
	showPage := true
	if page == "" {
		page = "1"
		showPage = false
	}
	location = strings.TrimSpace(location)
	reg, err := regexp.Compile("[^a-zA-Z0-9\\s]+")
	if err != nil {
		s.Log(err, "unable to compile regex (this should never happen)")
	}
	location = reg.ReplaceAllString(location, "")
	pageID, err := strconv.Atoi(page)
	if err != nil {
		pageID = 1
		showPage = false
	}
	var complementaryRemote bool
	companiesForPage, totalCompaniesCount, err := companyRepo.CompaniesByQuery(location, pageID, s.cfg.CompaniesPerPage)
	if err != nil {
		s.Log(err, "unable to get companies by query")
		s.JSON(w, http.StatusInternalServerError, "Oops! An internal error has occurred")
		return
	}
	if len(companiesForPage) == 0 {
		complementaryRemote = true
		companiesForPage, totalCompaniesCount, err = companyRepo.CompaniesByQuery("Remote", pageID, s.cfg.CompaniesPerPage)
	}
	loc, err := database.GetLocation(s.Conn, location)
	if err != nil {
		loc.Name = "Remote"
		loc.Currency = "$"
	}
	pages := []int{}
	pageLinksPerPage := 8
	pageLinkShift := ((pageLinksPerPage / 2) + 1)
	firstPage := 1
	if pageID-pageLinkShift > 0 {
		firstPage = pageID - pageLinkShift
	}
	for i, j := firstPage, 1; i <= totalCompaniesCount/s.cfg.CompaniesPerPage+1 && j <= pageLinksPerPage; i, j = i+1, j+1 {
		pages = append(pages, i)
	}

	ua := r.Header.Get("user-agent")
	ref := r.Header.Get("referer")
	ips := strings.Split(r.Header.Get("x-forwarded-for"), ", ")
	if len(ips) > 0 && strings.Contains(ref, "golang.cafe") {
		hashedIP := sha256.Sum256([]byte(ips[0]))
		go func() {
			if err := database.TrackSearchEvent(s.Conn, ua, hex.EncodeToString(hashedIP[:]), location, "", len(companiesForPage), company.SearchTypeCompany); err != nil {
				fmt.Printf("err while saving event: %s\n", err)
			}
		}()
	}
	jobPosts, err := jobRepo.TopNJobsByCurrencyAndLocation(loc.Currency, loc.Name, 3)
	if err != nil {
		s.Log(err, "database.TopNJobsByCurrencyAndLocation")
	}
	if len(jobPosts) == 0 {
		jobPosts, err = jobRepo.TopNJobsByCurrencyAndLocation("$", "Remote", 3)
		if err != nil {
			s.Log(err, "database.TopNJobsByCurrencyAndLocation")
		}
	}

	var lastJobPostedAt, lastJobPostedAtHumanized string
	if len(jobPosts) > 0 {
		lastJobPostedAt = time.Unix(jobPosts[0].CreatedAt, 0).Format(time.RFC3339)
		lastJobPostedAtHumanized = humanize.Time(time.Unix(jobPosts[0].CreatedAt, 0))
	}

	emailSubscribersCount, err := database.CountEmailSubscribers(s.Conn)
	if err != nil {
		s.Log(err, "database.CountEmailSubscribers")
	}

	s.Render(w, http.StatusOK, htmlView, map[string]interface{}{
		"Companies":                companiesForPage,
		"CompaniesMinusOne":        len(companiesForPage) - 1,
		"LocationFilter":           strings.Title(location),
		"LocationURLEncoded":       url.PathEscape(strings.ReplaceAll(location, "-", " ")),
		"TextCompanies":            textifyCompanies(loc.Name, jobPosts, jobPosts),
		"TextJobTitles":            textifyJobTitles(jobPosts),
		"TextJobCount":             textifyJobCount(totalCompaniesCount),
		"CurrentPage":              pageID,
		"ShowPage":                 showPage,
		"PageSize":                 s.cfg.CompaniesPerPage,
		"PageIndexes":              pages,
		"TotalCompaniesCount":      totalCompaniesCount,
		"ComplementaryRemote":      complementaryRemote,
		"MonthAndYear":             time.Now().UTC().Format("January 2006"),
		"Country":                  loc.Country,
		"Region":                   loc.Region,
		"Population":               loc.Population,
		"LastJobPostedAt":          lastJobPostedAt,
		"LastJobPostedAtHumanized": lastJobPostedAtHumanized,
		"EmailSubscribersCount":    humanize.Comma(int64(emailSubscribersCount)),
	})
}

func (s Server) RenderPageForLocationAndTagAdmin(w http.ResponseWriter, jobRepo *job.Repository, location, tag, page, salary, currency, htmlView string) {
	showPage := true
	if page == "" {
		page = "1"
		showPage = false
	}
	salaryInt, err := strconv.Atoi(salary)
	if err != nil {
		salaryInt = 0
	}
	salaryInt = int(salaryInt)
	tag = strings.TrimSpace(tag)
	location = strings.TrimSpace(location)
	reg, err := regexp.Compile("[^a-zA-Z0-9\\s]+")
	if err != nil {
		s.Log(err, "unable to compile regex (this should never happen)")
	}
	tag = reg.ReplaceAllString(tag, "")
	location = reg.ReplaceAllString(location, "")
	pageID, err := strconv.Atoi(page)
	if err != nil {
		pageID = 1
		showPage = false
	}
	var pinnedJobs []*job.JobPost
	pinnedJobs, err = jobRepo.GetPinnedJobs()
	if err != nil {
		s.Log(err, "unable to get pinned jobs")
	}
	var pendingJobs []*job.JobPost
	pendingJobs, err = jobRepo.GetPendingJobs()
	if err != nil {
		s.Log(err, "unable to get pending jobs")
	}
	for i, j := range pendingJobs {
		pendingJobs[i].SalaryRange = fmt.Sprintf("%s%s to %s%s", j.SalaryCurrency, humanize.Comma(j.SalaryMin), j.SalaryCurrency, humanize.Comma(j.SalaryMax))
	}
	jobsForPage, totalJobCount, err := jobRepo.JobsByQuery(location, tag, pageID, salaryInt, currency, s.cfg.JobsPerPage, false)
	if err != nil {
		s.Log(err, "unable to get jobs by query")
		s.JSON(w, http.StatusInternalServerError, "Oops! An internal error has occurred")
		return
	}
	var complementaryRemote bool
	if len(jobsForPage) == 0 {
		complementaryRemote = true
		jobsForPage, totalJobCount, err = jobRepo.JobsByQuery("Remote", tag, pageID, salaryInt, currency, s.cfg.JobsPerPage, false)
		if len(jobsForPage) == 0 {
			jobsForPage, totalJobCount, err = jobRepo.JobsByQuery("Remote", "", pageID, salaryInt, currency, s.cfg.JobsPerPage, false)
		}
	}
	if err != nil {
		s.Log(err, "unable to retrieve jobs by query")
		s.JSON(w, http.StatusInternalServerError, "Oops! An internal error has occurred")
		return
	}
	pages := []int{}
	pageLinksPerPage := 8
	pageLinkShift := ((pageLinksPerPage / 2) + 1)
	firstPage := 1
	if pageID-pageLinkShift > 0 {
		firstPage = pageID - pageLinkShift
	}
	for i, j := firstPage, 1; i <= totalJobCount/s.cfg.JobsPerPage+1 && j <= pageLinksPerPage; i, j = i+1, j+1 {
		pages = append(pages, i)
	}
	for i, j := range jobsForPage {
		jobsForPage[i].JobDescription = string(s.tmpl.MarkdownToHTML(j.JobDescription))
		jobsForPage[i].Perks = string(s.tmpl.MarkdownToHTML(j.Perks))
		jobsForPage[i].SalaryRange = fmt.Sprintf("%s%s to %s%s", j.SalaryCurrency, humanize.Comma(j.SalaryMin), j.SalaryCurrency, humanize.Comma(j.SalaryMax))
		jobsForPage[i].InterviewProcess = string(s.tmpl.MarkdownToHTML(j.InterviewProcess))
		if s.IsEmail(j.HowToApply) {
			jobsForPage[i].IsQuickApply = true
		}
	}
	for i, j := range pinnedJobs {
		pinnedJobs[i].JobDescription = string(s.tmpl.MarkdownToHTML(j.JobDescription))
		pinnedJobs[i].Perks = string(s.tmpl.MarkdownToHTML(j.Perks))
		pinnedJobs[i].SalaryRange = fmt.Sprintf("%s%s to %s%s", j.SalaryCurrency, humanize.Comma(j.SalaryMin), j.SalaryCurrency, humanize.Comma(j.SalaryMax))
		pinnedJobs[i].InterviewProcess = string(s.tmpl.MarkdownToHTML(j.InterviewProcess))
		if s.IsEmail(j.HowToApply) {
			pinnedJobs[i].IsQuickApply = true
		}
	}

	s.Render(w, http.StatusOK, htmlView, map[string]interface{}{
		"Jobs":                jobsForPage,
		"PinnedJobs":          pinnedJobs,
		"PendingJobs":         pendingJobs,
		"JobsMinusOne":        len(jobsForPage) - 1,
		"LocationFilter":      location,
		"TagFilter":           tag,
		"CurrentPage":         pageID,
		"ShowPage":            showPage,
		"PageSize":            s.cfg.JobsPerPage,
		"PageIndexes":         pages,
		"TotalJobCount":       totalJobCount,
		"ComplementaryRemote": complementaryRemote,
		"MonthAndYear":        time.Now().UTC().Format("January 2006"),
	})
}

func (s Server) RenderPostAJobForLocation(w http.ResponseWriter, r *http.Request, companyRepo *company.Repository, jobRepo *job.Repository, location string) {
	currency, err := s.GetCurrencyFromRequest(r)
	if err != nil {
		s.Log(err, "could not find ip address in x-forwarded-for, defaulting currency to USD")
	}
	pageviewsLast30Days, err := database.GetWebsitePageViewsLast30Days(s.Conn)
	if err != nil {
		s.Log(err, "could not retrieve pageviews for last 30 days")
		pageviewsLast30Days = 100000
	}
	jobPageviewsLast30Days, err := database.GetJobPageViewsLast30Days(s.Conn)
	if err != nil {
		s.Log(err, "could not retrieve job pageviews for last 30 days")
		jobPageviewsLast30Days = 25000
	}
	jobApplicantsLast30Days, err := database.GetJobClickoutsLast30Days(s.Conn)
	if err != nil {
		s.Log(err, "could not retrieve job clickouts for last 30 days")
		jobApplicantsLast30Days = 8000
	}
	featuredCompanies, err := companyRepo.FeaturedCompaniesPostAJob()
	if err != nil {
		s.Log(err, "could not retrieve featured companies for post a job page")
	}
	lastJobPosted, err := jobRepo.LastJobPosted()
	if err != nil {
		s.Log(err, "could not retrieve last job posted at")
		lastJobPosted = time.Now().AddDate(0, 0, -1)
	}
	newJobsLastWeek, newJobsLastMonth, err := jobRepo.NewJobsLastWeekOrMonth()
	if err != nil {
		s.Log(err, "unable to retrieve new jobs last week last month")
		newJobsLastWeek = 1
	}
	s.Render(w, http.StatusOK, "post-a-job.html", map[string]interface{}{
		"Location":                 location,
		"Currency":                 currency,
		"PageviewsLastMonth":       humanize.Comma(int64(pageviewsLast30Days)),
		"JobPageviewsLastMonth":    humanize.Comma(int64(jobPageviewsLast30Days)),
		"JobApplicantsLastMonth":   humanize.Comma(int64(jobApplicantsLast30Days)),
		"FeaturedCompanies":        featuredCompanies,
		"FeaturedCompaniesNames":   textifyCompanyNames(featuredCompanies, 10),
		"LastJobPostedAtHumanized": humanize.Time(lastJobPosted),
		"LastJobPostedAt":          lastJobPosted.Format(time.RFC3339),
		"NewJobsLastWeek":          newJobsLastWeek,
		"NewJobsLastMonth":         newJobsLastMonth,
		"StripePublishableKey":     s.GetConfig().StripePublishableKey,
	})
}

func (s Server) GetCurrencyFromRequest(r *http.Request) (ipgeolocation.Currency, error) {
	currency := ipgeolocation.Currency{ipgeolocation.CurrencyUSD, "$"}
	ip, err := ipgeolocation.GetIPFromRequest(r)
	if err != nil {
		return currency, err
	}
	currency, err = s.ipGeoLocation.GetCurrencyForIP(ip)
	if err != nil {
		s.Log(err, fmt.Sprintf("unable to retrieve currency for ip addr %+v", ip))
	}
	return s.ipGeoLocation.GetCurrencyForIP(ip)
}

func (s Server) Render(w http.ResponseWriter, status int, htmlView string, data interface{}) error {
	dataMap := make(map[string]interface{}, 0)
	if data != nil {
		dataMap = data.(map[string]interface{})
	}
	dataMap["SiteName"] = s.GetConfig().SiteName
	dataMap["SiteJobCategory"] = strings.Title(strings.ToLower(s.GetConfig().SiteJobCategory))
	dataMap["SiteJobCategoryURLEncoded"] = strings.ReplaceAll(strings.Title(strings.ToLower(s.GetConfig().SiteJobCategory)), " ", "-")
	dataMap["SupportEmail"] = s.GetConfig().SupportEmail
	dataMap["SiteHost"] = s.GetConfig().SiteHost

	return s.tmpl.Render(w, status, htmlView, dataMap)
}

func (s Server) XML(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	w.Write(data)
}

func (s Server) JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func (s Server) TEXT(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	w.Write([]byte(text))
}

func (s Server) MEDIA(w http.ResponseWriter, status int, media []byte, mediaType string) {
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "max-age=31536000")
	w.WriteHeader(status)
	w.Write(media)
}

func (s Server) Log(err error, msg string) {
	raven.CaptureErrorAndWait(err, map[string]string{"ctx": msg})
	log.Printf("%s: %+v", msg, err)
}

func (s Server) GetEmail() email.Client {
	return s.emailClient
}

func (s Server) Redirect(w http.ResponseWriter, r *http.Request, status int, dst string) {
	http.Redirect(w, r, dst, status)
}

func (s Server) Run() error {
	addr := fmt.Sprintf(":%s", s.cfg.Port)
	if s.cfg.Env == "dev" {
		log.Printf("local env http://localhost:%s", s.cfg.Port)
		addr = fmt.Sprintf("localhost:%s", s.cfg.Port)
	}
	return http.ListenAndServe(
		addr,
		middleware.HTTPSMiddleware(
			middleware.GzipMiddleware(
				middleware.LoggingMiddleware(middleware.HeadersMiddleware(s.router, s.cfg.Env)),
			),
			s.cfg.Env,
		),
	)
}

func (s Server) GetJWTSigningKey() []byte {
	return s.cfg.JwtSigningKey
}

func (s Server) CacheGet(key string) ([]byte, bool) {
	out, err := s.bigCache.Get(key)
	if err != nil {
		return []byte{}, false
	}
	return out, true
}

func (s Server) CacheSet(key string, val []byte) error {
	return s.bigCache.Set(key, val)
}

func (s Server) CacheDelete(key string) error {
	return s.bigCache.Delete(key)
}

func (s Server) SeenSince(r *http.Request, timeAgo time.Duration) bool {
	ipAddrs := strings.Split(r.Header.Get("x-forwarded-for"), ", ")
	if len(ipAddrs) == 0 {
		return false
	}
	lastSeen, err := s.bigCache.Get(ipAddrs[0])
	if err == bigcache.ErrEntryNotFound {
		s.bigCache.Set(ipAddrs[0], []byte(time.Now().Format(time.RFC3339)))
		return false
	}
	if err != nil {
		return false
	}
	lastSeenTime, err := time.Parse(time.RFC3339, string(lastSeen))
	if err != nil {
		s.bigCache.Set(ipAddrs[0], []byte(time.Now().Format(time.RFC3339)))
		return false
	}
	if !lastSeenTime.After(time.Now().Add(-timeAgo)) {
		s.bigCache.Set(ipAddrs[0], []byte(time.Now().Format(time.RFC3339)))
		return false
	}

	return true
}

func (s Server) IsEmail(val string) bool {
	return s.emailRe.MatchString(val)
}
