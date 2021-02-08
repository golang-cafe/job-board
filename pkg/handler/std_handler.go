package handler

import (
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
	jwt "github.com/dgrijalva/jwt-go"
	humanize "github.com/dustin/go-humanize"
	"github.com/gorilla/feeds"
	"github.com/gorilla/mux"
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
