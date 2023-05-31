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
	port := os.Getenv("PORT")
	if port == "" {
		return Config{}, fmt.Errorf("PORT cannot be empty")
	}
	databaseUser := os.Getenv("DATABASE_USER")
	if databaseUser == "" {
		return Config{}, fmt.Errorf("DATABASE_USER cannot be empty")
	}
	databasePassword := os.Getenv("DATABASE_PASSWORD")
	if databasePassword == "" {
		return Config{}, fmt.Errorf("DATABASE_PASSWORD cannot be empty")
	}
	databaseHost := os.Getenv("DATABASE_HOST")
	if databaseHost == "" {
		return Config{}, fmt.Errorf("DATABASE_HOST cannot be empty")
	}
	databasePort := os.Getenv("DATABASE_PORT")
	if databasePort == "" {
		return Config{}, fmt.Errorf("DATABASE_PORT cannot be empty")
	}
	databaseName := os.Getenv("DATABASE_NAME")
	if databaseName == "" {
		return Config{}, fmt.Errorf("DATABASE_NAME cannot be empty")
	}
	databaseSSLMode := os.Getenv("DATABASE_SSL_MODE")
	if databaseSSLMode == "" {
		return Config{}, fmt.Errorf("DATABASE_SSL_MODE cannot be empty")
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
	smtpUser := os.Getenv("SMTP_USER")
	if smtpUser == "" {
		return Config{}, fmt.Errorf("SMTP_USER cannot be empty")
	}
	smtpPassword := os.Getenv("SMTP_PASSWORD")
	if smtpPassword == "" {
		return Config{}, fmt.Errorf("SMTP_PASSWORD cannot be empty")
	}
	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		return Config{}, fmt.Errorf("SMTP_HOST cannot be empty")
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
	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		return Config{}, fmt.Errorf("ADMIN_EMAIL cannot be empty")
	}
	supportEmail := os.Getenv("SUPPORT_EMAIL")
	if supportEmail == "" {
		return Config{}, fmt.Errorf("SUPPORT_EMAIL cannot be empty")
	}
	noReplyEmail := os.Getenv("NO_REPLY_EMAIL")
	if noReplyEmail == "" {
		return Config{}, fmt.Errorf("NO_REPLY_EMAIL cannot be empty")
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
	siteLogoImageID := os.Getenv("SITE_LOGO_IMAGE_ID")
	if siteLogoImageID == "" {
		return Config{}, fmt.Errorf("SITE_LOGO_IMAGE_ID cannot be empty")
	}
	planID1PriceStr := os.Getenv("PLAN_ID_1_PRICE")
	if planID1PriceStr == "" {
		return Config{}, fmt.Errorf("PLAN_ID_1_PRICE cannot be empty")
	}
	planID1Price, err := strconv.Atoi(planID1PriceStr)
	if err != nil {
		return Config{}, fmt.Errorf("could not convert ascii to int: %v", err)
	}
	planID2PriceStr := os.Getenv("PLAN_ID_2_PRICE")
	if planID2PriceStr == "" {
		return Config{}, fmt.Errorf("PLAN_ID_2_PRICE cannot be empty")
	}
	planID2Price, err := strconv.Atoi(planID2PriceStr)
	if err != nil {
		return Config{}, fmt.Errorf("could not convert ascii to int: %v", err)
	}
	planID3PriceStr := os.Getenv("PLAN_ID_3_PRICE")
	if planID3PriceStr == "" {
		return Config{}, fmt.Errorf("PLAN_ID_3_PRICE cannot be empty")
	}
	planID3Price, err := strconv.Atoi(planID3PriceStr)
	if err != nil {
		return Config{}, fmt.Errorf("could not convert ascii to int: %v", err)
	}
	devDirectoryPlanID1PriceStr := os.Getenv("DEV_DIRECTORY_PLAN_ID_1_PRICE")
	if devDirectoryPlanID1PriceStr == "" {
		return Config{}, fmt.Errorf("DEV_DIRECTORY_PLAN_ID_1_PRICE cannot be empty")
	}
	devDirectoryPlanID1Price, err := strconv.Atoi(devDirectoryPlanID1PriceStr)
	if err != nil {
		return Config{}, fmt.Errorf("could not convert ascii to int: %v", err)
	}
	devDirectoryPlanID2PriceStr := os.Getenv("DEV_DIRECTORY_PLAN_ID_2_PRICE")
	if devDirectoryPlanID2PriceStr == "" {
		return Config{}, fmt.Errorf("DEV_DIRECTORY_PLAN_ID_2_PRICE cannot be empty")
	}
	devDirectoryPlanID2Price, err := strconv.Atoi(devDirectoryPlanID2PriceStr)
	if err != nil {
		return Config{}, fmt.Errorf("could not convert ascii to int: %v", err)
	}
	devDirectoryPlanID3PriceStr := os.Getenv("DEV_DIRECTORY_PLAN_ID_3_PRICE")
	if devDirectoryPlanID3PriceStr == "" {
		return Config{}, fmt.Errorf("DEV_DIRECTORY_PLAN_ID_3_PRICE cannot be empty")
	}
	devDirectoryPlanID3Price, err := strconv.Atoi(devDirectoryPlanID3PriceStr)
	if err != nil {
		return Config{}, fmt.Errorf("could not convert ascii to int: %v", err)
	}
	developersBannerLink := os.Getenv("DEVELOPERS_BANNER_LINK")
	developersBannerText := os.Getenv("DEVELOPERS_BANNER_TEXT")
	urlProtocol := "http://"
	if !strings.EqualFold(env, "dev") {
		urlProtocol = "https://"
	}
	devOfferLink1 := os.Getenv("DEV_OFFER_LINK_1")
	devOfferLink2 := os.Getenv("DEV_OFFER_LINK_2")
	devOfferLink3 := os.Getenv("DEV_OFFER_LINK_3")
	devOfferLink4 := os.Getenv("DEV_OFFER_LINK_4")
	devOfferRate1 := os.Getenv("DEV_OFFER_RATE_1")
	devOfferRate2 := os.Getenv("DEV_OFFER_RATE_2")
	devOfferRate3 := os.Getenv("DEV_OFFER_RATE_3")
	devOfferRate4 := os.Getenv("DEV_OFFER_RATE_4")
	devOfferCode1 := os.Getenv("DEV_OFFER_CODE_1")
	devOfferCode2 := os.Getenv("DEV_OFFER_CODE_2")
	devOfferCode3 := os.Getenv("DEV_OFFER_CODE_3")
	devOfferCode4 := os.Getenv("DEV_OFFER_CODE_4")

	return Config{
		Port:                     port,
		DatabaseUser:             databaseUser,
		DatabasePassword:         databasePassword,
		DatabaseHost:             databaseHost,
		DatabasePort:             databasePort,
		DatabaseName:             databaseName,
		DatabaseSSLMode:          databaseSSLMode,
		StripeKey:                stripeKey,
		StripeEndpointSecret:     stripeEndpointSecret,
		StripePublishableKey:     stripePublishableKey,
		SmtpUser:                 smtpUser,
		SmtpPassword:             smtpPassword,
		SmtpHost:                 smtpHost,
		AdminEmail:               adminEmail,
		SupportEmail:             supportEmail,
		NoReplyEmail:             noReplyEmail,
		SessionKey:               sessionKeyBytes,
		JwtSigningKey:            jwtSigningKeyBytes,
		Env:                      env,
		JobsPerPage:              10,
		DevelopersPerPage:        10,
		CompaniesPerPage:         10,
		TwitterJobsToPost:        twitterJobsToPost,
		TwitterAccessToken:       twitterAccessToken,
		TwitterAccessTokenSecret: twitterAccessTokenSecret,
		TwitterClientSecret:      twitterClientSecret,
		TwitterClientKey:         twitterClientKey,
		NewsletterJobsToSend:     newsletterJobsToSend,
		CloudflareAPIToken:       cloudflareAPIToken,
		CloudflareZoneTag:        cloudflareZoneTag,
		CloudflareAPIEndpoint:    cloudflareAPIEndpoint,
		MachineToken:             machineToken,
		TelegramAPIToken:         telegramAPIToken,
		TelegramChannelID:        int64(telegramChannelID),
		FXAPIKey:                 fxAPIKey,
		SiteName:                 siteName,
		SiteJobCategory:          siteJobCategory,
		SiteHost:                 siteHost,
		SiteGithub:               siteGithub,
		SiteTwitter:              siteTwitter,
		SiteYoutube:              siteYoutube,
		SiteTelegramChannel:      siteTelegramChannel,
		SiteLinkedin:             siteLinkedin,
		PrimaryColor:             primaryColor,
		SecondaryColor:           secondaryColor,
		SiteLogoImageID:          siteLogoImageID,
		AvailableCurrencies:      []string{"USD", "EUR", "JPY", "GBP", "AUD", "CAD", "CHF", "CNY", "HKD", "NZD", "SEK", "KRW", "SGD", "NOK", "MXN", "INR", "ZAR", "TRY", "BRL"},
		AvailableSalaryBands:     []int{10000, 20000, 30000, 40000, 50000, 60000, 70000, 80000, 90000, 100000, 110000, 120000, 130000, 140000, 150000, 160000, 170000, 180000, 190000, 200000, 210000, 220000, 230000, 240000, 250000},
		PlanID1Price:             planID1Price,
		PlanID2Price:             planID2Price,
		PlanID3Price:             planID3Price,
		DevDirectoryPlanID1Price: devDirectoryPlanID1Price,
		DevDirectoryPlanID2Price: devDirectoryPlanID2Price,
		DevDirectoryPlanID3Price: devDirectoryPlanID3Price,
		DevelopersBannerLink:     developersBannerLink,
		DevelopersBannerText:     developersBannerText,
		URLProtocol:              urlProtocol,
		DevOfferLink1:            devOfferLink1,
		DevOfferLink2:            devOfferLink2,
		DevOfferLink3:            devOfferLink3,
		DevOfferLink4:            devOfferLink4,
		DevOfferRate1:            devOfferRate1,
		DevOfferRate2:            devOfferRate2,
		DevOfferRate3:            devOfferRate3,
		DevOfferRate4:            devOfferRate4,
		DevOfferCode1:            devOfferCode1,
		DevOfferCode2:            devOfferCode2,
		DevOfferCode3:            devOfferCode3,
		DevOfferCode4:            devOfferCode4,
	}, nil
}
