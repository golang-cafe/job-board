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
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ChimeraCoder/anaconda"
	"github.com/PuerkitoBio/goquery"
	"github.com/bot-api/telegram"
	"github.com/dgrijalva/jwt-go"
	"github.com/dustin/go-humanize"
	"github.com/gorilla/feeds"
	"github.com/gorilla/mux"
	"github.com/gosimple/slug"
	"github.com/machinebox/graphql"
	"github.com/microcosm-cc/bluemonday"
	"github.com/nfnt/resize"
	"github.com/segmentio/ksuid"
	"github.com/snabb/sitemap"

	"github.com/golang-cafe/job-board/internal/blog"
	"github.com/golang-cafe/job-board/internal/bookmark"
	"github.com/golang-cafe/job-board/internal/company"
	"github.com/golang-cafe/job-board/internal/database"
	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/email"
	"github.com/golang-cafe/job-board/internal/imagemeta"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/payment"
	"github.com/golang-cafe/job-board/internal/recruiter"
	"github.com/golang-cafe/job-board/internal/seo"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/golang-cafe/job-board/internal/user"
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
	SaveTokenSignOn(email, token, userType string) error
}

func GetAuthPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile, _ := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
		if profile != nil {
			svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("%s%s/", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost))
			return
		}
		email := r.URL.Query().Get("email")
		svr.Render(r, w, http.StatusOK, "auth.html", map[string]interface{}{
			"DefaultEmail": email,
		})
	}
}

func CompaniesHandler(svr server.Server, companyRepo *company.Repository, jobRepo *job.Repository, devRepo *developer.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		location := vars["location"]
		page := r.URL.Query().Get("p")
		svr.RenderPageForCompanies(w, r, companyRepo, jobRepo, devRepo, location, page, "companies.html")
	}
}

func DevelopersHandler(svr server.Server, devRepo *developer.Repository, recruiterRepo *recruiter.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		location := vars["location"]
		tag := vars["tag"]
		page := r.URL.Query().Get("p")
		profile, _ := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
		if profile != nil && profile.Type == "recruiter" {
			expTime, err := recruiterRepo.RecruiterProfilePlanExpiration(profile.Email)
			if err == nil && expTime.Before(time.Now().UTC()) {
				svr.Redirect(w, r, http.StatusTemporaryRedirect, fmt.Sprintf("%s%s/profile/home#developer-subscription", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost))
				fmt.Println(expTime)
				return
			}
		}
		svr.RenderPageForDevelopers(w, r, devRepo, location, tag, page, "developers.html")
	}
}

func SubmitDeveloperProfileHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile, _ := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
		if profile != nil {
			routeName := fmt.Sprintf("%s-Developers", strings.Title(svr.GetConfig().SiteJobCategory))
			svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("%s%s/%s", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, routeName))
			return
		}
		svr.RenderPageForProfileRegistration(w, r, devRepo, "submit-developer-profile.html")
	}
}

func SubmitRecruiterProfileHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profile, _ := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
		if profile != nil {
			routeName := fmt.Sprintf("%s-Developers", strings.Title(svr.GetConfig().SiteJobCategory))
			svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("%s%s/%s", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, routeName))
			return
		}
		svr.RenderPageForProfileRegistration(w, r, devRepo, "submit-recruiter-profile.html")
	}
}

