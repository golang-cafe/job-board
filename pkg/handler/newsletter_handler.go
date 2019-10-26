package handler

import (
	"net/http"
	"strings"
	"fmt"
	"regexp"
	"encoding/json"
	"errors"
	"bytes"

	"github.com/0x13a/golang.cafe/pkg/server"
)

type SubscribeRqMailerlite struct {
	Email  string                 `json:"email"`
	Fields map[string]interface{} `json:"fields"`
}

func ViewNewsletterPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, "", "", "", "newsletter.html")
	}
}

func ViewCommunityNewsletterPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, "", "", "", "community.html")
	}
}

func SaveMemberToCommunityNewsletterPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := strings.ToLower(r.URL.Query().Get("email"))
		communityType := strings.ToLower(r.URL.Query().Get("type"))
		emailRe := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
		if !emailRe.MatchString(email) {
			svr.Log(errors.New("invalid email provided"), fmt.Sprintf("invalid email provided: %v", email))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		defer r.Body.Close()
		mailerliteRq := &SubscribeRqMailerlite{}
		mailerliteRq.Fields = make(map[string]interface{})
		mailerliteRq.Email = email
		if communityType == "slack" || communityType == "forum" {
			mailerliteRq.Fields["community_type"] = communityType
		} else {
			mailerliteRq.Fields["community_type"] = "slack"
		}
		jsonMailerliteRq, err := json.Marshal(mailerliteRq)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to marshal mailerliteRq %v: %v", mailerliteRq, err))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		// send http request to mailerlite
		client := &http.Client{}
		req, err := http.NewRequest("POST", "https://api.mailerlite.com/api/v2/subscribers", bytes.NewBuffer(jsonMailerliteRq))
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to create req for mailerlite %v: %v", jsonMailerliteRq, err))
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		req.Header.Add("X-MailerLite-ApiKey", svr.GetConfig().MailerLiteAPIKey)
		req.Header.Add("content-type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to save subscriber to mailerlite %v: %v", jsonMailerliteRq, err))
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			svr.Log(errors.New("got non 200 status code from mailerlite"), fmt.Sprintf("got non 200 status code: %v req: %v", res.StatusCode, jsonMailerliteRq))
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func SaveMemberToNewsletterPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := strings.ToLower(r.URL.Query().Get("email"))
		frequency := strings.ToLower(r.URL.Query().Get("frequency"))
		emailRe := regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
		if !emailRe.MatchString(email) {
			svr.Log(errors.New("invalid email provided"), fmt.Sprintf("invalid email provided: %v", email))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		defer r.Body.Close()
		mailerliteRq := &SubscribeRqMailerlite{}
		mailerliteRq.Fields = make(map[string]interface{})
		mailerliteRq.Email = email
		if frequency == "weekly" || frequency == "daily" {
			mailerliteRq.Fields["frequency"] = frequency
		} else {
			mailerliteRq.Fields["frequency"] = "weekly"
		}
		jsonMailerliteRq, err := json.Marshal(mailerliteRq)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to marshal mailerliteRq %v: %v", mailerliteRq, err))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		// send http request to mailerlite
		client := &http.Client{}
		req, err := http.NewRequest("POST", "https://api.mailerlite.com/api/v2/subscribers", bytes.NewBuffer(jsonMailerliteRq))
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to create req for mailerlite %v: %v", jsonMailerliteRq, err))
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		req.Header.Add("X-MailerLite-ApiKey", svr.GetConfig().MailerLiteAPIKey)
		req.Header.Add("content-type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to save subscriber to mailerlite %v: %v", jsonMailerliteRq, err))
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			svr.Log(errors.New("got non 200 status code from mailerlite"), fmt.Sprintf("got non 200 status code: %v req: %v", res.StatusCode, jsonMailerliteRq))
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}
