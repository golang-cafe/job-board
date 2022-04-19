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
	StripeKey                    string // stripe secret API Key
	StripeEndpointSecret         string // stripe endpoint webhook secret token
	StripePublishableKey         string // stripe publishable API key
	EmailAPIKey                  string // sparkpost email API Key
	AdminEmail                   string // used to log on to the management dashboard
	SupportEmail                 string // displayed on the site for support queries
	SessionKey                   []byte
	JwtSigningKey                []byte
	Env                          string // either prod or dev, will disable https and few other bits
	IPGeoLocationGeoliteFile     string
	IPGeoLocationCurrencyMapFile string
	SentryDSN                    string
	JobsPerPage                  int // configures how many jobs are shown per page result
	DevelopersPerPage            int // configures how many dev profiles are shown per page result
	CompaniesPerPage             int // configures how many companies are shown per page result
	TwitterJobsToPost            int // max number of jobs to post each day
	TwitterAccessToken           string
	TwitterAccessTokenSecret     string
	TwitterClientKey             string
	TwitterClientSecret          string
	NewsletterJobsToSend         int
	CloudflareAPIToken           string
	CloudflareZoneTag            string
	CloudflareAPIEndpoint        string
	MachineToken                 string
	TelegramAPIToken             string   // Telegram API Token used to integrate with site's Telegram channel
	TelegramChannelID            int64    // Telegram Channel ID used to integrate with site's Telegram channel
	FXAPIKey                     string   // FX rate api API Key to access recent FX data
	AvailableCurrencies          []string // currencies used throughout the site for salary compensation (post a job, salary filter FX, etc)
	AvailableSalaryBands         []int    // salary upper limits used in search to filter job by minimum salary
	SiteName                     string   // Job site name, in this case is "Golang Cafe"
	SiteJobCategory              string   // Job site category, in this case is "golang"
	SiteHost                     string   // Job site hostname, just the domain name where the site is deployed ie. "golang.cafe"
	SiteGithub                   string   // job site github project url (username+repository name)
	SiteTwitter                  string   // job site twitter account username
	SiteLinkedin                 string
	SiteYoutube                  string
	SiteTelegramChannel          string
	PrimaryColor                 string
	SecondaryColor               string
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
	supportEmail := os.Getenv("SUPPORT_EMAIL")
	if supportEmail == "" {
		return Config{}, fmt.Errorf("SUPPORT_EMAIL cannot be empty")
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
	fxAPIKey := os.Getenv("FX_API_KEY")
	if fxAPIKey == "" {
		return Config{}, fmt.Errorf("FX_API_KEY cannot be empty")
	}
	siteName := os.Getenv("SITE_NAME")
	if siteName == "" {
		return Config{}, fmt.Errorf("SITE_NAME cannot be empty")
	}
	siteJobCategory := os.Getenv("SITE_JOB_CATEGORY")
	if siteJobCategory == "" {
		return Config{}, fmt.Errorf("SITE_JOB_CATEGORU cannot be empty")
	}
	siteHost := os.Getenv("SITE_HOST")
	if siteHost == "" {
		return Config{}, fmt.Errorf("SITE_HOST cannot be empty")
	}
	siteTwitter := os.Getenv("SITE_TWITTER")
	siteGithub := os.Getenv("SITE_GITHUB")
	siteYoutube := os.Getenv("SITE_YOUTUBE")
	siteLinkedin := os.Getenv("SITE_LINKEDIN")
	siteTelegramChannel := os.Getenv("SITE_TELEGRAM_CHANNEL")
	primaryColor := os.Getenv("PRIMARY_COLOR")
	if primaryColor == "" {
		primaryColor = "#000090"
	}
	secondaryColor := os.Getenv("SECONDARY_COLOR")
	if secondaryColor == "" {
		secondaryColor = "#0000c5"
	}

	return Config{
		Port:                         port,
		DatabaseURL:                  databaseURL,
		StripeKey:                    stripeKey,
		StripeEndpointSecret:         stripeEndpointSecret,
		StripePublishableKey:         stripePublishableKey,
		EmailAPIKey:                  emailAPIKey,
		AdminEmail:                   adminEmail,
		SupportEmail:                 supportEmail,
		SessionKey:                   sessionKeyBytes,
		JwtSigningKey:                jwtSigningKeyBytes,
		Env:                          env,
		IPGeoLocationCurrencyMapFile: ipGeolocationCurrencyMapFile,
		IPGeoLocationGeoliteFile:     ipGeolocationGeoliteFile,
		SentryDSN:                    sentryDSN,
		JobsPerPage:                  10,
		DevelopersPerPage:            10,
		CompaniesPerPage:             10,
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
		TelegramAPIToken:             telegramAPIToken,
		TelegramChannelID:            int64(telegramChannelID),
		FXAPIKey:                     fxAPIKey,
		SiteName:                     siteName,
		SiteJobCategory:              siteJobCategory,
		SiteHost:                     siteHost,
		SiteGithub:                   siteGithub,
		SiteTwitter:                  siteTwitter,
		SiteYoutube:                  siteYoutube,
		SiteTelegramChannel:          siteTelegramChannel,
		SiteLinkedin:                 siteLinkedin,
		PrimaryColor:                 primaryColor,
		SecondaryColor:               secondaryColor,
		AvailableCurrencies:          []string{"USD", "EUR", "JPY", "GBP", "AUD", "CAD", "CHF", "CNY", "HKD", "NZD", "SEK", "KRW", "SGD", "NOK", "MXN", "INR", "ZAR", "TRY", "BRL"},
		AvailableSalaryBands:         []int{10000, 20000, 30000, 40000, 50000, 60000, 70000, 80000, 90000, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000, 190000, 200000, 210000, 220000, 230000, 240000, 250000},
	}, nil
}
