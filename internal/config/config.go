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
	Port                     string
	DatabaseUser             string
	DatabasePassword         string
	DatabaseHost             string
	DatabasePort             string
	DatabaseName             string
	DatabaseSSLMode          string
	StripeKey                string // stripe secret API Key
	StripeEndpointSecret     string // stripe endpoint webhook secret token
	StripePublishableKey     string // stripe publishable API key
	SmtpUser                 string
	SmtpPassword             string
	SmtpHost                 string
	AdminEmail               string
	SupportEmail             string // displayed on the site for support queries
	NoReplyEmail             string // used for transactional emails
	SessionKey               []byte
	JwtSigningKey            []byte
	Env                      string // either prod or dev, will disable https and few other bits
	JobsPerPage              int    // configures how many jobs are shown per page result
	DevelopersPerPage        int    // configures how many dev profiles are shown per page result
	CompaniesPerPage         int    // configures how many companies are shown per page result
	TwitterJobsToPost        int    // max number of jobs to post each day
	TwitterAccessToken       string
	TwitterAccessTokenSecret string
	TwitterClientKey         string
	TwitterClientSecret      string
	LinkedinAccessToken      string // LinkedIn OAuth2 bearer token used to share job posts
	LinkedinPageURL          string // LinkedIn author/company page URL or URN for job shares
	NewsletterJobsToSend     int
	CloudflareAPIToken       string
	CloudflareZoneTag        string
	CloudflareAPIEndpoint    string
	MachineToken             string
	TelegramAPIToken         string   // Telegram API Token used to integrate with site's Telegram channel
	TelegramChannelID        int64    // Telegram Channel ID used to integrate with site's Telegram channel
	FXAPIKey                 string   // FX rate api API Key to access recent FX data
	AvailableCurrencies      []string // currencies used throughout the site for salary compensation (post a job, salary filter FX, etc)
	AvailableSalaryBands     []int    // salary upper limits used in search to filter job by minimum salary
	SiteName                 string   // Job site name
	SiteJobCategory          string   // Job site category
	SiteHost                 string   // Job site hostname
	SiteGithub               string   // job site github project url (username+repository name)
	SiteTwitter              string   // job site twitter account username
	SiteLinkedin             string
	SiteYoutube              string
	SiteTelegramChannel      string
	PrimaryColor             string
	SecondaryColor           string
	SiteLogoImageID          string
	PlanID1Price             int // price in cents
	PlanID2Price             int // price in cents
	PlanID3Price             int // price in cents
	DevDirectoryPlanID1Price int // price in cents
	DevDirectoryPlanID2Price int // price in cents
	DevDirectoryPlanID3Price int // price in cents
	DevelopersBannerLink     string
	DevelopersBannerText     string
	URLProtocol              string
	DevOfferLink1            string
	DevOfferLink2            string
	DevOfferLink3            string
	DevOfferLink4            string
	DevOfferRate1            string
	DevOfferRate2            string
	DevOfferRate3            string
	DevOfferRate4            string
	DevOfferCode1            string
	DevOfferCode2            string
	DevOfferCode3            string
	DevOfferCode4            string
}

func LoadConfig() (Config, error) {
	cfg := Config{
		JobsPerPage:          10,
		DevelopersPerPage:    10,
		CompaniesPerPage:     10,
		AvailableCurrencies:  defaultCurrencies(),
		AvailableSalaryBands: defaultSalaryBands(),
	}
	loaders := []func(*Config) error{
		loadCoreAndDatabaseConfig,
		loadStripeAndSmtpConfig,
		loadAuthAndEmailConfig,
		loadSocialAndCloudflareConfig,
		loadSiteAndBrandConfig,
		loadPricingPlansConfig,
	}
	for _, loader := range loaders {
		if err := loader(&cfg); err != nil {
			return Config{}, err
		}
	}
	loadDeveloperOffersConfig(&cfg)
	return cfg, nil
}

func requireEnv(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("%s cannot be empty", key)
	}
	return val, nil
}

func requireIntEnv(key string) (int, error) {
	raw, err := requireEnv(key)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("could not convert ascii to int: %v", err)
	}
	return parsed, nil
}

func loadCoreAndDatabaseConfig(cfg *Config) error {
	var err error
	if cfg.Port, err = requireEnv("PORT"); err != nil {
		return err
	}
	if cfg.DatabaseUser, err = requireEnv("DATABASE_USER"); err != nil {
		return err
	}
	if cfg.DatabasePassword, err = requireEnv("DATABASE_PASSWORD"); err != nil {
		return err
	}
	if cfg.DatabaseHost, err = requireEnv("DATABASE_HOST"); err != nil {
		return err
	}
	if cfg.DatabasePort, err = requireEnv("DATABASE_PORT"); err != nil {
		return err
	}
	if cfg.DatabaseName, err = requireEnv("DATABASE_NAME"); err != nil {
		return err
	}
	cfg.DatabaseSSLMode, err = requireEnv("DATABASE_SSL_MODE")
	return err
}

