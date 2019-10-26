package handler

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

func SitemapIndexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/sitemap.xml")
}

func SitemapHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	n := vars["n"]
	http.ServeFile(w, r, fmt.Sprintf("static/sitemap-%s.xml.gz", n))
}
