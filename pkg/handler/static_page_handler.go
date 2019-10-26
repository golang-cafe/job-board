package handler

import "net/http"

func AboutPageHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/views/about.html")
}

func PrivacyPolicyPageHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/views/privacy-policy.html")
}

func TermsOfServicePageHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/views/terms-of-service.html")
}
