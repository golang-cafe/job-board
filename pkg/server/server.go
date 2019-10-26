package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	stdtemplate "html/template"

	"github.com/0x13a/golang.cafe/pkg/affiliate"
	"github.com/0x13a/golang.cafe/pkg/config"
	"github.com/0x13a/golang.cafe/pkg/database"
	"github.com/0x13a/golang.cafe/pkg/email"
	"github.com/0x13a/golang.cafe/pkg/ipgeolocation"
	"github.com/0x13a/golang.cafe/pkg/middleware"
	"github.com/0x13a/golang.cafe/pkg/obfuscator"
	"github.com/0x13a/golang.cafe/pkg/template"
	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"

	raven "github.com/getsentry/raven-go"
)

type Server struct {
	cfg           config.Config
	Conn          *sql.DB
	router        *mux.Router
	tmpl          *template.Template
	emailClient   email.Client
	ipGeoLocation ipgeolocation.IPGeoLocation
	SessionStore  *sessions.CookieStore
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
	
	return Server{
		cfg:           cfg,
		Conn:          conn,
		router:        r,
		tmpl:          t,
		emailClient:   emailClient,
		ipGeoLocation: ipGeoLocation,
		SessionStore:  sessionStore,
	}
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

func (s Server) RenderSalaryForLocation(w http.ResponseWriter, r *http.Request, location string) {
	loc, currency, country, err := database.GetLocation(s.Conn, location)
	if err != nil {
		s.Log(err, fmt.Sprintf("unable to retrieve location %s from db, err: %#v", location, err))
		s.JSON(w, http.StatusBadRequest, map[string]string{"status": "error"})
		return
	}
	set, err := database.GetSalaryDataForLocationAndCurrency(s.Conn, loc, currency)
	if err != nil {
		s.Log(err, fmt.Sprintf("unable to retrieve salary stats for location %s and currency %s, err: %#v", location, currency, err))
		s.JSON(w, http.StatusInternalServerError, map[string]string{"status": "error"})
		return
	}
	complimentaryRemote := false
	if len(set) < 1 {
		complimentaryRemote = true
		set, err = database.GetSalaryDataForLocationAndCurrency(s.Conn, "Remote", "$")
		if err != nil {
			s.Log(err, fmt.Sprintf("unable to retrieve salary stats for location %s and currency %s, err: %#v", location, currency, err))
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
	s.Render(w, http.StatusOK, "salary-explorer.html", map[string]interface{}{
		"Location":            strings.ReplaceAll(location, "-", " "),
		"Currency":            currency,
		"DataSet":             string(jsonRes),
		"Country":             country,
		"ComplimentaryRemote": complimentaryRemote,
	})
}

func (s Server) RenderPageForLocationAndTag(w http.ResponseWriter, location, tag, page, htmlView string) {
	showPage := true
	if page == "" {
		page = "1"
		showPage = false
	}
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
	var pinnedJobs []*database.JobPost
	pinnedJobs, err = database.GetPinnedJobs(s.Conn)
	if err != nil {
		s.Log(err, "unable to get pinned jobs")
	}
	jobsForPage, totalJobCount, err := database.JobsByQuery(s.Conn, location, tag, pageID, s.cfg.JobsPerPage)
	if err != nil {
		s.Log(err, "unable to get jobs by query")
		s.JSON(w, http.StatusInternalServerError, "Oops! An internal error has occurred")
		return
	}
	var complementaryRemote bool
	if len(jobsForPage) == 0 {
		complementaryRemote = true
		jobsForPage, totalJobCount, err = database.JobsByQuery(s.Conn, "Remote", tag, pageID, s.cfg.JobsPerPage)
		if len(jobsForPage) == 0 {
			jobsForPage, totalJobCount, err = database.JobsByQuery(s.Conn, "Remote", "", pageID, s.cfg.JobsPerPage)
		}
	}
	if err != nil {
		s.Log(err, "unable to retrieve jobs by query")
		s.JSON(w, http.StatusInternalServerError, "Oops! An internal error has occurred")
		return
	}
	pages := []int{}
	pageLinksPerPage := 10
	pageLinkShift := ((pageLinksPerPage/2)+1)
	firstPage := 1
	if pageID - pageLinkShift > 0 {
		firstPage = pageID - pageLinkShift
	}
	for i, j := firstPage, 1; i <= totalJobCount/s.cfg.JobsPerPage+1 && j <= pageLinksPerPage; i, j = i+1, j+1 {
		pages = append(pages, i)
	}
	jobTrackIDs := make(map[int]string, len(jobsForPage))
	for i, j := range jobsForPage {
		jobsForPage[i].JobDescription = string(s.tmpl.MarkdownToHTML(j.JobDescription))
		jobsForPage[i].Perks = string(s.tmpl.MarkdownToHTML(j.Perks))
		jobsForPage[i].InterviewProcess = string(s.tmpl.MarkdownToHTML(j.InterviewProcess))
		encryptedID, err := obfuscator.ObfuscateInt(j.ID)
		if err != nil {
			continue
		}
		jobTrackIDs[j.ID] = encryptedID
	}
	for i, j := range pinnedJobs {
		pinnedJobs[i].JobDescription = string(s.tmpl.MarkdownToHTML(j.JobDescription))
		pinnedJobs[i].Perks = string(s.tmpl.MarkdownToHTML(j.Perks))
		pinnedJobs[i].InterviewProcess = string(s.tmpl.MarkdownToHTML(j.InterviewProcess))
		encryptedID, err := obfuscator.ObfuscateInt(j.ID)
		if err != nil {
			continue
		}
		jobTrackIDs[j.ID] = encryptedID
	}

	s.Render(w, http.StatusOK, htmlView, map[string]interface{}{
		"Jobs":                jobsForPage,
		"JobTrackIDs":         jobTrackIDs,
		"PinnedJobs":          pinnedJobs,
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

func (s Server) RenderPostAJobForLocation(w http.ResponseWriter, r *http.Request, location string) {
	affiliateRef := r.URL.Query().Get("ref")
	affiliateRefCookie, err := r.Cookie(affiliate.PostAJobAffiliateRefCookie)
	var affiliateRefCookieVal string
	if err == nil {
		affiliateRefCookieVal = affiliateRefCookie.String()
	}
	// not in cookie not in rq
	if affiliateRef == "" && affiliateRefCookieVal == "" {
		affiliateRef = affiliate.DefaultAffiliateID
	}
	// in cookie not in rq
	if affiliateRef == "" && affiliateRefCookieVal != "" {
		affiliateRef = affiliateRefCookieVal
	}
	if affiliate.ValidAffiliateRef(affiliateRef) {
		err := database.SaveAffiliatePostAJobView(s.Conn, affiliateRef)
		if err != nil {
			s.Log(err, fmt.Sprintf("unable to save affiliate %s post a job view", affiliateRef))
		}
		// if cookie is already present we don't set Cookie
		// if cookie is same as ref coming from the req we don't set cookie
		if affiliateRefCookieVal != r.URL.Query().Get("ref") && affiliateRefCookieVal == "" {
			thirtyDays := time.Now().Add(30 * 24 * time.Hour)
			cookie := http.Cookie{Name: affiliate.PostAJobAffiliateRefCookie, Value: affiliateRef, Expires: thirtyDays}
			http.SetCookie(w, &cookie)
		}
	}
	ipAddrs := strings.Split(r.Header.Get("x-forwarded-for"), ", ")
	currency := ipgeolocation.Currency{ipgeolocation.CurrencyUSD, "$"}
	if len(ipAddrs) > 0 {
		currency, err = s.ipGeoLocation.GetCurrencyForIP(ipAddrs[0])
		if err != nil {
			s.Log(err, fmt.Sprintf("unable to retrieve currency for ip addr %+v", ipAddrs[0]))
		}
	} else {
		s.Log(errors.New("coud not find ip address in x-forwarded-for"), "could not find ip address in x-forwarded-for, defaulting currency to USD")
	}
	s.Render(w, http.StatusOK, "post-a-job.html", map[string]interface{}{
		"Location":             location,
		"Currency":             currency,
		"Ref":                  affiliateRef,
		"StripePublishableKey": s.GetConfig().StripePublishableKey,
	})
}

func (s Server) Render(w http.ResponseWriter, status int, htmlView string, data interface{}) error {
	return s.tmpl.Render(w, status, htmlView, data)
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

func (s Server) MEDIA(w http.ResponseWriter, status int, media database.Media, mediaID string) {
	w.Header().Set("Content-Type", media.MediaType)
	w.Header().Set("Cache-Control", "max-age=31536000")
	w.WriteHeader(status)
	w.Write(media.Bytes)
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
	return http.ListenAndServe(
		fmt.Sprintf(":%s", s.cfg.Port),
		middleware.HTTPSMiddleware(
			middleware.GzipMiddleware(
				middleware.HeadersMiddleware(s.router, s.cfg.Env),
			),
			s.cfg.Env,
		),
	)
}

func (s Server) GetJWTSigningKey() []byte {
	return s.cfg.JwtSigningKey
}
