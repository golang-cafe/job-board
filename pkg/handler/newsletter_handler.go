package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/0x13a/golang.cafe/pkg/email"
	"github.com/0x13a/golang.cafe/pkg/server"
)

type SubscribeRqMailerlite struct {
	Email  string                 `json:"email"`
	Fields map[string]interface{} `json:"fields"`
}

func ViewNewsletterPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, r, "", "", "", "newsletter.html")
	}
}

func ViewShopPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, r, "", "", "", "shop.html")
	}
}

func ViewCommunityNewsletterPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, r, "", "", "", "news.html")
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

func BlogListHandler(svr server.Server, blogDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		files, err := ioutil.ReadDir(blogDir)
		if err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		posts := make([]struct{ Title, Path string }, 0, len(files))
		for _, f := range files {
			posts = append(posts, struct{ Title, Path string }{
				Title: strings.Title(
					strings.ReplaceAll(
						strings.ReplaceAll(f.Name(), ".html", ""),
						"-",
						" ",
					)),
				Path: f.Name(),
			})
		}

		svr.Render(w, http.StatusOK, "blog.html", map[string]interface{}{
			"Posts":        posts,
			"MonthAndYear": time.Now().UTC().Format("January 2006"),
		})
	}
}

func ViewSupportPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderPageForLocationAndTag(w, r, "", "", "", "support.html")
	}
}

func SaveMemberToNewsletterPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		email := strings.ToLower(r.URL.Query().Get("email"))
		if err := svr.SaveSubscriber(email); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}