func SaveRecruiterProfileHandler(svr server.Server, recRepo *recruiter.Repository, userRepo tokenSaver, paymentRepo *payment.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req := &struct {
			Fullname     string `json:"fullname"`
			CompanyURL   string `json:"company_url"`
			Email        string `json:"email"`
			PlanDuration int    `json:"plan_duration"`
			ItemPrice    int    `json:"item_price"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			svr.JSON(w, http.StatusBadRequest, "request is invalid")
			return
		}
		if !svr.IsEmail(req.Email) {
			svr.JSON(w, http.StatusBadRequest, "email is invalid")
			return
		}
		for _, e := range []string{"gmail.com", "outlook.com", "live.com", "yahoo.com", "icloud.com"} {
			if strings.Contains(req.Email, e) {
				svr.JSON(w, http.StatusBadRequest, "email must be a valid company email")
				return
			}
		}
		req.Fullname = strings.Title(strings.ToLower(bluemonday.StrictPolicy().Sanitize(req.Fullname)))
		existingRec, err := recRepo.RecruiterProfileByEmail(req.Email)
		if err != nil {
			svr.Log(err, "unable to create profile")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		if existingRec.Email == req.Email {
			svr.JSON(w, http.StatusBadRequest, "recruiter profile with this email already exists")
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate token")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		t := time.Now().UTC()
		rec := recruiter.Recruiter{
			ID:         k.String(),
			Name:       req.Fullname,
			CompanyURL: req.CompanyURL,
			CreatedAt:  t,
			UpdatedAt:  t,
			Email:      strings.ToLower(req.Email),
		}
		err = userRepo.SaveTokenSignOn(strings.ToLower(req.Email), k.String(), user.UserTypeRecruiter)
		if err != nil {
			svr.Log(err, "unable to save sign on token")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		err = recRepo.SaveRecruiterProfile(rec)
		if err != nil {
			svr.Log(err, "unable to save recruiter profile")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		err = svr.GetEmail().SendHTMLEmail(
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			email.Address{Email: req.Email},
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			fmt.Sprintf("Verify Your Recruiter Profile on %s", svr.GetConfig().SiteName),
			fmt.Sprintf(
				"Verify Your Recruiter Profile on %s %s%s/x/auth/%s",
				svr.GetConfig().SiteName,
				svr.GetConfig().URLProtocol,
				svr.GetConfig().SiteHost,
				k.String(),
			),
		)
		if err != nil {
			svr.Log(err, "unable to send email while submitting recruiter profile")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		sess, err := paymentRepo.CreateDevDirectorySession(rec.Email, rec.ID, int64(req.ItemPrice*100), int64(req.PlanDuration), false)
		if err != nil {
			svr.Log(err, "unable to create payment session")
		}
		err = svr.GetEmail().SendHTMLEmail(
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			email.Address{Email: svr.GetEmail().DefaultAdminAddress()},
			email.Address{Email: req.Email},
			fmt.Sprintf("New Dev Directory Subscriber on %s", svr.GetConfig().SiteName),
			fmt.Sprintf(
				"Hey! There is a new Developer Directory Subscription on %s. Developer Directory Subscription %d Months Plan @ US$%d/month, Email: %s, Company: %s",
				svr.GetConfig().SiteName,
				req.PlanDuration,
				req.ItemPrice,
				rec.Email,
				rec.CompanyURL,
			),
		)
		if err != nil {
			svr.Log(err, "unable to send email to admin while creating subscription")
		}
		if sess != nil {
			err = database.InitiatePaymentEventForDeveloperDirectoryAccess(
				svr.Conn,
				sess.ID,
				int64(req.ItemPrice*req.PlanDuration*100),
				fmt.Sprintf("Developer Directory Subscription %d Months Plan @ US$%d/month", req.PlanDuration, req.ItemPrice),
				rec.ID,
				rec.Email,
				int64(req.PlanDuration),
			)
			if err != nil {
				svr.Log(err, "unable to save payment initiated event")
			}
			svr.JSON(w, http.StatusOK, map[string]string{"s_id": sess.ID})
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func SaveDeveloperMetadataHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			req := &struct {
				DeveloperProfileID string  `json:"developer_profile_id"`
				MetadataType       string  `json:"metadata_type"`
				Title              string  `json:"title"`
				Description        string  `json:"description"`
				Link               *string `json:"link,omitempty"`
			}{}

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				svr.Log(errors.New("invalid developer metadata"), "invalid developer metadata")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			dev, err := devRepo.DeveloperProfileByID(req.DeveloperProfileID)
			if !profile.IsAdmin && dev.Email != profile.Email {
				svr.Log(err, "Only same user or admin can edit metadata.")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			req.Title = strings.Title(strings.ToLower(bluemonday.StrictPolicy().Sanitize(req.Title)))
			req.Description = bluemonday.StrictPolicy().Sanitize(req.Description)
			k, err := ksuid.NewRandom()
			if err != nil {
				svr.Log(err, "unable to generate token")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}

			devMetadata := developer.DeveloperMetadata{
				ID:                 k.String(),
				DeveloperProfileID: req.DeveloperProfileID,
				MetadataType:       req.MetadataType,
				Title:              req.Title,
				Description:        req.Description,
				Link:               req.Link,
			}
			err = devRepo.SaveDeveloperMetadata(devMetadata)
			if err != nil {
				svr.Log(err, "unable to save developer metadata")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func DeleteDeveloperMetadataHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			req := &struct {
				ID                 string `json:"id"`
				DeveloperProfileID string `json:"developer_profile_id"`
			}{}

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				svr.Log(errors.New("invalid developer metadata ID"), "invalid developer metadata ID")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			dev, err := devRepo.DeveloperProfileByID(req.DeveloperProfileID)
			if err != nil {
				svr.Log(err, "unable to get user from profileID")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			if dev.Email != profile.Email && !profile.IsAdmin {
				svr.Log(err, "Only same user or admin can edit metadata.")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			err = devRepo.DeleteDeveloperMetadata(req.ID, req.DeveloperProfileID)
			if err != nil {
				svr.Log(err, "unable to delete developer metadata")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func UpdateDeveloperMetadataHandler(svr server.Server, devRepo *developer.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			req := &struct {
				ID                 string  `json:"id"`
				DeveloperProfileID string  `json:"developer_profile_id"`
				MetadataType       string  `json:"metadata_type"`
				Title              string  `json:"title"`
				Description        string  `json:"description"`
				Link               *string `json:"link,omitempty"`
			}{}

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				svr.Log(errors.New("invalid developer metadata"), "invalid developer metadata")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			dev, err := devRepo.DeveloperProfileByID(req.DeveloperProfileID)
			if err != nil {
				svr.Log(err, "unable to get user from profileID")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			if dev.Email != profile.Email && !profile.IsAdmin {
				svr.Log(err, "Only same user or admin can edit metadata.")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			req.Title = strings.Title(strings.ToLower(bluemonday.StrictPolicy().Sanitize(req.Title)))
			req.Description = bluemonday.StrictPolicy().Sanitize(req.Description)

			devMetadata := developer.DeveloperMetadata{
				ID:                 req.ID,
				DeveloperProfileID: req.DeveloperProfileID,
				MetadataType:       req.MetadataType,
				Title:              req.Title,
				Description:        req.Description,
				Link:               req.Link,
			}
			err = devRepo.UpdateDeveloperMetadata(devMetadata)
			if err != nil {
				svr.Log(err, "unable to save developer metadata")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func SaveDeveloperProfileHandler(svr server.Server, devRepo devGetSaver, userRepo tokenSaver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req := &struct {
			Fullname           string   `json:"fullname"`
			HourlyRate         string   `json:"hourly_rate"`
			LinkedinURL        string   `json:"linkedin_url"`
			CurrentLocation    string   `json:"current_location"`
			GithubURL          *string  `json:"github_url,omitempty"`
			TwitterURL         *string  `json:"twitter_url,omitempty"`
			Bio                string   `json:"bio"`
			Tags               string   `json:"tags"`
			ProfileImageID     string   `json:"profile_image_id"`
			Email              string   `json:"email"`
			SearchStatus       string   `json:"search_status"`
			RoleLevel          string   `json:"role_level"`
			RoleTypes          []string `json:"role_types"`
			DetectedLocationID string   `json:"detected_location_id"`
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
		if _, ok := developer.ValidSearchStatus[req.SearchStatus]; !ok {
			svr.JSON(w, http.StatusBadRequest, "invalid search status")
			return
		}
		if _, ok := developer.ValidRoleLevels[req.RoleLevel]; !ok {
			svr.JSON(w, http.StatusBadRequest, "invalid role level")
			return
		}
		for _, v := range req.RoleTypes {
			if _, ok := developer.ValidRoleTypes[v]; !ok {
				svr.JSON(w, http.StatusBadRequest, "invalid role type")
				return
			}
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
		detectedLocationID := &req.DetectedLocationID
		if req.DetectedLocationID == "" {
			svr.Log(err, "detected location should be set")
			svr.JSON(w, http.StatusBadRequest, "detected_location_id should be set")
			return
		}
		if req.HourlyRate == "" || req.HourlyRate == "0" {
			svr.JSON(w, http.StatusBadRequest, "Please specify hourly rate")
			return
		}
		hourlyRate, err := strconv.ParseInt(req.HourlyRate, 10, 64)
		if err != nil {
			svr.Log(err, "unable to parse string to int")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}

		if hourlyRate > 1000 && hourlyRate < 0 {
			svr.Log(err, "Hourly rate cannot be more than 1000 or less than 0")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}

		dev := developer.Developer{
			ID:                 k.String(),
			Name:               req.Fullname,
			Location:           req.CurrentLocation,
			HourlyRate:         hourlyRate,
			LinkedinURL:        req.LinkedinURL,
			GithubURL:          req.GithubURL,
			TwitterURL:         req.TwitterURL,
			Bio:                req.Bio,
			Available:          true,
			CreatedAt:          t,
			UpdatedAt:          t,
			Email:              strings.ToLower(req.Email),
			ImageID:            req.ProfileImageID,
			Skills:             req.Tags,
			SearchStatus:       req.SearchStatus,
			RoleTypes:          req.RoleTypes,
			RoleLevel:          req.RoleLevel,
			DetectedLocationID: detectedLocationID,
		}
		err = userRepo.SaveTokenSignOn(strings.ToLower(req.Email), k.String(), user.UserTypeDeveloper)
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
		err = database.AddEmailSubscriber(svr.Conn, req.Email, k.String())
		if err != nil {
			svr.Log(err, "unable to add email subscriber to db")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		err = svr.GetEmail().SendHTMLEmail(
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			email.Address{Email: req.Email},
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			fmt.Sprintf("Verify Your Developer Profile on %s", svr.GetConfig().SiteName),
			fmt.Sprintf(
				"Verify Your Developer Profile on %s %s%s/x/auth/%s",
				svr.GetConfig().SiteName,
				svr.GetConfig().URLProtocol,
				svr.GetConfig().SiteHost,
				k.String(),
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
					req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.currencyapi.com/v3/latest?apikey=%s&base_currency=%s", svr.GetConfig().FXAPIKey, base), nil)
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
						Rates map[string]struct {
							Code  string  `json:"code"`
							Value float64 `json:"value"`
						} `json:"data"`
					}
					defer res.Body.Close()
					if err := json.NewDecoder(res.Body).Decode(&ratesResponse); err != nil {
						svr.Log(err, "json.NewDecoder(res.Body).Decode(ratesResponse)")
						continue
					}
					log.Printf("rate response for currency %s: %#v", base, ratesResponse)
					for _, target := range svr.GetConfig().AvailableCurrencies {
						if target == base {
							continue
						}
						cur, ok := ratesResponse.Rates[target]
						if !ok {
							svr.Log(errors.New("could not find target currency"), fmt.Sprintf("could not find target currency %s for base %s", target, base))
							continue
						}
						log.Println("updating fx rate pair ", base, target, cur.Code, cur.Value)
						fx := database.FXRate{
							Base:      base,
							UpdatedAt: time.Now(),
							Target:    target,
							Value:     cur.Value,
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

func TriggerSitemapUpdate(svr server.Server, devRepo *developer.Repository, jobRepo *job.Repository, blogRepo *blog.Repository, companyRepo *company.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				fmt.Println("generating sitemap")
				database.SaveSEOSkillFromCompany(svr.Conn)
				landingPages, err := seo.GenerateSearchSEOLandingPages(svr.Conn, svr.GetConfig().SiteJobCategory)
				if err != nil {
					svr.Log(err, "seo.GenerateSearchSEOLandingPages")
					return
				}
				fmt.Println("generating post a job landing page")
				postAJobLandingPages, err := seo.GeneratePostAJobSEOLandingPages(svr.Conn, svr.GetConfig().SiteJobCategory)
				if err != nil {
					svr.Log(err, "seo.GeneratePostAJobSEOLandingPages")
					return
				}
				fmt.Println("generating salary landing page")
				salaryLandingPages, err := seo.GenerateSalarySEOLandingPages(svr.Conn, svr.GetConfig().SiteJobCategory)
				if err != nil {
					svr.Log(err, "seo.GenerateSalarySEOLandingPages")
					return
				}
				fmt.Println("generating companies landing page")
				companyLandingPages, err := seo.GenerateCompaniesLandingPages(svr.Conn, svr.GetConfig().SiteJobCategory)
				if err != nil {
					svr.Log(err, "seo.GenerateCompaniesLandingPages")
					return
				}
				fmt.Println("generating dev skill landing pages")
				developerSkillsPages, err := seo.GenerateDevelopersSkillLandingPages(devRepo, svr.GetConfig().SiteJobCategory)
				if err != nil {
					svr.Log(err, "seo.GenerateDevelopersSkillLandingPages")
					return
				}
				fmt.Println("generating dev profile landing pages")
				developerProfilePages, err := seo.GenerateDevelopersProfileLandingPages(devRepo)
				if err != nil {
					svr.Log(err, "seo.GenerateDevelopersProfileLandingPages")
					return
				}
				fmt.Println("generating company profile landing page")
				companyProfilePages, err := seo.GenerateCompanyProfileLandingPages(companyRepo)
				if err != nil {
					svr.Log(err, "seo.GenerateDevelopersProfileLandingPages")
					return
				}
				fmt.Println("generating dev location pages")
				developerLocationPages, err := seo.GenerateDevelopersLocationPages(svr.Conn, svr.GetConfig().SiteJobCategory)
				if err != nil {
					svr.Log(err, "seo.GenerateDevelopersLocationPages")
					return
				}
				fmt.Println("generating blog pages")
				blogPosts, err := seo.BlogPages(blogRepo)
				if err != nil {
					svr.Log(err, "seo.BlogPages")
					return
				}
				fmt.Println("generating static pages")
				pages := seo.StaticPages(svr.GetConfig().SiteJobCategory)
				jobPosts, err := jobRepo.JobPostByCreatedAt()
				if err != nil {
					svr.Log(err, "database.JobPostByCreatedAt")
					return
				}
				n := time.Now().UTC()

				database.CreateTmpSitemapTable(svr.Conn)
				for _, j := range jobPosts {
					fmt.Println("job post page generating...")
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`%s%s/job/%s`, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, j.Slug),
						LastMod:    time.Unix(j.CreatedAt, 0),
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", j.Slug))
					}
				}

				for _, b := range blogPosts {
					fmt.Println("blog post page generating...")
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`%s%s/blog/%s`, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, b.Path),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", b.Path))
					}
				}

				for _, p := range pages {
					fmt.Println("static page generating...")
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`%s%s/%s`, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for i, p := range postAJobLandingPages {
					fmt.Println("post a job landing page generating...", i, len(postAJobLandingPages))
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`%s%s/%s`, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for i, p := range salaryLandingPages {
					fmt.Println("salary landing page generating...", i, len(salaryLandingPages))
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`%s%s/%s`, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for i, p := range landingPages {
					fmt.Println("landing page generating...", i, len(landingPages))
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`%s%s/%s`, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, p.URI),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for _, p := range companyLandingPages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`%s%s/%s`, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for _, p := range developerSkillsPages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`%s%s/%s`, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for _, p := range developerProfilePages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`%s%s/%s`, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}
				for _, p := range companyProfilePages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`%s%s/%s`, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}

				for _, p := range developerLocationPages {
					if err := database.SaveSitemapEntry(svr.Conn, database.SitemapEntry{
						Loc:        fmt.Sprintf(`%s%s/%s`, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, p),
						LastMod:    n,
						ChangeFreq: "weekly",
					}); err != nil {
						svr.Log(err, fmt.Sprintf("database.SaveSitemapEntry: %s", p))
					}
				}
				fmt.Println("swapping sitemap table")
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
// 				lastJobIDStr, err := jobRepo.GetValue("last_sent_job_id_weekly")
// 				if err != nil {
// 					svr.Log(err, "unable to retrieve last newsletter weekly job id")
// 					return
// 				}
// 				lastJobID, err := strconv.Atoi(lastJobIDStr)
// 				if err != nil {
// 					svr.Log(err, fmt.Sprintf("unable to convert job str %s to id", lastJobIDStr))
// 					return
// 				}
// 				jobPosts, err := jobRepo.GetLastNJobsFromID(svr.GetConfig().NewsletterJobsToSend, lastJobID)
// 				if len(jobPosts) < 1 {
// 					log.Printf("found 0 new jobs for weekly newsletter. quitting")
// 					return
// 				}
// 				fmt.Printf("found %d/%d jobs for weekly newsletter\n", len(jobPosts), svr.GetConfig().NewsletterJobsToSend)
// 				subscribers, err := database.GetEmailSubscribers(svr.Conn)
// 				if err != nil {
// 					svr.Log(err, fmt.Sprintf("unable to retrieve subscribers"))
// 					return
// 				}
// 				var jobsHTMLArr []string
// 				for _, j := range jobPosts {
// 					jobsHTMLArr = append(jobsHTMLArr, `Job Title: `+j.JobTitle+`\r\nCompany: `+j.Company+`\r\nLocation: `+j.Location+`\r\nSalary: `+j.SalaryRange+`\r\nDetail: `+svr.GetConfig().URLProtocol+svr.GetConfig().SiteHost+`/job/`+j.Slug)
// 					lastJobID = j.ID
// 				}
// 				jobsHTML := strings.Join(jobsHTMLArr, " ")
// 				campaignContentHTML := `Here's a list of the newest ` + fmt.Sprintf("%d", len(jobPosts)) + ` ` + svr.GetConfig().SiteJobCategory + ` jobs this week on ` + svr.GetConfig().SiteName + `\r\n
// ` + jobsHTML + `
//     Check out more jobs at ` + svr.GetConfig().SiteName + `` + svr.GetConfig().URLProtocol + svr.GetConfig().SiteHost + `
//     Get companies apply to you, join the ` + strings.ToUpper(svr.GetConfig().SiteJobCategory) + ` Developer Community ` + svr.GetConfig().SiteName + `` + svr.GetConfig().URLProtocol + svr.GetConfig().SiteHost + `/Join-` + strings.Title(svr.GetConfig().SiteJobCategory) + `-Community
//     ` + svr.GetConfig().SiteName + `
//     `
// 				unsubscribeLink := `
//     ` + svr.GetConfig().SiteName + ` | London, United Kingdom\r\nThis email was sent to %s | ` + svr.GetConfig().URLProtocol + svr.GetConfig().SiteHost + `/x/email/unsubscribe?token=%s"`

// 				for _, s := range subscribers {
// 					err = svr.GetEmail().SendHTMLEmail(
// 						email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
// 						email.Address{Email: s.Email},
// 						email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
// 						fmt.Sprintf("Go Jobs This Week (%d New)", len(jobPosts)),
// 						campaignContentHTML+fmt.Sprintf(unsubscribeLink, s.Email, s.Token),
// 					)
// 					if err != nil {
// 						svr.Log(err, fmt.Sprintf("unable to send email for newsletter email %s", s.Email))
// 						continue
// 					}
// 				}
// 				lastJobIDStr = strconv.Itoa(lastJobID)
// 				err = jobRepo.SetValue("last_sent_job_id_weekly", lastJobIDStr)
// 				if err != nil {
// 					svr.Log(err, "unable to save last weekly newsletter job id to db")
// 					return
// 				}
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
					_, err := api.SendMessage(ctx, telegram.NewMessage(svr.GetConfig().TelegramChannelID, fmt.Sprintf("%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s%s/job/%s", j.JobTitle, j.Company, j.Location, j.SalaryRange, svr.GetConfig().SiteJobCategory, svr.GetConfig().SiteJobCategory, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, j.Slug)))
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
				highlights := fmt.Sprintf(`This months highlight ✨

📣 %s new jobs posted last month
✉️  %s applicants last month
🌎 %s pageviews last month
💼 %s jobs viewed last month
`, newJobsLastMonthText, jobApplicantsLast30DaysText, pageviewsLast30DaysText, jobPageviewsLast30DaysText)
				err = svr.GetEmail().SendHTMLEmail(
					email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
					email.Address{Email: svr.GetEmail().DefaultAdminAddress()},
					email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
					fmt.Sprintf("%s Monthly Highlights", svr.GetConfig().SiteName),
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
					_, err := api.PostTweet(fmt.Sprintf("%s with %s - %s | %s\n\n#%s #%sjobs\n\n%s%s/job/%s", j.JobTitle, j.Company, j.Location, j.SalaryRange, svr.GetConfig().SiteJobCategory, svr.GetConfig().SiteJobCategory, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, j.Slug), url.Values{})
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
				if err := companyRepo.DeleteStaleImages(svr.GetConfig().SiteLogoImageID); err != nil {
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
			// select * from job where plan_expired_at >= current_date - interval '30' day and plan_expired_at < NOW() and approved_at is not null and company_email != '<support email>'
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
				ID                 string   `json:"id"`
				Fullname           string   `json:"fullname"`
				HourlyRate         string   `json:"hourly_rate"`
				LinkedinURL        string   `json:"linkedin_url"`
				Bio                string   `json:"bio"`
				CurrentLocation    string   `json:"current_location"`
				Skills             string   `json:"skills"`
				ImageID            string   `json:"profile_image_id"`
				Email              string   `json:"email"`
				SearchStatus       string   `json:"search_status"`
				RoleLevel          string   `json:"role_level"`
				RoleTypes          []string `json:"role_types"`
				DetectedLocationID string   `json:"detected_location_id"`
			}{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				svr.Log(errors.New("invalid search status"), "invalid search status")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			if !svr.IsEmail(req.Email) {
				svr.Log(errors.New("invalid search status"), "invalid search status")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			linkedinRe := regexp.MustCompile(`^https:\/\/(?:[a-z]{2,3}\.)?linkedin\.com\/.*$`)
			if !linkedinRe.MatchString(req.LinkedinURL) {
				svr.Log(errors.New("invalid search status"), "invalid search status")
				svr.JSON(w, http.StatusBadRequest, "linkedin url is invalid")
				return
			}
			if _, ok := developer.ValidSearchStatus[req.SearchStatus]; !ok {
				svr.Log(errors.New("invalid search status"), "invalid search status")
				svr.JSON(w, http.StatusBadRequest, "invalid search status")
				return
			}
			if _, ok := developer.ValidRoleLevels[req.RoleLevel]; !ok {
				svr.Log(errors.New("invalid role level"), "invalid role level")
				svr.JSON(w, http.StatusBadRequest, "invalid role level")
				return
			}
			for _, v := range req.RoleTypes {
				if _, ok := developer.ValidRoleTypes[v]; !ok {
					svr.Log(errors.New("invalid role type"), "invalid role type")
					svr.JSON(w, http.StatusBadRequest, "invalid role type")
					return
				}
			}
			if req.HourlyRate == "" || req.HourlyRate == "0" {
				svr.JSON(w, http.StatusBadRequest, "please specify hourly rate")
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
			avail := true
			if req.SearchStatus == developer.SearchStatusNotAvailable {
				avail = false
			}
			hourlyRate, err := strconv.ParseInt(req.HourlyRate, 10, 64)
			if err != nil {
				svr.Log(err, "unable to parse string to int")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}

			dev := developer.Developer{
				ID:           req.ID,
				Name:         req.Fullname,
				Location:     req.CurrentLocation,
				HourlyRate:   hourlyRate,
				LinkedinURL:  req.LinkedinURL,
				Bio:          req.Bio,
				Email:        req.Email,
				Available:    avail,
				UpdatedAt:    t,
				Skills:       req.Skills,
				ImageID:      req.ImageID,
				SearchStatus: req.SearchStatus,
				RoleLevel:    req.RoleLevel,
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
		emailStr := strings.ToLower(r.URL.Query().Get("email"))
		if !svr.IsEmail(emailStr) {
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
		err = database.AddEmailSubscriber(svr.Conn, emailStr, k.String())
		if err != nil {
			svr.Log(err, "unable to add email subscriber to db")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		err = svr.GetEmail().SendHTMLEmail(
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			email.Address{Email: emailStr},
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			fmt.Sprintf("Confirm Your Email Subscription on %s", svr.GetConfig().SiteName),
			fmt.Sprintf(
				"Please click on the link below to confirm your subscription to receive weekly emails from %s\n\n%s\n\nIf this was not requested by you, please ignore this email.",
				svr.GetConfig().SiteName,
				fmt.Sprintf("%s%s/x/email/confirm/%s", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, k.String()),
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
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			profileID := vars["id"]
			sender, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if sender.IsDeveloper {
				svr.JSON(w, http.StatusUnauthorized, "unauthorized")
				return
			}
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
			err = devRepo.SendMessageDeveloperProfile(message, sender.UserID)
			if err != nil {
				svr.Log(err, "unable to send message to developer profile")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
			if err := devRepo.TrackDeveloperProfileMessageSent(dev); err != nil {
				svr.Log(err, "unable to track message sent to developer profile")
			}
			err = svr.GetEmail().SendHTMLEmail(
				email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
				email.Address{Email: dev.Email},
				email.Address{Email: message.Email},
				fmt.Sprintf("New Message from %s", svr.GetConfig().SiteName),
				fmt.Sprintf(
					"You received a new message from %s: \n\nMessage: %s\n\nFrom: %s",
					svr.GetConfig().SiteName,
					message.Content,
					message.Email,
				),
			)
			if err != nil {
				svr.Log(err, "unable to send email to developer profile")
				svr.JSON(w, http.StatusBadRequest, "There was a problem while sending the email")
				return
			}
			if err := devRepo.MarkDeveloperMessageAsSent(message.ID); err != nil {
				svr.Log(err, "unable to mark developer message as sent "+message.ID)
			}
			svr.JSON(w, http.StatusOK, nil)
		})
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
		message, emailStr, err := devRepo.MessageForDeliveryByID(messageID)
		if err != nil {
			svr.JSON(w, http.StatusBadRequest, "Your link may be invalid or expired")
			return
		}
		err = svr.GetEmail().SendHTMLEmail(
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			email.Address{Email: emailStr},
			email.Address{Email: message.Email},
			fmt.Sprintf("New Message from %s", svr.GetConfig().SiteName),
			fmt.Sprintf(
				"You received a new message from %s: \n\nMessage: %s\n\nFrom: %s",
				svr.GetConfig().SiteName,
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

func EditProfileHandler(svr server.Server, devRepo *developer.Repository, recRepo *recruiter.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			profileID := vars["id"]
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				http.Redirect(w, r, "/auth", http.StatusUnauthorized)
				return
			}
			// todo: allow admin to edit any profile type
			// todo: check that only owners can edit their own profiles
			switch profile.Type {
			case user.UserTypeDeveloper:
				dev, err := devRepo.DeveloperProfileByID(profileID)
				devExps, err := devRepo.DeveloperMetadataByProfileID("experience", profileID)
				devEducation, err := devRepo.DeveloperMetadataByProfileID("education", profileID)
				devProjects, err := devRepo.DeveloperMetadataByProfileID("github", profileID)
				if err != nil {
					svr.Log(err, "unable to find developer profile")
					http.Redirect(w, r, "/auth", http.StatusUnauthorized)
					return
				}
				if dev.Email != profile.Email && !profile.IsAdmin {
					http.Redirect(w, r, "/auth", http.StatusUnauthorized)
					return
				}
				svr.Render(r, w, http.StatusOK, "edit-developer-profile.html", map[string]interface{}{
					"DeveloperProfile":        dev,
					"DeveloperExperiences":    devExps,
					"DeveloperEducation":      devEducation,
					"DeveloperGithubProjects": devProjects,
				})
			case user.UserTypeRecruiter:
				rec, err := recRepo.RecruiterProfileByID(profileID)
				if err != nil {
					svr.Log(err, "unable to find recruiter profile")
					http.Redirect(w, r, "/auth", http.StatusUnauthorized)
					return
				}
				svr.Render(r, w, http.StatusOK, "edit-recruiter-profile.html", map[string]interface{}{
					"RecruiterProfile": rec,
				})
			case user.UserTypeAdmin:
				svr.Log(err, "admin does not have profile to edit yet")
				http.Redirect(w, r, "/auth", http.StatusUnauthorized)
				return
			}
		},
	)
}

func ViewDeveloperProfileHandler(svr server.Server, devRepo *developer.Repository, recruiterRepo *recruiter.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		profileSlug := vars["slug"]
		profile, _ := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
		if profile != nil && profile.Type == "recruiter" {
			expTime, err := recruiterRepo.RecruiterProfilePlanExpiration(profile.Email)
			if err == nil && expTime.Before(time.Now().UTC()) {
				svr.Redirect(w, r, http.StatusTemporaryRedirect, fmt.Sprintf("%s%s/profile/home#developer-subscription", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost))
				return
			}

		}
		dev, err := devRepo.DeveloperProfileBySlug(profileSlug)
		if err != nil {
			svr.Log(err, "unable to find developer profile by slug "+profileSlug)
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		if err := devRepo.TrackDeveloperProfileView(dev); err != nil {
			svr.Log(err, "unable to track developer profile view")
		}
		devExps, err := devRepo.DeveloperMetadataByProfileID("experience", dev.ID)
		devEducation, err := devRepo.DeveloperMetadataByProfileID("education", dev.ID)
		devProjects, err := devRepo.DeveloperMetadataByProfileID("github", dev.ID)
		if err != nil {
			svr.Log(err, "unable to find developer metadata")
			http.Redirect(w, r, "/auth", http.StatusUnauthorized)
			return
		}
		dev.UpdatedAtHumanized = dev.UpdatedAt.UTC().Format("January 2006")
		dev.SkillsArray = strings.Split(dev.Skills, ",")
		svr.Render(r, w, http.StatusOK, "view-developer-profile.html", map[string]interface{}{
			"DeveloperProfile":        dev,
			"DeveloperExperiences":    devExps,
			"DeveloperEducation":      devEducation,
			"DeveloperGithubProjects": devProjects,
			"IsAdmin":                 profile != nil && profile.Type == "admin",
			"MonthAndYear":            time.Now().UTC().Format("January 2006"),
		})
	}
}

func CompaniesForLocationHandler(svr server.Server, companyRepo *company.Repository, jobRepo *job.Repository, devRepo *developer.Repository, loc string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("p")
		svr.RenderPageForCompanies(w, r, companyRepo, jobRepo, devRepo, loc, page, "companies.html")
	}
}

func IndexPageHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository, bookmarkRepo *bookmark.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		location := r.URL.Query().Get("l")
		tag := r.URL.Query().Get("t")
		page := r.URL.Query().Get("p")

		var dst string
		if location != "" && tag != "" {
			dst = fmt.Sprintf("/%s-%s-Jobs-In-%s", strings.Title(svr.GetConfig().SiteJobCategory), tag, location)
		} else if location != "" {
			dst = fmt.Sprintf("/%s-Jobs-In-%s", strings.Title(svr.GetConfig().SiteJobCategory), location)
		} else if tag != "" {
			dst = fmt.Sprintf("/%s-%s-Jobs", strings.Title(svr.GetConfig().SiteJobCategory), tag)
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
			dst = fmt.Sprintf("/%s-%s-Jobs-In-%s", strings.Title(svr.GetConfig().SiteJobCategory), tag, location)
		} else if location != "" {
			dst = fmt.Sprintf("/%s-Jobs-In-%s", strings.Title(svr.GetConfig().SiteJobCategory), location)
		} else if tag != "" {
			dst = fmt.Sprintf("/%s-%s-Jobs", strings.Title(svr.GetConfig().SiteJobCategory), tag)
		}
		if page != "" {
			dst += fmt.Sprintf("?p=%s", page)
		}
		if (salary != "" && !validSalary) || (currency != "" && currency != "USD") {
			svr.Redirect(w, r, http.StatusMovedPermanently, dst)
			return
		}

		svr.RenderPageForLocationAndTag(w, r, jobRepo, devRepo, bookmarkRepo, "", "", page, salary, currency, "landing.html")
	}
}

func PermanentRedirectHandler(svr server.Server, dst string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("https://%s/%s", svr.GetConfig().SiteHost, dst))
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
		svr.Render(r, w, http.StatusOK, "payment.html", map[string]interface{}{
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
			svr.Render(r, w, http.StatusOK, "post-a-job-without-payment.html", nil)
		},
	)
}

func RequestTokenSignOn(svr server.Server, userRepo *user.Repository, jobRepo *job.Repository, recRepo *recruiter.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		numberOfAttempts := 0
		cachedAttempts, found := svr.CacheGet(fmt.Sprintf("sign-on-request-%s", req.Email))
		if found {
			attempts, err := strconv.Atoi(string(cachedAttempts))
			if err == nil {
				numberOfAttempts = attempts
			}
		}

		if numberOfAttempts >= 5 {
			svr.JSON(w, http.StatusTooManyRequests, nil)
			return
		}

		userType, err := userRepo.GetUserTypeByEmailOrCreateUserIfRecruiter(req.Email, jobRepo, recRepo)
		if err != nil {
			svr.JSON(w, http.StatusNotFound, nil)
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err = userRepo.SaveTokenSignOn(req.Email, k.String(), userType)
		if err != nil {
			svr.Log(err, "unable to save sign on token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}

		numberOfAttempts = numberOfAttempts + 1
		svr.CacheSet(fmt.Sprintf("sign-on-request-%s", req.Email), []byte(strconv.Itoa(numberOfAttempts)))

		token := k.String()
		err = svr.GetEmail().SendHTMLEmail(
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			email.Address{Email: req.Email},
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			fmt.Sprintf("Sign On on %s", svr.GetConfig().SiteName),
			fmt.Sprintf("Sign On on %s %s%s/x/auth/%s", svr.GetConfig().SiteName, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, token))
		if err != nil {
			svr.Log(err, "unable to send email while applying to job")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func VerifyTokenSignOn(svr server.Server, userRepo *user.Repository, devRepo *developer.Repository, recRepo *recruiter.Repository, adminEmail string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		token := vars["token"]
		u, _, err := userRepo.GetOrCreateUserFromToken(token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to validate signon token %s", token))
			svr.TEXT(w, http.StatusBadRequest, "Invalid or expired token")
			return
		}
		fmt.Println("verify")
		sess, err := svr.SessionStore.Get(r, "____gc")
		if err != nil {
			svr.TEXT(w, http.StatusInternalServerError, "Invalid or expired token")
			svr.Log(err, "unable to get session cookie from request")
			return
		}
		stdClaims := &jwt.StandardClaims{
			ExpiresAt: time.Now().Add(30 * 24 * time.Hour).UTC().Unix(),
			IssuedAt:  time.Now().UTC().Unix(),
			Issuer:    fmt.Sprintf("%s%s", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost),
		}
		claims := middleware.UserJWT{
			UserID:         u.ID,
			Email:          u.Email,
			IsAdmin:        u.Type == user.UserTypeAdmin,
			IsRecruiter:    u.Type == user.UserTypeRecruiter,
			IsDeveloper:    u.Type == user.UserTypeDeveloper,
			CreatedAt:      u.CreatedAt,
			Type:           u.Type,
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
		fmt.Println("got step user type", u.Type)
		switch u.Type {
		case user.UserTypeDeveloper:
			dev, err := devRepo.DeveloperProfileByEmail(u.Email)
			if err != nil {
				svr.Log(err, "unable to find developer profile by email")
				svr.JSON(w, http.StatusNotFound, "unable to find developer profile by email")
				return
			}
			if !dev.UpdatedAt.After(dev.CreatedAt) {
				if activateDevProfileErr := devRepo.ActivateDeveloperProfile(u.Email); activateDevProfileErr != nil {
					svr.Log(err, "unable to activate developer profile")
					svr.JSON(w, http.StatusInternalServerError, nil)
					return
				}
			}
			if err := database.ConfirmEmailSubscriber(svr.Conn, token); err != nil {
				svr.Log(err, "unable to confirm subscriber using token "+token)
			}
			svr.Redirect(w, r, http.StatusMovedPermanently, "/profile/home")
			return
		case user.UserTypeRecruiter:
			rec, err := recRepo.RecruiterProfileByEmail(u.Email)
			if err != nil {
				svr.Log(err, "unable to find recruiter profile by email")
				svr.JSON(w, http.StatusNotFound, "unable to find recruiter profile by email")
				return
			}
			if !rec.UpdatedAt.After(rec.CreatedAt) {
				if activateRecProfileErr := recRepo.ActivateRecruiterProfile(u.Email); activateRecProfileErr != nil {
					svr.Log(err, "unable to activate recruiter profile")
					svr.JSON(w, http.StatusInternalServerError, nil)
					return
				}
			}
			svr.Redirect(w, r, http.StatusMovedPermanently, "/profile/home")
			return
		case user.UserTypeAdmin:
			svr.Redirect(w, r, http.StatusMovedPermanently, "/profile/home")
			return
		}
		svr.Log(errors.New("unable to complete token verification flow"), fmt.Sprintf("email %s token %s and user type %s", u.Email, token, u.Type))
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
			svr.RenderPageForLocationAndTagAdmin(r, w, jobRepo, loc, skill, page, salary, currency, "list-jobs-admin.html")
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

func JobBySlugPageHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository, bookmarkRepo *bookmark.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		slug := vars["slug"]
		location := vars["l"]
		jobPost, err := jobRepo.JobPostBySlug(slug)
		if err != nil || jobPost == nil {
			svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job %s/job/%s not found", svr.GetConfig().SiteHost, slug))
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

		messagesSentLastMonth, err := devRepo.GetDeveloperMessagesSentLastMonth()
		if err != nil {
			svr.Log(err, "GetDeveloperMessagesSentLastMonth")
		}
		devsRegisteredLastMonth, err := devRepo.GetDevelopersRegisteredLastMonth()
		if err != nil {
			svr.Log(err, "GetDevelopersRegisteredLastMonth")
		}
		devPageViewsLastMonth, err := devRepo.GetDeveloperProfilePageViewsLastMonth()
		if err != nil {
			svr.Log(err, "GetDeveloperProfilePageViewsLastMonth")
		}
		lastDevUpdatedAt, err := devRepo.GetLastDevUpdatedAt()
		if err != nil {
			svr.Log(err, "unable to retrieve last developer joined at")
		}

		profile, _ := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
		bookmarksByJobId := make(map[int]*bookmark.Bookmark)
		if profile != nil {
			bookmarksByJobId, err = bookmarkRepo.GetBookmarksByJobId(profile.UserID)
			if err != nil {
				svr.Log(err, "GetBookmarksByJobId")
			}
		}

		svr.Render(r, w, http.StatusOK, "job.html", map[string]interface{}{
			"Job":                                jobPost,
			"JobURIEncoded":                      url.QueryEscape(jobPost.Slug),
			"IsQuickApply":                       isQuickApply,
			"HTMLJobDescription":                 svr.MarkdownToHTML(jobPost.JobDescription),
			"HTMLJobPerks":                       svr.MarkdownToHTML(jobPost.Perks),
			"HTMLJobInterviewProcess":            svr.MarkdownToHTML(jobPost.InterviewProcess),
			"LocationFilter":                     location,
			"ExternalJobId":                      jobPost.ExternalID,
			"MonthAndYear":                       time.Unix(jobPost.CreatedAt, 0).UTC().Format("January 2006"),
			"GoogleJobCreatedAt":                 time.Unix(jobPost.CreatedAt, 0).Format(time.RFC3339),
			"GoogleJobValidThrough":              time.Unix(jobPost.CreatedAt, 0).AddDate(0, 5, 0),
			"GoogleJobLocation":                  jobLocations[0],
			"GoogleJobDescription":               strconv.Quote(strings.ReplaceAll(string(svr.MarkdownToHTML(jobPost.JobDescription)), "\n", "")),
			"RelevantJobs":                       relevantJobs,
			"DeveloperMessagesSentLastMonth":     messagesSentLastMonth,
			"DevelopersRegisteredLastMonth":      devsRegisteredLastMonth,
			"DeveloperProfilePageViewsLastMonth": devPageViewsLastMonth,
			"LastDevCreatedAt":                   lastDevUpdatedAt.Format(time.RFC3339),
			"LastDevCreatedAtHumanized":          humanize.Time(lastDevUpdatedAt),
			"BookmarksByJobId":                   bookmarksByJobId,
		})
	}
}

func CompanyBySlugPageHandler(svr server.Server, companyRepo *company.Repository, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		slug := vars["slug"]
		company, err := companyRepo.CompanyBySlug(slug)
		if err != nil || company == nil {
			svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Company %s/job/%s not found", svr.GetConfig().SiteHost, slug))
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
			companyJobs[i].SalaryPeriod = j.SalaryPeriod
			companyJobs[i].InterviewProcess = string(svr.MarkdownToHTML(j.InterviewProcess))
			if svr.IsEmail(j.HowToApply) {
				companyJobs[i].IsQuickApply = true
			}
		}
		if err := svr.Render(r, w, http.StatusOK, "company.html", map[string]interface{}{
			"Company":      company,
			"MonthAndYear": time.Now().UTC().Format("January 2006"),
			"CompanyJobs":  companyJobs,
		}); err != nil {
			svr.Log(err, "unable to render template")
		}
	}
}

func LandingPageForLocationHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository, bookmarkRepo *bookmark.Repository, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		salary := vars["salary"]
		currency := vars["currency"]
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, jobRepo, devRepo, bookmarkRepo, location, "", page, salary, currency, "landing.html")
	}
}

func LandingPageForLocationAndSkillPlaceholderHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository, bookmarkRepo *bookmark.Repository, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		salary := vars["salary"]
		currency := vars["currency"]
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, jobRepo, devRepo, bookmarkRepo, location, skill, page, salary, currency, "landing.html")
	}
}

func LandingPageForLocationPlaceholderHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository, bookmarkRepo *bookmark.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		salary := vars["salary"]
		currency := vars["currency"]
		loc := strings.ReplaceAll(vars["location"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, jobRepo, devRepo, bookmarkRepo, loc, "", page, salary, currency, "landing.html")
	}
}

func LandingPageForSkillPlaceholderHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository, bookmarkRepo *bookmark.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		salary := vars["salary"]
		currency := vars["currency"]
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, jobRepo, devRepo, bookmarkRepo, "", skill, page, salary, currency, "landing.html")
	}
}

