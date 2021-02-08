package middleware

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/0x13a/golang.cafe/pkg/gzip"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/gorilla/sessions"
	"github.com/rs/zerolog"
)

func HTTPSMiddleware(next http.Handler, env string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if env != "dev" && r.Header.Get("X-Forwarded-Proto") != "https" {
			target := "https://" + r.Host + r.URL.Path
			http.Redirect(w, r, target, http.StatusMovedPermanently)
		}

		next.ServeHTTP(w, r)
	})
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
			With().
			Timestamp().
			Logger()
		logger.Info().
			Str("Host", r.Host).
			Str("method", r.Method).
			Stringer("url", r.URL).
			Str("x-forwarded-for", r.Header.Get("x-forwarded-for")).
			Str("cf-ipcountry", r.Header.Get("cf-ipcountry")).
			Msg("req")
		next.ServeHTTP(w, r)
	})
}

func HeadersMiddleware(next http.Handler, env string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if env != "dev" {
			// filter out HeadlessChrome user agent
			if strings.Contains(r.Header.Get("User-Agent"), "HeadlessChrome") {
				w.WriteHeader(http.StatusTeapot)
				return
			}
			w.Header().Set("Content-Security-Policy", "upgrade-insecure-requests")
			w.Header().Set("X-Frame-Options", "deny")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			w.Header().Set("Referrer-Policy", "origin")
		}
		next.ServeHTTP(w, r)
	})
}

func GzipMiddleware(next http.Handler) http.Handler {
	return gzip.GzipHandler(next)
}

type UserJWT struct {
	IsAdmin   bool      `json:"is_admin"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	jwt.StandardClaims
}

func AdminAuthenticatedMiddleware(sessionStore *sessions.CookieStore, jwtKey []byte, next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := sessionStore.Get(r, "____gc")
		if err != nil {
			http.Redirect(w, r, "/auth", http.StatusUnauthorized)
			return
		}
		tk, ok := sess.Values["jwt"].(string)
		if !ok {
			http.Redirect(w, r, "/auth", http.StatusUnauthorized)
			return
		}
		token, err := jwt.ParseWithClaims(tk, &UserJWT{}, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})
		if !token.Valid {
			http.Redirect(w, r, "/auth", http.StatusUnauthorized)
			return
		}
		claims, ok := token.Claims.(*UserJWT)
		if !ok {
			http.Redirect(w, r, "/auth", http.StatusUnauthorized)
			return
		}
		if !claims.IsAdmin {
			http.Redirect(w, r, "/auth", http.StatusUnauthorized)
			return
		}
		next(w, r)
	})
}

func UserAuthenticatedMiddleware(sessionStore *sessions.CookieStore, jwtKey []byte, next http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, err := sessionStore.Get(r, "____gc")
		if err != nil {
			http.Redirect(w, r, "/auth", http.StatusUnauthorized)
			return
		}
		tk, ok := sess.Values["jwt"].(string)
		if !ok {
			http.Redirect(w, r, "/auth", http.StatusUnauthorized)
			return
		}
		token, err := jwt.ParseWithClaims(tk, &UserJWT{}, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})
		if !token.Valid {
			http.Redirect(w, r, "/auth", http.StatusUnauthorized)
			return
		}
		claims, ok := token.Claims.(*UserJWT)
		if !ok || claims.Email == "" {
			http.Redirect(w, r, "/auth", http.StatusUnauthorized)
			return
		}
		next(w, r)
	})
}

func GetUserFromJWT(r *http.Request, sessionStore *sessions.CookieStore, jwtKey []byte) (*UserJWT, error) {
	sess, err := sessionStore.Get(r, "____gc")
	if err != nil {
		return nil, errors.New("could not find cookie")
	}
	tk, ok := sess.Values["jwt"].(string)
	if !ok {
		return nil, errors.New("could not find jwt in session")
	}
	token, err := jwt.ParseWithClaims(tk, &UserJWT{}, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if !token.Valid {
		return nil, errors.New("token is expired")
	}
	claims, ok := token.Claims.(*UserJWT)
	if !ok {
		return nil, errors.New("could not convert jwt claims to UserJWT")
	}
	return claims, nil
}

func IsSignedOn(r *http.Request, sessionStore *sessions.CookieStore, jwtKey []byte) bool {
	sess, err := sessionStore.Get(r, "____gc")
	if err != nil {
		return false
	}
	tk, ok := sess.Values["jwt"].(string)
	if !ok {
		return false
	}
	token, err := jwt.ParseWithClaims(tk, &UserJWT{}, func(token *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if !token.Valid {
		return false
	}
	if !ok {
		return false
	}
	return true
}
