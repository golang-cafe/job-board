package handler

import "net/http"

func RobotsTxtHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/robots.txt")
}

func WellKnownSecurityHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "static/security.txt")
}