func LandingPageForSkillAndLocationPlaceholderHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository, bookmarkRepo *bookmark.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		salary := vars["salary"]
		currency := vars["currency"]
		loc := strings.ReplaceAll(vars["location"], "-", " ")
		skill := strings.ReplaceAll(vars["skill"], "-", " ")
		page := r.URL.Query().Get("p")
		svr.RenderPageForLocationAndTag(w, r, jobRepo, devRepo, bookmarkRepo, loc, skill, page, salary, currency, "landing.html")
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
			Title:       fmt.Sprintf("%s Jobs", svr.GetConfig().SiteName),
			Link:        &feeds.Link{Href: fmt.Sprintf("%s%s", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost)},
			Description: fmt.Sprintf("%s Jobs RSS Feed", svr.GetConfig().SiteName),
			Author:      &feeds.Author{Name: svr.GetConfig().SiteName, Email: svr.GetConfig().SupportEmail},
			Created:     now,
		}

		for _, j := range jobPosts {
			if j.CompanyIconID != "" {
				feed.Items = append(feed.Items, &feeds.Item{
					Title:       fmt.Sprintf("%s with %s - %s", j.JobTitle, j.Company, j.Location),
					Link:        &feeds.Link{Href: fmt.Sprintf("%s%s/job/%s", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, j.Slug)},
					Description: string(svr.MarkdownToHTML(j.JobDescription + "\n\n**Salary Range:** " + j.SalaryRange)),
					Author:      &feeds.Author{Name: svr.GetConfig().SiteName, Email: svr.GetConfig().SupportEmail},
					Enclosure:   &feeds.Enclosure{Length: "not implemented", Type: "image", Url: fmt.Sprintf("%s%s/x/s/m/%s", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, j.CompanyIconID)},
					Created:     *j.ApprovedAt,
				})
			} else {
				feed.Items = append(feed.Items, &feeds.Item{
					Title:       fmt.Sprintf("%s with %s - %s", j.JobTitle, j.Company, j.Location),
					Link:        &feeds.Link{Href: fmt.Sprintf("%s%s/job/%s", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, j.Slug)},
					Description: string(svr.MarkdownToHTML(j.JobDescription + "\n\n**Salary Range:** " + j.SalaryRange)),
					Author:      &feeds.Author{Name: svr.GetConfig().SiteName, Email: svr.GetConfig().SupportEmail},
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

func StripePaymentConfirmationWebhookHandler(svr server.Server, jobRepo *job.Repository, recruiterRepo *recruiter.Repository) http.HandlerFunc {
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
		svr.Log(nil, fmt.Sprintf("got session %v", string(body)))
		sess, err := payment.HandleCheckoutSessionComplete(body, svr.GetConfig().StripeEndpointSecret, stripeSig)
		if err != nil {
			svr.Log(err, "error while handling checkout session complete")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		if sess == nil {
			svr.JSON(w, http.StatusNotFound, nil)
			return
		}
		isJobAd, err := database.IsJobAdPaymentEvent(svr.Conn, sess.ID)
		if err != nil {
			svr.Log(err, "IsJobAdPaymentEvent: error")
			svr.JSON(w, http.StatusInternalServerError, map[string]interface{}{"erorr": err.Error()})
			return
		}
		if isJobAd {
			affectedRows, err := database.SaveSuccessfulPaymentForJobAd(svr.Conn, sess.ID)
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
			purchaseEvent, err := database.GetJobAdPurchaseEventBySessionID(svr.Conn, sess.ID)
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

			expiration, err := jobRepo.PlanTypeAndDurationToExpirations(
				purchaseEvent.PlanType,
				purchaseEvent.PlanDuration,
			)
			if err != nil {
				svr.Log(errors.New("unable to get expiration for plan type and duration"), fmt.Sprintf("unable to get expiration for plan type %s and duration %d for session id %s", purchaseEvent.PlanType, purchaseEvent.PlanDuration, sess.ID))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			if err := jobRepo.UpdateJobPlan(jobPost.ID, purchaseEvent.PlanType, purchaseEvent.PlanDuration, expiration); err != nil {
				svr.Log(errors.New("unable to update job to new ad type"), fmt.Sprintf("unable to update job id %d to new ad type %s and duration %d for session id %s", jobPost.ID, purchaseEvent.PlanType, purchaseEvent.PlanDuration, sess.ID))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			err = svr.GetEmail().SendHTMLEmail(
				email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().SupportSenderAddress()},
				email.Address{Email: purchaseEvent.Email},
				email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().SupportSenderAddress()},
				fmt.Sprintf("Your Job Ad is live on %s", svr.GetConfig().SiteName),
				fmt.Sprintf("Your Job Ad has been approved and it's now live. You can edit the Job Ad at any time and check page views and clickouts by following this link %s%s/edit/%s. You can also create an account by following this link: %s%s/auth?email=%s",
					svr.GetConfig().URLProtocol,
					svr.GetConfig().SiteHost,
					jobToken,
					svr.GetConfig().URLProtocol,
					svr.GetConfig().SiteHost,
					purchaseEvent.Email,
				),
			)
			if err != nil {
				svr.Log(err, "unable to send email while upgrading job ad")
			}
			if err := svr.CacheDelete(server.CacheKeyPinnedJobs); err != nil {
				svr.Log(err, "unable to cleanup cache after approving job")
			}
			svr.JSON(w, http.StatusOK, nil)
			return
		}
		isDevDirectory, err := database.IsDevDirectoryPaymentEvent(svr.Conn, sess.ID)
		if err != nil {
			svr.Log(err, "IsDevDirectoryPaymentEvent: error")
			svr.JSON(w, http.StatusInternalServerError, map[string]interface{}{"erorr": err.Error()})
			return
		}
		if isDevDirectory {
			affectedRows, err := database.SaveSuccessfulPaymentForDevDirectory(svr.Conn, sess.ID)
			if err != nil {
				svr.Log(err, "error while saving successful payment for dev directory")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			if affectedRows != 1 {
				svr.Log(errors.New("invalid number of rows affected when saving payment"), fmt.Sprintf("got %d expected 1", affectedRows))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			purchaseEvent, err := database.GetDevDirectoryPurchaseEventBySessionID(svr.Conn, sess.ID)
			if err != nil {
				svr.Log(errors.New("unable to find purchase event by stripe session id"), fmt.Sprintf("session id %s", sess.ID))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			if err := recruiterRepo.UpdateRecruiterPlanExpiration(purchaseEvent.Email, purchaseEvent.ExpiredAt); err != nil {
				svr.Log(errors.New("unable to update job to new ad type"), fmt.Sprintf("unable to update recruiter developer directory access for session id %s", sess.ID))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			err = svr.GetEmail().SendHTMLEmail(
				email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().SupportSenderAddress()},
				email.Address{Email: purchaseEvent.Email},
				email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().SupportSenderAddress()},
				fmt.Sprintf("Your Developer Directory Access is active on %s", svr.GetConfig().SiteName),
				fmt.Sprintf("Your payment has been received successfully and you can now access the Developer Directory. Please follow this link to login %s%s/auth?email=%s", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, purchaseEvent.Email))
			if err != nil {
				svr.Log(err, "unable to send email while activating recruiter developer directory plan")
			}
			svr.JSON(w, http.StatusOK, nil)
			return
		}
		svr.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "sessionID is not dev or job ad type"})
		return
	}
}

func SitemapIndexHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		index := sitemap.NewSitemapIndex()
		entries, err := database.GetSitemapIndex(svr.Conn, svr.GetConfig().SiteHost)
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

func RobotsTXTHandler(svr server.Server, robotsTxtContent []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.TEXT(w, http.StatusOK, strings.ReplaceAll(string(robotsTxtContent), "__host_placeholder__", svr.GetConfig().SiteHost))
	}
}

func AdsTXTHandler(svr server.Server, adsTxtContent []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.TEXT(w, http.StatusOK, string(adsTxtContent))
	}
}

func WellKnownSecurityHandler(svr server.Server, securityTxtContent []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		contentWithHost := strings.ReplaceAll(string(securityTxtContent), "__host_placeholder__", svr.GetConfig().SiteHost)
		svr.TEXT(w, http.StatusOK, strings.ReplaceAll(contentWithHost, "__support_email_placeholder__", svr.GetConfig().SupportEmail))
	}
}

func AboutPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(r, w, http.StatusOK, "about.html", nil)
	}
}

func PrivacyPolicyPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(r, w, http.StatusOK, "privacy-policy.html", nil)
	}
}

func TermsOfServicePageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(r, w, http.StatusOK, "terms-of-service.html", nil)
	}
}

func SalaryLandingPageLocationPlaceholderHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		location := strings.ReplaceAll(vars["location"], "-", " ")
		svr.RenderSalaryForLocation(w, r, jobRepo, devRepo, location)
	}
}

func SalaryLandingPageLocationHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderSalaryForLocation(w, r, jobRepo, devRepo, location)
	}
}

func ViewNewsletterPageHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository, bookmarkRepo *bookmark.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, r, jobRepo, devRepo, bookmarkRepo, "", "", "", "", "", "newsletter.html")
	}
}

func ViewCommunityNewsletterPageHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository, bookmarkRepo *bookmark.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, r, jobRepo, devRepo, bookmarkRepo, "", "", "", "", "", "news.html")
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

func ViewSupportPageHandler(svr server.Server, jobRepo *job.Repository, devRepo *developer.Repository, bookmarkRepo *bookmark.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, r, jobRepo, devRepo, bookmarkRepo, "", "", "", "", "", "support.html")
	}
}

var allowedMediaTypes = []string{"image/png", "image/jpeg", "image/jpg"}

func PostAJobSuccessPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(r, w, http.StatusOK, "post-a-job-success.html", nil)
	}
}

func PostAJobFailurePageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(r, w, http.StatusOK, "post-a-job-error.html", nil)
	}
}

func ApplyForJobPageHandler(svr server.Server, jobRepo *job.Repository, bookmarkRepo *bookmark.Repository) http.HandlerFunc {
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
		profile, _ := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
		// user is not logged in
		// standard flow to confirm application
		if profile == nil {
			err = jobRepo.ApplyToJob(jobPost.ID, fileBytes, emailAddr, randomTokenStr)
			if err != nil {
				svr.Log(err, "unable to apply for job while saving to db")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			err = svr.GetEmail().SendHTMLEmail(
				email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
				email.Address{Email: emailAddr},
				email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
				fmt.Sprintf("Confirm your job application with %s", jobPost.Company),
				fmt.Sprintf(
					"Thanks for applying for the position %s with %s - %s.<br>Please confirm your application now by following this link %s%s/apply/%s",
					jobPost.JobTitle,
					jobPost.Company,
					jobPost.Location,
					svr.GetConfig().URLProtocol,
					svr.GetConfig().SiteHost,
					randomTokenStr,
				),
			)
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
				err = svr.GetEmail().SendHTMLEmail(
					email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
					email.Address{Email: emailAddr},
					email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
					fmt.Sprintf("Confirm Your Email Subscription on %s", svr.GetConfig().SiteName),
					fmt.Sprintf(
						"Please click on the link below to confirm your subscription to receive weekly emails from %s\n\n%s\n\nIf this was not requested by you, please ignore this email.",
						svr.GetConfig().SiteName,
						fmt.Sprintf("%s%s/x/email/confirm/%s", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, k.String()),
					),
				)
				if err != nil {
					svr.Log(err, "unable to send email while submitting message")
					svr.JSON(w, http.StatusBadRequest, nil)
					return
				}
			}
			svr.JSON(w, http.StatusOK, nil)
			return
		}
		if profile.Email != emailAddr {
			svr.JSON(w, http.StatusBadRequest, "Please use the same email address you have registered on your profile.")
			return
		}
		err = jobRepo.ApplyToJob(jobPost.ID, fileBytes, emailAddr, randomTokenStr)
		if err != nil {
			svr.Log(err, "unable to apply for job while saving to db")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}

		// Bookmark applied jobs for the user
		err = bookmarkRepo.BookmarkJob(profile.UserID, jobPost.ID, true)
		if err != nil {
			svr.Log(err, "error bookmarking job during application")
		}

		retrievedJobPost, applicant, err := jobRepo.GetJobByApplyToken(randomTokenStr)
		if err != nil {
			svr.Render(r, w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Invalid Job Application",
				"Description": "Oops, seems like the application you are trying to complete is no longer valid. Your application request may be expired or simply the company may not be longer accepting applications.",
			})
			return
		}
		err = svr.GetEmail().SendHTMLEmail(
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			email.Address{Email: retrievedJobPost.HowToApply},
			email.Address{Email: applicant.Email},
			fmt.Sprintf("New Applicant from %s", svr.GetConfig().SiteName),
			fmt.Sprintf(
				"Hi, there is a new applicant for your position on %s: %s with %s - %s (%s%s/job/%s). Applicant's Email: %s. Please find applicant's CV on your job dashboard",
				svr.GetConfig().SiteName,
				retrievedJobPost.JobTitle,
				retrievedJobPost.Company,
				retrievedJobPost.Location,
				svr.GetConfig().SiteHost,
				svr.GetConfig().URLProtocol,
				retrievedJobPost.Slug,
				applicant.Email,
			),
		)
		if err != nil {
			svr.Log(err, "unable to send email while applying to job")
			svr.Render(r, w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Job Application Failure",
				"Description": fmt.Sprintf("Oops, there was a problem while completing yuor application. Please try again later. If the problem persists, please contact %s", svr.GetConfig().SupportEmail),
			})
			return
		}
		err = jobRepo.ConfirmApplyToJob(randomTokenStr)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to update apply_token with successfull application for token %s", randomTokenStr))
			svr.Render(r, w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Job Application Failure",
				"Description": fmt.Sprintf("Oops, there was a problem while completing yuor application. Please try again later. If the problem persists, please contact %s", svr.GetConfig().SupportEmail),
			})
			return
		}
		svr.Render(r, w, http.StatusOK, "apply-message.html", map[string]interface{}{
			"Title": "Job Application Successfull",
			"Description": svr.StringToHTML(
				fmt.Sprintf(
					"Thank you for applying for <b>%s with %s - %s</b><br><a href=\"%s%s/job/%s\">%s%s/job/%s</a>. <br><br>Your CV has been forwarded to company HR. <br>Consider joining our Golang Cafe Developer community where companies can apply to you",
					retrievedJobPost.JobTitle,
					retrievedJobPost.Company,
					retrievedJobPost.Location,
					svr.GetConfig().URLProtocol,
					svr.GetConfig().SiteHost,
					retrievedJobPost.Slug,
					svr.GetConfig().URLProtocol,
					svr.GetConfig().SiteHost,
					retrievedJobPost.Slug,
				),
			),
		})
	}
}