func loadStripeAndSmtpConfig(cfg *Config) error {
	var err error
	if cfg.StripeKey, err = requireEnv("STRIPE_KEY"); err != nil {
		return err
	}
	if cfg.StripeEndpointSecret, err = requireEnv("STRIPE_ENDPOINT_SECRET"); err != nil {
		return err
	}
	if cfg.StripePublishableKey, err = requireEnv("STRIPE_PUBLISHABLE_KEY"); err != nil {
		return err
	}
	if cfg.SmtpUser, err = requireEnv("SMTP_USER"); err != nil {
		return err
	}
	if cfg.SmtpPassword, err = requireEnv("SMTP_PASSWORD"); err != nil {
		return err
	}
	cfg.SmtpHost, err = requireEnv("SMTP_HOST")
	return err
}

func loadAuthAndEmailConfig(cfg *Config) error {
	env := strings.ToLower(os.Getenv("ENV"))
	if env == "" {
		return fmt.Errorf("ENV cannot be empty")
	}
	cfg.Env = env
	cfg.URLProtocol = "https://"
	if strings.EqualFold(env, "dev") {
		cfg.URLProtocol = "http://"
	}
	sessionBytes, err := decodeBase64Env("SESSION_KEY")
	if err != nil {
		return err
	}
	cfg.SessionKey = sessionBytes
	jwtBytes, err := decodeBase64Env("JWT_SIGNING_KEY")
	if err != nil {
		return err
	}
	cfg.JwtSigningKey = jwtBytes
	if cfg.AdminEmail, err = requireEnv("ADMIN_EMAIL"); err != nil {
		return err
	}
	if cfg.SupportEmail, err = requireEnv("SUPPORT_EMAIL"); err != nil {
		return err
	}
	cfg.NoReplyEmail, err = requireEnv("NO_REPLY_EMAIL")
	return err
}

func decodeBase64Env(key string) ([]byte, error) {
	raw, err := requireEnv(key)
	if err != nil {
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to decode session key to bytes")
	}
	return decoded, nil
}

func loadSocialAndCloudflareConfig(cfg *Config) error {
	if err := loadTwitterAndLinkedinConfig(cfg); err != nil {
		return err
	}
	var err error
	if cfg.NewsletterJobsToSend, err = requireIntEnv("NEWSLETTER_JOBS_TO_SEND"); err != nil {
		return err
	}
	if cfg.CloudflareAPIToken, err = requireEnv("CLOUDFLARE_API_TOKEN"); err != nil {
		return err
	}
	if cfg.CloudflareZoneTag, err = requireEnv("CLOUDFLARE_ZONE_TAG"); err != nil {
		return err
	}
	if cfg.CloudflareAPIEndpoint, err = requireEnv("CLOUDFLARE_API_ENDPOINT"); err != nil {
		return err
	}
	if cfg.MachineToken, err = requireEnv("MACHINE_TOKEN"); err != nil {
		return err
	}
	return loadTelegramAndFxConfig(cfg)
}

func loadTwitterAndLinkedinConfig(cfg *Config) error {
	var err error
	if cfg.TwitterAccessToken, err = requireEnv("TWITTER_ACCESS_TOKEN"); err != nil {
		return err
	}
	if cfg.TwitterAccessTokenSecret, err = requireEnv("TWITTER_ACCESS_TOKEN_SECRET"); err != nil {
		return err
	}
	if cfg.TwitterClientKey, err = requireEnv("TWITTER_CLIENT_KEY"); err != nil {
		return err
	}
	if cfg.TwitterClientSecret, err = requireEnv("TWITTER_CLIENT_SECRET"); err != nil {
		return err
	}
	if cfg.TwitterJobsToPost, err = requireIntEnv("TWITTER_JOBS_TO_POST"); err != nil {
		return err
	}
	cfg.LinkedinAccessToken = os.Getenv("LINKEDIN_ACCESS_TOKEN")
	cfg.LinkedinPageURL = os.Getenv("LINKEDIN_PAGE_URL")
	return nil
}

