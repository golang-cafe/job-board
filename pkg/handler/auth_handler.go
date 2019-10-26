package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/0x13a/golang.cafe/pkg/authoriser"
	"github.com/0x13a/golang.cafe/pkg/middleware"
	"github.com/0x13a/golang.cafe/pkg/server"

	jwt "github.com/dgrijalva/jwt-go"
)

func GetAuthPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(w, http.StatusOK, "auth.html", nil)
	}
}

func PostAuthPageHandler(svr server.Server, auth authoriser.Authoriser) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		authRq := &authoriser.AuthRq{}
		if err := decoder.Decode(&authRq); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		authRes := auth.ValidAuthRequest(authRq)
		if !authRes.Valid {
			svr.JSON(w, http.StatusUnauthorized, nil)
			return
		}
		sess, err := svr.SessionStore.Get(r, "_gc_session_token")
		if err != nil {
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		stdClaims := &jwt.StandardClaims{
			ExpiresAt: time.Now().Add(30 * 24 * time.Hour).UTC().Unix(),
			IssuedAt:  time.Now().UTC().Unix(),
			Issuer:    "https://golang.cafe",
		}
		claims := middleware.MyCustomClaims{
			IsAdmin:        authRes.IsAdmin,
			Email:          authRes.Email,
			StandardClaims: *stdClaims,
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		ss, err := token.SignedString(svr.GetJWTSigningKey())
		sess.Values["jwt"] = ss
		err = sess.Save(r, w)
		if err != nil {
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}