func ApplyToJobConfirmation(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		token := vars["token"]
		jobPost, applicant, err := jobRepo.GetJobByApplyToken(token)
		if err != nil {
			svr.Render(r, w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Invalid Job Application",
				"Description": "Oops, seems like the application you are trying to complete is no longer valid. Your application request may be expired or simply the company may not be longer accepting applications.",
			})
			return
		}
		err = svr.GetEmail().SendHTMLEmail(
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			email.Address{Email: jobPost.HowToApply},
			email.Address{Email: applicant.Email},
			fmt.Sprintf("New Applicant from %s", svr.GetConfig().SiteName),
			fmt.Sprintf(
				"Hi, there is a new applicant for your position on %s: %s with %s - %s (%s%s/job/%s). Applicant's Email: %s. Please find applicant's CV on your job dashboard",
				svr.GetConfig().SiteName,
				jobPost.JobTitle,
				jobPost.Company,
				jobPost.Location,
				svr.GetConfig().URLProtocol,
				svr.GetConfig().SiteHost,
				jobPost.Slug,
				applicant.Email,
			),
		)
		if err != nil {
			svr.Log(err, "unable to send email while applying to job")
			svr.Render(r, w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Job Application Failure",
				"Description": fmt.Sprintf("Oops, there was a problem while completing yuor application. Please try again later. If the problem persists, please contact %s", svr.GetConfig().SupportEmail),
			})
			return
		}
		err = jobRepo.ConfirmApplyToJob(token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to update apply_token with successfull application for token %s", token))
			svr.Render(r, w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Job Application Failure",
				"Description": fmt.Sprintf("Oops, there was a problem while completing yuor application. Please try again later. If the problem persists, please contact %s", svr.GetConfig().SupportEmail),
			})
			return
		}
		svr.Render(r, w, http.StatusOK, "apply-message.html", map[string]interface{}{
			"Title": "Job Application Successfull",
			"Description": svr.StringToHTML(
				fmt.Sprintf(
					"Thank you for applying for <b>%s with %s - %s</b><br><a href=\"%s%s/job/%s\">%s%s/job/%s</a>. <br><br>Your CV has been forwarded to company HR. <br>Consider joining our Golang Cafe Developer community where companies can apply to you",
					jobPost.JobTitle,
					jobPost.Company,
					jobPost.Location,
					svr.GetConfig().URLProtocol,
					svr.GetConfig().SiteHost,
					jobPost.Slug,
					svr.GetConfig().URLProtocol,
					svr.GetConfig().SiteHost,
					jobPost.Slug,
				),
			),
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
			jobRq.PlanType = job.JobPlanTypeBasic
			jobRq.PlanDuration = 1
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
		},
	)
}

