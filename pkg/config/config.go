package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
)

type Config struct {
	Port                 string
	DatabaseURL          string
	StripeKey            string
	StripeEndpointSecret string
	StripePublishableKey string
	EmailAPIKey          string
	AdminEmail           string
	AdminPassword        string
	SessionKey           []byte
	JwtSigningKey        []byte
	Env                  string
	IPGeoLocationApiKey  string
	IPGeoLocationURI     string
	MailerLiteAPIKey     string
	SentryDSN            string
	JobsPerPage          int
}

func LoadConfig() (Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		return Config{}, fmt.Errorf("PORT cannot be empty")
	}
	databaseURL := os.Getenv("HEROKU_POSTGRESQL_PINK_URL")
	if databaseURL == "" {
		return Config{}, fmt.Errorf("HEROKU_POSTGRESQL_PINK_URL cannot be empty")
	}
	stripeKey := os.Getenv("STRIPE_KEY")
	if stripeKey == "" {
		return Config{}, fmt.Errorf("STRIPE_KEY cannot be empty")
	}
	stripeEndpointSecret := os.Getenv("STRIPE_ENDPOINT_SECRET")
	if stripeEndpointSecret == "" {
		return Config{}, fmt.Errorf("STRIPE_ENDPOINT_SECRET cannot be empty")
	}
	stripePublishableKey := os.Getenv("STRIPE_PUBLISHABLE_KEY")
	if stripePublishableKey == "" {
		return Config{}, fmt.Errorf("STRIPE_PUBLISHABLE_KEY cannot be empty")
	}
	emailAPIKey := os.Getenv("EMAIL_API_KEY")
	if emailAPIKey == "" {
		return Config{}, fmt.Errorf("EMAIL_API_KEY cannot be empty")
	}
	env := strings.ToLower(os.Getenv("ENV"))
	if env == "" {
		return Config{}, fmt.Errorf("ENV cannot be empty")
	}
	sessionKeyString := os.Getenv("SESSION_KEY")
	if sessionKeyString == "" {
		return Config{}, fmt.Errorf("SESSION_KEY cannot be empty")
	}
	sessionKeyBytes, err := base64.StdEncoding.DecodeString(sessionKeyString)
	if err != nil {
		return Config{}, errors.Wrapf(err, "unable to decode session key to bytes")
	}
	jwtSigningKey := os.Getenv("JWT_SIGNING_KEY")
	if jwtSigningKey == "" {
		return Config{}, fmt.Errorf("JWT_SIGNING_KEY cannot be empty")
	}
	jwtSigningKeyBytes, err := base64.StdEncoding.DecodeString(jwtSigningKey)
	if err != nil {
		return Config{}, errors.Wrapf(err, "unable to decode session key to bytes")
	}
	ipGeolocationAPIKey := os.Getenv("IP_GEOLOCATION_API_KEY")
	if ipGeolocationAPIKey == "" {
		return Config{}, fmt.Errorf("IP_GEOLOCATION_API_KEY cannot be empty")
	}
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		return Config{}, fmt.Errorf("ADMIN_EMAIL cannot be empty")
	}
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		return Config{}, fmt.Errorf("ADMIN_PASSWORD cannot be empty")
	}
	mailerliteAPIKey := os.Getenv("MAILERLITE_API_KEY")
	if mailerliteAPIKey == "" {
		return Config{}, fmt.Errorf("MAILERLITE_API_KEY cannot be empty")
	}
	sentryDSN := os.Getenv("SENTRY_DSN")
	if sentryDSN == "" {
		return Config{}, fmt.Errorf("SENTRY_DSN cannot be empty")
	}

	return Config{
		Port:                 port,
		DatabaseURL:          databaseURL,
		StripeKey:            stripeKey,
		StripeEndpointSecret: stripeEndpointSecret,
		StripePublishableKey: stripePublishableKey,
		EmailAPIKey:          emailAPIKey,
		AdminEmail:           adminEmail,
		AdminPassword:        adminPassword,
		SessionKey:           sessionKeyBytes,
		JwtSigningKey:        jwtSigningKeyBytes,
		Env:                  env,
		IPGeoLocationApiKey:  ipGeolocationAPIKey,
		IPGeoLocationURI:     "https://api.ipgeolocation.io/ipgeo",
		MailerLiteAPIKey:     mailerliteAPIKey,
		SentryDSN:            sentryDSN,
		JobsPerPage:          20,
	}, nil
}
