package handler

import (
	"net/http"
	"strings"

	"github.com/0x13a/golang.cafe/pkg/server"
	"github.com/gorilla/mux"
)

func SalaryLandingPageLocationPlaceholderHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		location := strings.ReplaceAll(vars["location"], "-", " ")
		svr.RenderSalaryForLocation(w, r, location)
	}
}

func SalaryLandingPageLocationHandler(svr server.Server, location string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.RenderSalaryForLocation(w, r, location)
	}
}
