package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type Config struct {
	Port                         string
	DatabaseURL                  string
	StripeKey                    string
	StripeEndpointSecret         string
	StripePublishableKey         string
	EmailAPIKey                  string
	AdminEmail                   string
	SessionKey                   []byte
	JwtSigningKey                []byte
	Env                          string
	IPGeoLocationGeoliteFile     string
	IPGeoLocationCurrencyMapFile string
	MailerLiteAPIKey             string
	SentryDSN                    string
	JobsPerPage                  int
	DevelopersPerPage            int
	CompaniesPerPage             int
	TwitterJobsToPost            int
	TwitterAccessToken           string
	TwitterAccessTokenSecret     string
	TwitterClientKey             string
	TwitterClientSecret          string
	NewsletterJobsToSend         int
	CloudflareAPIToken           string
	CloudflareZoneTag            string
	CloudflareAPIEndpoint        string
	MachineToken                 string
	WhatsappLink                 string
	PhoneNumber                  string
	TelegramAPIToken             string
	TelegramChannelID            int64
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
	ipGeolocationCurrencyMapFile := os.Getenv("IP_GEOLOCATION_CURRENCY_MAPPING_FILE")
	if ipGeolocationCurrencyMapFile == "" {
		return Config{}, fmt.Errorf("IP_GEOLOCATION_CURRENCY_MAPPING_FILE cannot be empty")
	}
	ipGeolocationGeoliteFile := os.Getenv("IP_GEOLOCATION_GEOLITE_DB_FILE")
	if ipGeolocationGeoliteFile == "" {
		return Config{}, fmt.Errorf("IP_GEOLOCATION_GEOLITE_DB_FILE cannot be empty")
	}
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		return Config{}, fmt.Errorf("ADMIN_EMAIL cannot be empty")
	}
	mailerliteAPIKey := os.Getenv("MAILERLITE_API_KEY")
	if mailerliteAPIKey == "" {
		return Config{}, fmt.Errorf("MAILERLITE_API_KEY cannot be empty")
	}
	sentryDSN := os.Getenv("SENTRY_DSN")
	if sentryDSN == "" {
		return Config{}, fmt.Errorf("SENTRY_DSN cannot be empty")
	}
	twitterAccessToken := os.Getenv("TWITTER_ACCESS_TOKEN")
	if twitterAccessToken == "" {
		return Config{}, fmt.Errorf("TWITTER_ACCESS_TOKEN cannot be empty")
	}
	twitterAccessTokenSecret := os.Getenv("TWITTER_ACCESS_TOKEN_SECRET")
	if twitterAccessTokenSecret == "" {
		return Config{}, fmt.Errorf("TWITTER_ACCESS_TOKEN_SECRET cannot be empty")
	}
	twitterClientKey := os.Getenv("TWITTER_CLIENT_KEY")
	if twitterClientKey == "" {
		return Config{}, fmt.Errorf("TWITTER_CLIENT_KEY cannot be empty")
	}
	twitterClientSecret := os.Getenv("TWITTER_CLIENT_SECRET")
	if twitterClientSecret == "" {
		return Config{}, fmt.Errorf("TWITTER_CLIENT_SECRET cannot be empty")
	}
	twitterJobsToPostStr := os.Getenv("TWITTER_JOBS_TO_POST")
	if twitterJobsToPostStr == "" {
		return Config{}, fmt.Errorf("TWITTER_JOBS_TO_POST cannot be empty")
	}
	twitterJobsToPost, err := strconv.Atoi(twitterJobsToPostStr)
	if err != nil {
		return Config{}, fmt.Errorf("could not convert ascii to int: %v", err)
	}
	newsletterJobsToSendStr := os.Getenv("NEWSLETTER_JOBS_TO_SEND")
	if newsletterJobsToSendStr == "" {
		return Config{}, fmt.Errorf("NEWSLETTER_JOBS_TO_SEND cannot be empty")
	}
	newsletterJobsToSend, err := strconv.Atoi(newsletterJobsToSendStr)
	if err != nil {
		return Config{}, fmt.Errorf("could not convert ascii to int: %v", err)
	}
	cloudflareAPIToken := os.Getenv("CLOUDFLARE_API_TOKEN")
	if cloudflareAPIToken == "" {
		return Config{}, fmt.Errorf("CLOUDFLARE_API_TOKEN cannot be empty")
	}
	cloudflareZoneTag := os.Getenv("CLOUDFLARE_ZONE_TAG")
	if cloudflareZoneTag == "" {
		return Config{}, fmt.Errorf("CLOUDFLARE_ZONE_TAG cannot be empty")
	}
	cloudflareAPIEndpoint := os.Getenv("CLOUDFLARE_API_ENDPOINT")
	if cloudflareAPIEndpoint == "" {
		return Config{}, fmt.Errorf("CLOUDFLARE_API_ENDPOINT cannot be empty")
	}
	machineToken := os.Getenv("MACHINE_TOKEN")
	if machineToken == "" {
		return Config{}, fmt.Errorf("MACHINE_TOKEN cannot be empty")
	}
	whatsappLink := os.Getenv("WHATSAPP_LINK")
	if machineToken == "" {
		return Config{}, fmt.Errorf("WHATSAPP_LINK cannot be empty")
	}
	phoneNumber := os.Getenv("PHONE_NUMBER")
	if machineToken == "" {
		return Config{}, fmt.Errorf("PHONE_NUMBER cannot be empty")
	}
	telegramAPIToken := os.Getenv("TELEGRAM_API_TOKEN")
	if telegramAPIToken == "" {
		return Config{}, fmt.Errorf("TELEGRAM_API_TOKEN cannot be empty")
	}
	telegramChannelIDStr := os.Getenv("TELEGRAM_CHANNEL_ID")
	if telegramChannelIDStr == "" {
		return Config{}, fmt.Errorf("TELEGRAM_CHANNEL_ID cannot be empty")
	}
	telegramChannelID, err := strconv.Atoi(telegramChannelIDStr)
	if err != nil {
		return Config{}, errors.Wrap(err, "unable to convert telegram channel id to int")
	}

	return Config{
		Port:                         port,
		DatabaseURL:                  databaseURL,
		StripeKey:                    stripeKey,
		StripeEndpointSecret:         stripeEndpointSecret,
		StripePublishableKey:         stripePublishableKey,
		EmailAPIKey:                  emailAPIKey,
		AdminEmail:                   adminEmail,
		SessionKey:                   sessionKeyBytes,
		JwtSigningKey:                jwtSigningKeyBytes,
		Env:                          env,
		IPGeoLocationCurrencyMapFile: ipGeolocationCurrencyMapFile,
		IPGeoLocationGeoliteFile:     ipGeolocationGeoliteFile,
		MailerLiteAPIKey:             mailerliteAPIKey,
		SentryDSN:                    sentryDSN,
		JobsPerPage:                  20,
		DevelopersPerPage:            10,
		CompaniesPerPage:             20,
		TwitterJobsToPost:            twitterJobsToPost,
		TwitterAccessToken:           twitterAccessToken,
		TwitterAccessTokenSecret:     twitterAccessTokenSecret,
		TwitterClientSecret:          twitterClientSecret,
		TwitterClientKey:             twitterClientKey,
		NewsletterJobsToSend:         newsletterJobsToSend,
		CloudflareAPIToken:           cloudflareAPIToken,
		CloudflareZoneTag:            cloudflareZoneTag,
		CloudflareAPIEndpoint:        cloudflareAPIEndpoint,
		MachineToken:                 machineToken,
		WhatsappLink:                 whatsappLink,
		PhoneNumber:                  phoneNumber,
		TelegramAPIToken:             telegramAPIToken,
		TelegramChannelID:            int64(telegramChannelID),
	}, nil
}