func DeveloperDirectoryUpsellPageHandler(svr server.Server, jobRepo *job.Repository, paymentRepo *payment.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			upsellRq := &struct {
				RecruiterID  string `json:"recruiter_id"`
				PlanDuration int64  `json:"plan_duration"`
				ItemPrice    int64  `json:"item_price"`
			}{}
			if err := decoder.Decode(&upsellRq); err != nil {
				svr.Log(err, "unable to decode request")
				svr.JSON(w, http.StatusBadRequest, err.Error())
				return
			}
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to retrieve user from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			sess, err := paymentRepo.CreateDevDirectorySession(profile.Email, upsellRq.RecruiterID, int64(upsellRq.ItemPrice*100), int64(upsellRq.PlanDuration), true)
			if err != nil {
				svr.Log(err, "unable to create payment session")
			}
			err = svr.GetEmail().SendHTMLEmail(
				email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
				email.Address{Email: svr.GetEmail().DefaultAdminAddress()},
				email.Address{Email: profile.Email},
				fmt.Sprintf("New Dev Directory Subscriber Renew on %s", svr.GetConfig().SiteName),
				fmt.Sprintf(
					"Hey! There is a new Developer Directory Subscription Renew on %s. Developer Directory Subscription %d Months Plan @ US$%d/month, Email: %s",
					svr.GetConfig().SiteName,
					upsellRq.PlanDuration,
					upsellRq.ItemPrice,
					profile.Email,
				),
			)
			if err != nil {
				svr.Log(err, "unable to send email to admin while creating subscription")
			}
			if sess != nil {
				err = database.InitiatePaymentEventForDeveloperDirectoryAccess(
					svr.Conn,
					sess.ID,
					int64(upsellRq.ItemPrice*upsellRq.PlanDuration*100),
					fmt.Sprintf("Developer Directory Subscription %d Months Plan @ US$%d/month", upsellRq.PlanDuration, upsellRq.ItemPrice),
					upsellRq.RecruiterID,
					profile.Email,
					int64(upsellRq.PlanDuration),
				)
				if err != nil {
					svr.Log(err, "unable to save payment initiated event")
				}
				svr.JSON(w, http.StatusOK, map[string]string{"s_id": sess.ID})
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func SubmitJobPostPaymentUpsellPageHandler(svr server.Server, jobRepo *job.Repository, paymentRepo *payment.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		jobRq := &job.JobRqUpsell{}
		if err := decoder.Decode(&jobRq); err != nil {
			svr.Log(err, "unable to decode request")
			svr.JSON(w, http.StatusBadRequest, err.Error())
			return
		}
		planDuration, err := strconv.Atoi(jobRq.PlanDurationStr)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to convert duration to int %s", jobRq.PlanDurationStr))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		jobRq.PlanDuration = planDuration
		jobID, err := jobRepo.JobPostIDByToken(jobRq.Token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to find job by token %s", jobRq.Token))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		monthlyAmount := 59
		switch jobRq.PlanType {
		case job.JobPlanTypeBasic:
			monthlyAmount = svr.GetConfig().PlanID1Price
		case job.JobPlanTypePro:
			monthlyAmount = svr.GetConfig().PlanID2Price
		case job.JobPlanTypePlatinum:
			monthlyAmount = svr.GetConfig().PlanID3Price
		}
		sess, err := paymentRepo.CreateJobAdSession(
			&job.JobRq{
				PlanType:     jobRq.PlanType,
				PlanDuration: jobRq.PlanDuration,
				CurrencyCode: "USD",
				Email:        jobRq.Email,
			},
			jobRq.Token,
			int64(monthlyAmount),
			int64(jobRq.PlanDuration),
		)
		if err != nil {
			svr.Log(err, "unable to create payment session")
		}

		err = svr.GetEmail().SendHTMLEmail(
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			email.Address{Email: svr.GetEmail().DefaultAdminAddress()},
			email.Address{Email: jobRq.Email},
			fmt.Sprintf("New Upgrade on %s", svr.GetConfig().SiteName),
			fmt.Sprintf(
				"Hey! There is a new ad upgrade on %s. Please check %s%s/manage/%s",
				svr.GetConfig().SiteName,
				svr.GetConfig().URLProtocol,
				svr.GetConfig().SiteHost,
				jobRq.Token,
			),
		)
		if err != nil {
			svr.Log(err, "unable to send email to admin while upgrading job ad")
		}
		if sess == nil {
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		err = database.InitiatePaymentEventForJobAd(
			svr.Conn,
			sess.ID,
			payment.PlanTypeAndDurationToAmount(
				jobRq.PlanType,
				int64(jobRq.PlanDuration),
				int64(svr.GetConfig().PlanID1Price),
				int64(svr.GetConfig().PlanID1Price),
				int64(svr.GetConfig().PlanID1Price),
			),
			payment.PlanTypeAndDurationToDescription(
				jobRq.PlanType,
				int64(jobRq.PlanDuration),
			),
			jobRq.Email,
			jobID,
			jobRq.PlanType,
			int64(jobRq.PlanDuration),
		)
		if err != nil {
			svr.Log(err, "unable to save payment initiated event")
		}
		svr.JSON(w, http.StatusOK, map[string]string{"s_id": sess.ID})
	}
}

func GeneratePaymentIntent(svr server.Server, paymentRepo *payment.Repository) http.HandlerFunc {
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
		sess, err := paymentRepo.CreateGenericSession(req.Email, req.Currency, req.Amount)
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

func SubmitJobPostPageHandler(svr server.Server, jobRepo *job.Repository, paymentRepo *payment.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		jobRq := &job.JobRq{}
		if err := decoder.Decode(&jobRq); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		planDurationInt, err := strconv.Atoi(jobRq.PlanDurationStr)
		if err != nil {
			svr.Log(fmt.Errorf("invalid plan duration: unable to save job request: %#v", jobRq), "unable to save job request")
			svr.JSON(w, http.StatusBadRequest, "invalid plan duration")
			return
		}
		jobRq.PlanDuration = planDurationInt
		jobRq.CurrencyCode = "USD"
		if jobRq.PlanType != job.JobPlanTypeBasic && jobRq.PlanType != job.JobPlanTypePro && jobRq.PlanType != job.JobPlanTypePlatinum {
			svr.Log(fmt.Errorf("invalid plan type: unable to save job request: %#v", jobRq), "unable to save job request")
			svr.JSON(w, http.StatusBadRequest, "invalid plan type")
			return
		}
		if jobRq.PlanDuration > 6 || jobRq.PlanDuration < 1 {
			svr.Log(fmt.Errorf("invalid plan duration: unable to save job request: %#v", jobRq), "unable to save job request")
			svr.JSON(w, http.StatusBadRequest, "invalid plan duration")
			return
		}
		jobID, err := jobRepo.SaveDraft(jobRq)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to save job request: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, err.Error())
			return
		}
		if jobID == 0 {
			svr.Log(err, fmt.Sprintf("unable to save job request: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, "unable to save job request invalid job returned")
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate unique token")
			svr.JSON(w, http.StatusBadRequest, err.Error())
			return
		}
		randomToken, err := k.Value()
		if err != nil {
			svr.Log(err, "unable to get token value")
			svr.JSON(w, http.StatusBadRequest, err.Error())
			return
		}
		randomTokenStr, ok := randomToken.(string)
		if !ok {
			svr.Log(err, "unbale to assert token value as string")
			svr.JSON(w, http.StatusBadRequest, "unbale to assert token value as string")
			return
		}
		err = jobRepo.SaveTokenForJob(randomTokenStr, jobID)
		if err != nil {
			svr.Log(err, "unbale to generate token")
			svr.JSON(w, http.StatusBadRequest, err.Error())
			return
		}
		monthlyAmount := 59
		switch jobRq.PlanType {
		case job.JobPlanTypeBasic:
			monthlyAmount = svr.GetConfig().PlanID1Price
		case job.JobPlanTypePro:
			monthlyAmount = svr.GetConfig().PlanID2Price
		case job.JobPlanTypePlatinum:
			monthlyAmount = svr.GetConfig().PlanID3Price
		}
		sess, err := paymentRepo.CreateJobAdSession(jobRq, randomTokenStr, int64(monthlyAmount), int64(jobRq.PlanDuration))
		if err != nil {
			svr.Log(err, "unable to create payment session")
		}
		err = svr.GetEmail().SendHTMLEmail(
			email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().NoReplySenderAddress()},
			email.Address{Email: svr.GetEmail().DefaultAdminAddress()},
			email.Address{Email: jobRq.Email},
			fmt.Sprintf("New Job Ad on %s", svr.GetConfig().SiteName),
			fmt.Sprintf(
				"Hey! There is a new Ad on %s. Please approve %s%s/manage/%s",
				svr.GetConfig().SiteName,
				svr.GetConfig().URLProtocol,
				svr.GetConfig().SiteHost,
				randomTokenStr,
			),
		)
		if err != nil {
			svr.Log(err, "unable to send email to admin while posting job ad")
		}
		if sess != nil {
			err = database.InitiatePaymentEventForJobAd(
				svr.Conn,
				sess.ID,
				payment.PlanTypeAndDurationToAmount(
					jobRq.PlanType,
					int64(jobRq.PlanDuration),
					int64(svr.GetConfig().PlanID1Price),
					int64(svr.GetConfig().PlanID2Price),
					int64(svr.GetConfig().PlanID3Price),
				),
				payment.PlanTypeAndDurationToDescription(
					jobRq.PlanType,
					int64(jobRq.PlanDuration),
				),
				jobRq.Email,
				jobID,
				jobRq.PlanType,
				int64(jobRq.PlanDuration),
			)
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
		}).SubImage(image.Rect(x, y, x+wi, y+he))
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
			err = svr.GetEmail().SendHTMLEmail(
				email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().SupportSenderAddress()},
				email.Address{Email: jobRq.Email},
				email.Address{Name: svr.GetEmail().DefaultSenderName(), Email: svr.GetEmail().SupportSenderAddress()},
				fmt.Sprintf("Your Job Ad on %s", svr.GetConfig().SiteName),
				fmt.Sprintf("Thanks for using %s,\n\nYour Job Ad has been approved and it's currently live on %s: %s%s.\n\nYou can track your Ad performance and renew your Ad via this edit link: %s%s/edit/%s\n.", svr.GetConfig().SiteName, svr.GetConfig().SiteName, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost, jobRq.Token),
			)
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

func EditJobViewPageHandler(svr server.Server, jobRepo *job.Repository, recRepo *recruiter.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		token := vars["token"]
		isCallback := r.URL.Query().Get("callback")
		paymentSuccess := r.URL.Query().Get("payment")
		jobID, err := jobRepo.JobPostIDByToken(token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", token))
			svr.JSON(w, http.StatusNotFound, nil)
			return
		}
		jobPost, err := jobRepo.JobPostByIDForEdit(jobID)
		if err != nil || jobPost == nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job by ID %d", jobID))
			svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job for %s/edit/%s not found", svr.GetConfig().SiteHost, token))
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
		applicants, err := jobRepo.GetApplicantsForJob(jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job applicants for job id %d", jobPost.ID))
		}

		profile, _ := recRepo.RecruiterProfileByEmail(jobPost.CompanyEmail)
		isSignedOn := middleware.IsSignedOn(r, svr.SessionStore, svr.GetJWTSigningKey())

		svr.Render(r, w, http.StatusOK, "edit.html", map[string]interface{}{
			"Job":                        jobPost,
			"HowToApplyIsURL":            !svr.IsEmail(jobPost.HowToApply),
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
			"DefaultPlanExpiration":      time.Now().UTC().AddDate(0, 0, 30),
			"StripePublishableKey":       svr.GetConfig().StripePublishableKey,
			"Applicants":                 applicants,
			"HasProfile":                 profile.ID != "",
			"IsSignedOn":                 isSignedOn,
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
				svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job for %s/manage/job/%s not found", svr.GetConfig().SiteHost, slug))
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
				svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job for %s/edit/%s not found", svr.GetConfig().SiteHost, token))
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
			applicants, err := jobRepo.GetApplicantsForJob(jobID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve job applicants for job id %d", jobID))
			}
			svr.Render(r, w, http.StatusOK, "manage.html", map[string]interface{}{
				"Job":                        jobPost,
				"JobPerksEscaped":            svr.JSEscapeString(jobPost.Perks),
				"JobInterviewProcessEscaped": svr.JSEscapeString(jobPost.InterviewProcess),
				"JobDescriptionEscaped":      svr.JSEscapeString(jobPost.JobDescription),
				"Token":                      token,
				"ViewCount":                  viewCount,
				"ClickoutCount":              clickoutCount,
				"ConversionRate":             conversionRate,
				"Applicants":                 applicants,
			})
		},
	)
}

func DownloadJobApplicationCvHandler(svr server.Server, jobRepo *job.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		token := vars["token"]
		applicant, err := jobRepo.GetApplicantByApplyToken(token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to find job application by applicant token: %s", token))
			svr.JSON(w, http.StatusNotFound, nil)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", applicant.CvSize))

		_, err = w.Write(applicant.Cv)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to serve job application CV for applicant token: %s", token))
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
	}
}

func GetBlogPostBySlugHandler(svr server.Server, blogPostRepo *blog.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		slug := vars["slug"]
		bp, err := blogPostRepo.GetBySlug(slug)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve blog post: Slug=%s", slug))
			svr.TEXT(w, http.StatusNotFound, "Could not retrieve blogpost. Please try again later.")
			return
		}
		svr.Render(r, w, http.StatusOK, "view-blogpost.html", map[string]interface{}{
			"BlogPost":         bp,
			"BlogPostTextHTML": svr.MarkdownToHTML(bp.Text),
		})
	}
}