func loadTelegramAndFxConfig(cfg *Config) error {
	var err error
	if cfg.TelegramAPIToken, err = requireEnv("TELEGRAM_API_TOKEN"); err != nil {
		return err
	}
	channelIDStr, err := requireEnv("TELEGRAM_CHANNEL_ID")
	if err != nil {
		return err
	}
	channelID, err := strconv.Atoi(channelIDStr)
	if err != nil {
		return errors.Wrap(err, "unable to convert telegram channel id to int")
	}
	cfg.TelegramChannelID = int64(channelID)
	cfg.FXAPIKey, err = requireEnv("FX_API_KEY")
	return err
}

func loadSiteAndBrandConfig(cfg *Config) error {
	var err error
	if cfg.SiteName, err = requireEnv("SITE_NAME"); err != nil {
		return err
	}
	if cfg.SiteJobCategory = os.Getenv("SITE_JOB_CATEGORY"); cfg.SiteJobCategory == "" {
		return fmt.Errorf("SITE_JOB_CATEGORU cannot be empty")
	}
	if cfg.SiteHost, err = requireEnv("SITE_HOST"); err != nil {
		return err
	}
	if cfg.SiteLogoImageID, err = requireEnv("SITE_LOGO_IMAGE_ID"); err != nil {
		return err
	}
	cfg.SiteTwitter = os.Getenv("SITE_TWITTER")
	cfg.SiteGithub = os.Getenv("SITE_GITHUB")
	cfg.SiteYoutube = os.Getenv("SITE_YOUTUBE")
	cfg.SiteLinkedin = os.Getenv("SITE_LINKEDIN")
	cfg.SiteTelegramChannel = os.Getenv("SITE_TELEGRAM_CHANNEL")
	cfg.PrimaryColor = defaultOrEnv("PRIMARY_COLOR", "#000090")
	cfg.SecondaryColor = defaultOrEnv("SECONDARY_COLOR", "#0000c5")
	return nil
}

func defaultOrEnv(key, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}

func loadPricingPlansConfig(cfg *Config) error {
	var err error
	if cfg.PlanID1Price, err = requireIntEnv("PLAN_ID_1_PRICE"); err != nil {
		return err
	}
	if cfg.PlanID2Price, err = requireIntEnv("PLAN_ID_2_PRICE"); err != nil {
		return err
	}
	if cfg.PlanID3Price, err = requireIntEnv("PLAN_ID_3_PRICE"); err != nil {
		return err
	}
	if cfg.DevDirectoryPlanID1Price, err = requireIntEnv("DEV_DIRECTORY_PLAN_ID_1_PRICE"); err != nil {
		return err
	}
	if cfg.DevDirectoryPlanID2Price, err = requireIntEnv("DEV_DIRECTORY_PLAN_ID_2_PRICE"); err != nil {
		return err
	}
	cfg.DevDirectoryPlanID3Price, err = requireIntEnv("DEV_DIRECTORY_PLAN_ID_3_PRICE")
	return err
}

func loadDeveloperOffersConfig(cfg *Config) {
	cfg.DevelopersBannerLink = os.Getenv("DEVELOPERS_BANNER_LINK")
	cfg.DevelopersBannerText = os.Getenv("DEVELOPERS_BANNER_TEXT")
	cfg.DevOfferLink1 = os.Getenv("DEV_OFFER_LINK_1")
	cfg.DevOfferLink2 = os.Getenv("DEV_OFFER_LINK_2")
	cfg.DevOfferLink3 = os.Getenv("DEV_OFFER_LINK_3")
	cfg.DevOfferLink4 = os.Getenv("DEV_OFFER_LINK_4")
	cfg.DevOfferRate1 = os.Getenv("DEV_OFFER_RATE_1")
	cfg.DevOfferRate2 = os.Getenv("DEV_OFFER_RATE_2")
	cfg.DevOfferRate3 = os.Getenv("DEV_OFFER_RATE_3")
	cfg.DevOfferRate4 = os.Getenv("DEV_OFFER_RATE_4")
	cfg.DevOfferCode1 = os.Getenv("DEV_OFFER_CODE_1")
	cfg.DevOfferCode2 = os.Getenv("DEV_OFFER_CODE_2")
	cfg.DevOfferCode3 = os.Getenv("DEV_OFFER_CODE_3")
	cfg.DevOfferCode4 = os.Getenv("DEV_OFFER_CODE_4")
}

func defaultCurrencies() []string {
	return []string{"USD", "EUR", "JPY", "GBP", "AUD", "CAD", "CHF", "CNY", "HKD", "NZD", "SEK", "KRW", "SGD", "NOK", "MXN", "INR", "ZAR", "TRY", "BRL"}
}

func defaultSalaryBands() []int {
	return []int{10000, 20000, 30000, 40000, 50000, 60000, 70000, 80000, 90000, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000, 190000, 200000, 210000, 220000, 230000, 240000, 250000}
}