func EditBlogPostHandler(svr server.Server, blogPostRepo *blog.Repository) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			id := vars["id"]
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to retrieve user from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			bp, err := blogPostRepo.GetByIDAndAuthor(id, profile.UserID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve blog post: ID=%s authorID=%s", id, profile.UserID))
				svr.TEXT(w, http.StatusNotFound, "Could not retrieve blogpost. Please try again later.")
				return
			}
			svr.Render(r, w, http.StatusOK, "edit-blogpost.html", map[string]interface{}{
				"BlogPost":    bp,
				"IsPublished": bp.PublishedAt != nil,
			})
		},
	)
}

func CreateBlogPostHandler(svr server.Server, blogPostRepo *blog.Repository) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			blogRq := &blog.CreateRq{}
			if err := decoder.Decode(&blogRq); err != nil {
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			k, err := ksuid.NewRandom()
			if err != nil {
				svr.Log(err, "unable to generate unique blog post id")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			blogPostID, err := k.Value()
			if err != nil {
				svr.Log(err, "unable to get blog post id value")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			blogPostIDStr, ok := blogPostID.(string)
			if !ok {
				svr.Log(err, "unbale to assert blog post id value as string")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to retrieve user from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			bp := blog.BlogPost{
				ID:          blogPostIDStr,
				Title:       blogRq.Title,
				Slug:        slug.Make(blogRq.Title),
				Description: blogRq.Description,
				Tags:        blogRq.Tags,
				Text:        blogRq.Text,
				CreatedBy:   profile.UserID,
			}
			if err := blogPostRepo.Create(bp); err != nil {
				svr.Log(err, fmt.Sprintf("unable to create blog post: ID=%s authorID=%s", blogPostIDStr, profile.UserID))
				svr.JSON(w, http.StatusInternalServerError, map[string]interface{}{"error": "could not create blog post. Please try again later." + err.Error()})
				return
			}
			svr.JSON(w, http.StatusOK, map[string]interface{}{"id": bp.ID})
		},
	)
}

func CreateDraftBlogPostHandler(svr server.Server, blogPostRepo *blog.Repository) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			svr.Render(r, w, http.StatusOK, "create-blogpost.html", map[string]interface{}{})
		},
	)
}

func UpdateBlogPostHandler(svr server.Server, blogPostRepo *blog.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			bpRq := &blog.UpdateRq{}
			if err := decoder.Decode(&bpRq); err != nil {
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to retrieve user from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			bp := blog.BlogPost{
				ID:          bpRq.ID,
				Title:       bpRq.Title,
				Description: bpRq.Description,
				Tags:        bpRq.Tags,
				Text:        bpRq.Text,
				CreatedBy:   profile.UserID,
			}
			if err := blogPostRepo.Update(bp); err != nil {
				svr.Log(err, fmt.Sprintf("unable to update blog post: ID=%s authorID=%s", bp.ID, profile.UserID))
				svr.JSON(w, http.StatusNotFound, map[string]interface{}{"error": "could not update blog post. Please try again later"})
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func PublishBlogPostHandler(svr server.Server, blogPostRepo *blog.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			id := vars["id"]
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			bp := blog.BlogPost{
				ID:        id,
				CreatedBy: profile.UserID,
			}
			if err := blogPostRepo.Publish(bp); err != nil {
				svr.Log(err, fmt.Sprintf("unable to unpublish blog post: ID=%s authorID=%s", id, profile.UserID))
				svr.JSON(w, http.StatusInternalServerError, "Could not unpublish blogpost. Please try again later.")
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func UnpublishBlogPostHandler(svr server.Server, blogPostRepo *blog.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			id := vars["id"]
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			bp := blog.BlogPost{
				ID:        id,
				CreatedBy: profile.UserID,
			}
			if err := blogPostRepo.Unpublish(bp); err != nil {
				svr.Log(err, fmt.Sprintf("unable to unpublish blog post: ID=%s authorID=%s", id, profile.UserID))
				svr.JSON(w, http.StatusInternalServerError, "Could not unpublish blogpost. Please try again later.")
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func GetAllPublishedBlogPostsHandler(svr server.Server, blogPostRepo *blog.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, err := blogPostRepo.GetAllPublished()
		if err != nil {
			svr.Log(err, "unable to retrieve blogposts")
			svr.TEXT(w, http.StatusNotFound, "could not retrieve blog posts. Please try again later")
			return
		}
		fmt.Println("returning all blogposts", len(all))
		svr.Render(r, w, http.StatusOK, "list-blogposts.html", map[string]interface{}{
			"BlogPosts": all,
		})
	}
}

func GetUserBlogPostsHandler(svr server.Server, blogPostRepo *blog.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			all, err := blogPostRepo.GetByCreatedBy(profile.UserID)
			if err != nil {
				svr.Log(err, "unable to retrieve blogposts")
				svr.TEXT(w, http.StatusNotFound, "could not retrieve blog posts. Please try again later")
				return
			}
			fmt.Println("returning all blogposts", len(all))
			svr.Render(r, w, http.StatusOK, "user-blogposts.html", map[string]interface{}{
				"BlogPosts": all,
			})
		},
	)
}

func ProfileHomepageHandler(svr server.Server, devRepo *developer.Repository, recRepo *recruiter.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			switch profile.Type {
			case user.UserTypeDeveloper:
				dev, err := devRepo.DeveloperProfileByEmail(profile.Email)
				if err != nil {
					svr.Log(err, "unable to find developer profile")
					svr.JSON(w, http.StatusNotFound, nil)
					return
				}
				svr.Render(r, w, http.StatusOK, "profile-home.html", map[string]interface{}{
					"IsAdmin":       profile.IsAdmin,
					"UserID":        profile.UserID,
					"UserEmail":     profile.Email,
					"UserCreatedAt": profile.CreatedAt,
					"ProfileID":     dev.ID,
					"UserType":      profile.Type,
					"Developer":     dev,
					"DevOfferLink1": svr.GetConfig().DevOfferLink1,
					"DevOfferLink2": svr.GetConfig().DevOfferLink2,
					"DevOfferLink3": svr.GetConfig().DevOfferLink3,
					"DevOfferLink4": svr.GetConfig().DevOfferLink4,
					"DevOfferRate1": svr.GetConfig().DevOfferRate1,
					"DevOfferRate2": svr.GetConfig().DevOfferRate2,
					"DevOfferRate3": svr.GetConfig().DevOfferRate3,
					"DevOfferRate4": svr.GetConfig().DevOfferRate4,
					"DevOfferCode1": svr.GetConfig().DevOfferCode1,
					"DevOfferCode2": svr.GetConfig().DevOfferCode2,
					"DevOfferCode3": svr.GetConfig().DevOfferCode3,
					"DevOfferCode4": svr.GetConfig().DevOfferCode4,
				})
			case user.UserTypeRecruiter:
				rec, err := recRepo.RecruiterProfileByEmail(profile.Email)
				if err != nil {
					svr.Log(err, "unable to find recruiter profile")
					svr.JSON(w, http.StatusNotFound, nil)
					return
				}
				svr.Render(r, w, http.StatusOK, "profile-home.html", map[string]interface{}{
					"IsAdmin":              profile.IsAdmin,
					"UserID":               profile.UserID,
					"UserEmail":            profile.Email,
					"UserCreatedAt":        profile.CreatedAt,
					"ProfileID":            rec.ID,
					"UserType":             profile.Type,
					"Recruiter":            rec,
					"StripePublishableKey": svr.GetConfig().StripePublishableKey,
				})
			case user.UserTypeAdmin:
				dev, err := devRepo.DeveloperProfileByEmail(profile.Email)
				if err != nil {
					svr.Log(err, "unable to find developer profile")
					svr.JSON(w, http.StatusNotFound, nil)
					return
				}
				svr.Render(r, w, http.StatusOK, "profile-home.html", map[string]interface{}{
					"IsAdmin":       profile.IsAdmin,
					"UserID":        profile.UserID,
					"UserEmail":     profile.Email,
					"UserCreatedAt": profile.CreatedAt,
					"ProfileID":     dev.ID,
					"UserType":      profile.Type,
					"Developer":     dev,
					"DevOfferLink1": svr.GetConfig().DevOfferLink1,
					"DevOfferLink2": svr.GetConfig().DevOfferLink2,
					"DevOfferLink3": svr.GetConfig().DevOfferLink3,
					"DevOfferLink4": svr.GetConfig().DevOfferLink4,
					"DevOfferRate1": svr.GetConfig().DevOfferRate1,
					"DevOfferRate2": svr.GetConfig().DevOfferRate2,
					"DevOfferRate3": svr.GetConfig().DevOfferRate3,
					"DevOfferRate4": svr.GetConfig().DevOfferRate4,
					"DevOfferCode1": svr.GetConfig().DevOfferCode1,
					"DevOfferCode2": svr.GetConfig().DevOfferCode2,
					"DevOfferCode3": svr.GetConfig().DevOfferCode3,
					"DevOfferCode4": svr.GetConfig().DevOfferCode4,
				})
			}
		},
	)
}

func TriggerExpiredUserSignOnTokensTask(svr server.Server, userRepo *user.Repository) http.HandlerFunc {
	return middleware.MachineAuthenticatedMiddleware(
		svr.GetConfig().MachineToken,
		func(w http.ResponseWriter, r *http.Request) {
			go func() {
				err := userRepo.DeleteExpiredUserSignOnTokens()
				if err != nil {
					svr.Log(err, "unable to delete expired user_sign_on_tokens")
					return
				}
			}()
			svr.JSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
		},
	)
}

func RecruiterJobPosts(svr server.Server, devRepo *developer.Repository, recRepo *recruiter.Repository, jobRepo *job.Repository) http.HandlerFunc {
	return middleware.UserAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			profile, err := middleware.GetUserFromJWT(r, svr.SessionStore, svr.GetJWTSigningKey())
			if err != nil {
				svr.Log(err, "unable to get email from JWT")
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}
			rec, err := recRepo.RecruiterProfileByEmail(profile.Email)
			if err != nil {
				svr.Log(err, "unable to find recruiter profile")
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			page := r.URL.Query().Get("p")
			pageID, err := strconv.Atoi(page)
			if err != nil {
				pageID = 1
			}
			jobsForPage, totalJobCount, err := jobRepo.JobsForRecruiter(profile.Email, pageID, svr.GetConfig().JobsPerPage)
			if err != nil {
				svr.Log(err, "unable to get jobs for recruiter")
				svr.JSON(w, http.StatusInternalServerError, "Oops! An internal error has occurred")
				return
			}
			svr.Render(r, w, http.StatusOK, "recruiter-job-posts.html", map[string]interface{}{
				"Jobs":          jobsForPage,
				"totalJobCount": totalJobCount,
				"IsAdmin":       profile.IsAdmin,
				"UserID":        profile.UserID,
				"UserEmail":     profile.Email,
				"UserCreatedAt": profile.CreatedAt,
				"ProfileID":     rec.ID,
				"UserType":      profile.Type,
				"Recruiter":     rec,
			})
		})
}
