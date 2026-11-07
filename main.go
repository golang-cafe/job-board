package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"strings"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"

	"github.com/golang-cafe/job-board/internal/blog"
	"github.com/golang-cafe/job-board/internal/bookmark"
	"github.com/golang-cafe/job-board/internal/company"
	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/database"
	"github.com/golang-cafe/job-board/internal/developer"
	"github.com/golang-cafe/job-board/internal/email"
	"github.com/golang-cafe/job-board/internal/handler"
	"github.com/golang-cafe/job-board/internal/job"
	"github.com/golang-cafe/job-board/internal/payment"
	"github.com/golang-cafe/job-board/internal/recruiter"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/golang-cafe/job-board/internal/template"
	"github.com/golang-cafe/job-board/internal/user"
)

//go:embed static/*
var staticFS embed.FS

// appDependencies aggregates initialized repositories and HTTP server state for route registration.
type appDependencies struct {
	cfg                config.Config
	svr                server.Server
	devRepo            *developer.Repository
	recRepo            *recruiter.Repository
	blogRepo           *blog.Repository
	userRepo           *user.Repository
	companyRepo        *company.Repository
	jobRepo            *job.Repository
	paymentRepo        *payment.Repository
	bookmarkRepo       *bookmark.Repository
	robotsTxtContent   []byte
	securityTxtContent []byte
	adsTxtContent      []byte
}

func main() {
	app := bootstrapApplication()
	registerStaticAndInfoRoutes(app)
	registerCompanyAndDeveloperRoutes(app)
	registerBlogAndProfileRoutes(app)
	registerTaskRoutes(app)
	registerJobAndMediaRoutes(app)
	registerAuthAndPrivateRoutes(app)
	registerSocialAndRemoteLandingRoutes(app)
	registerLocationAndSkillLandingRoutes(app)
	registerSalaryHireAndAdminRoutes(app)
	log.Fatal(app.svr.Run())
}

func bootstrapApplication() *appDependencies {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("unable to load config: %+v", err)
	}
	conn, err := database.GetDbConn(
		cfg.DatabaseUser,
		cfg.DatabasePassword,
		cfg.DatabaseHost,
		cfg.DatabasePort,
		cfg.DatabaseName,
		cfg.DatabaseSSLMode,
	)
	if err != nil {
		log.Fatalf("unable to connect to postgres: %v", err)
	}
	emailClient, err := email.NewClient(
		cfg.SmtpUser,
		cfg.SmtpPassword,
		cfg.SmtpHost,
		cfg.SupportEmail,
		cfg.NoReplyEmail,
		cfg.SiteName,
		cfg.Env == "dev",
	)
	if err != nil {
		log.Fatalf("unable to connect to sparkpost API: %v", err)
	}
	return buildAppDependencies(cfg, server.NewServer(
		cfg,
		conn,
		mux.NewRouter(),
		template.NewTemplate(staticFS),
		emailClient,
		sessions.NewCookieStore(cfg.SessionKey),
	))
}

func buildAppDependencies(cfg config.Config, svr server.Server) *appDependencies {
	robotsTxt, err := staticFS.ReadFile("static/robots.txt")
	if err != nil {
		log.Fatalf("unable to read robots.txt placeholder file: %w", err)
	}
	securityTxt, err := staticFS.ReadFile("static/security.txt")
	if err != nil {
		log.Fatalf("unable to read security.txt placeholder file: %w", err)
	}
	adsTxt, err := staticFS.ReadFile("static/ads.txt")
	if err != nil {
		log.Fatalf("unable to read security.txt placeholder file: %w", err)
	}
	return &appDependencies{
		cfg:                cfg,
		svr:                svr,
		devRepo:            developer.NewRepository(svr.Conn),
		recRepo:            recruiter.NewRepository(svr.Conn),
		blogRepo:           blog.NewRepository(svr.Conn),
		userRepo:           user.NewRepository(svr.Conn),
		companyRepo:        company.NewRepository(svr.Conn),
		jobRepo:            job.NewRepository(svr.Conn),
		paymentRepo:        payment.NewRepository(cfg.StripeKey, cfg.SiteName, cfg.SiteHost, cfg.URLProtocol),
		bookmarkRepo:       bookmark.NewRepository(svr.Conn),
		robotsTxtContent:   robotsTxt,
		securityTxtContent: securityTxt,
		adsTxtContent:      adsTxt,
	}
}

func registerStaticAndInfoRoutes(app *appDependencies) {
	svr := app.svr
	svr.RegisterRoute("/sitemap.xml", handler.SitemapIndexHandler(svr), []string{"GET"})
	svr.RegisterRoute("/sitemap-{number}.xml", handler.SitemapHandler(svr), []string{"GET"})
	svr.RegisterRoute("/robots.txt", handler.RobotsTXTHandler(svr, app.robotsTxtContent), []string{"GET"})
	svr.RegisterRoute("/ads.txt", handler.AdsTXTHandler(svr, app.adsTxtContent), []string{"GET"})
	svr.RegisterRoute("/.well-known/security.txt", handler.WellKnownSecurityHandler(svr, app.securityTxtContent), []string{"GET"})
	svr.RegisterPathPrefix("/s/", http.StripPrefix("/s/", http.FileServer(http.Dir("./static/assets"))), []string{"GET"})
	svr.RegisterRoute("/about", handler.AboutPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/privacy-policy", handler.PrivacyPolicyPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/terms-of-service", handler.TermsOfServicePageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/", handler.IndexPageHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo), []string{"GET"})
}

func registerCompanyAndDeveloperRoutes(app *appDependencies) {
	svr := app.svr
	cat := strings.Title(app.cfg.SiteJobCategory)
	svr.RegisterRoute(fmt.Sprintf("/Companies-Using-%s", cat), handler.CompaniesHandler(svr, app.companyRepo, app.jobRepo, app.devRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-Companies-Using-%s", cat), handler.CompaniesForLocationHandler(svr, app.companyRepo, app.jobRepo, app.devRepo, "Remote"), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Companies-Using-%s-In-{location}", cat), handler.CompaniesHandler(svr, app.companyRepo, app.jobRepo, app.devRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Developers", cat), handler.DevelopersHandler(svr, app.devRepo, app.recRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Developers-In-{location}", cat), handler.DevelopersHandler(svr, app.devRepo, app.recRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{tag}-Developers", cat), handler.DevelopersHandler(svr, app.devRepo, app.recRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{tag}-Developers-In-{location}", cat), handler.DevelopersHandler(svr, app.devRepo, app.recRepo), []string{"GET"})
	svr.RegisterRoute("/Submit-Developer-Profile", handler.PermanentRedirectHandler(svr, fmt.Sprintf("/Join-%s-Community", cat)), []string{"GET"})
	svr.RegisterRoute("/community", handler.PermanentRedirectHandler(svr, fmt.Sprintf("/Join-%s-Community", cat)), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Join-%s-Community", cat), handler.SubmitDeveloperProfileHandler(svr, app.devRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Hire-From-%s-Community", cat), handler.SubmitRecruiterProfileHandler(svr, app.devRepo), []string{"GET"})
	svr.RegisterRoute("/x/srp", handler.SaveRecruiterProfileHandler(svr, app.recRepo, app.userRepo, app.paymentRepo), []string{"POST"})
	svr.RegisterRoute("/x/sdp", handler.SaveDeveloperProfileHandler(svr, app.devRepo, app.userRepo), []string{"POST"})
	svr.RegisterRoute("/x/sdm", handler.SaveDeveloperMetadataHandler(svr, app.devRepo), []string{"POST"})
	svr.RegisterRoute("/x/udp", handler.UpdateDeveloperProfileHandler(svr, app.devRepo), []string{"POST"})
	svr.RegisterRoute("/x/udm", handler.UpdateDeveloperMetadataHandler(svr, app.devRepo), []string{"POST"})
	svr.RegisterRoute("/x/ddm", handler.DeleteDeveloperMetadataHandler(svr, app.devRepo), []string{"POST"})
	svr.RegisterRoute("/x/ddp", handler.DeleteDeveloperProfileHandler(svr, app.devRepo, app.userRepo), []string{"POST"})
	svr.RegisterRoute("/x/smdp/{id}", handler.SendMessageDeveloperProfileHandler(svr, app.devRepo), []string{"POST"})
	svr.RegisterRoute("/developer/{slug}", handler.ViewDeveloperProfileHandler(svr, app.devRepo, app.recRepo), []string{"GET"})
	svr.RegisterRoute("/x/auth/message/{id}", handler.DeliverMessageDeveloperProfileHandler(svr, app.devRepo), []string{"GET"})
}

func registerBlogAndProfileRoutes(app *appDependencies) {
	svr := app.svr
	svr.RegisterRoute("/profile/home", handler.ProfileHomepageHandler(svr, app.devRepo, app.recRepo), []string{"GET"})
	svr.RegisterRoute("/profile/{id}/edit", handler.EditProfileHandler(svr, app.devRepo, app.recRepo), []string{"GET"})
	svr.RegisterRoute("/profile/blog/create", handler.CreateDraftBlogPostHandler(svr, app.blogRepo), []string{"GET"})
	svr.RegisterRoute("/profile/blog/list", handler.GetUserBlogPostsHandler(svr, app.blogRepo), []string{"GET"})
	svr.RegisterRoute("/profile/blog/{id}/edit", handler.EditBlogPostHandler(svr, app.blogRepo), []string{"GET"})
	svr.RegisterRoute("/x/profile/blog/create", handler.CreateBlogPostHandler(svr, app.blogRepo), []string{"POST"})
	svr.RegisterRoute("/x/profile/blog/{id}/publish", handler.PublishBlogPostHandler(svr, app.blogRepo), []string{"POST"})
	svr.RegisterRoute("/x/profile/blog/{id}/unpublish", handler.UnpublishBlogPostHandler(svr, app.blogRepo), []string{"POST"})
	svr.RegisterRoute("/x/profile/blog/{id}/update", handler.UpdateBlogPostHandler(svr, app.blogRepo), []string{"POST"})
	svr.RegisterRoute("/blog/{slug}", handler.GetBlogPostBySlugHandler(svr, app.blogRepo), []string{"GET"})
	svr.RegisterRoute("/blog", handler.GetAllPublishedBlogPostsHandler(svr, app.blogRepo), []string{"GET"})
	svr.RegisterRoute("/profile/jobs", handler.RecruiterJobPosts(svr, app.devRepo, app.recRepo, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/profile/sent", handler.SentMessages(svr, app.devRepo), []string{"GET"})
	svr.RegisterRoute("/profile/messages", handler.ReceivedMessages(svr, app.devRepo), []string{"GET"})
}

func registerTaskRoutes(app *appDependencies) {
	svr := app.svr
	svr.RegisterRoute("/x/task/weekly-newsletter", handler.TriggerWeeklyNewsletter(svr, app.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/ads-manager", handler.TriggerAdsManager(svr, app.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/twitter-scheduler", handler.TriggerTwitterScheduler(svr, app.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/telegram-scheduler", handler.TriggerTelegramScheduler(svr, app.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/linkedin-share-jobs", handler.TriggerLinkedInShareJobs(svr, app.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/company-update", handler.TriggerCompanyUpdate(svr, app.companyRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/sitemap-update", handler.TriggerSitemapUpdate(svr, app.devRepo, app.jobRepo, app.blogRepo, app.companyRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/cloudflare-stats-export", handler.TriggerCloudflareStatsExport(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/expired-jobs", handler.TriggerExpiredJobsTask(svr, app.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/update-last-week-clickouts", handler.TriggerUpdateLastWeekClickouts(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/monthly-highlights", handler.TriggerMonthlyHighlights(svr, app.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/fx-rate-update", handler.TriggerFXRateUpdate(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/expire-sign-on-tokens", handler.TriggerExpiredUserSignOnTokensTask(svr, app.userRepo), []string{"POST"})
}

func registerJobAndMediaRoutes(app *appDependencies) {
	svr := app.svr
	svr.RegisterRoute("/newsletter", handler.ViewNewsletterPageHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute("/support", handler.ViewSupportPageHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute("/x/j/p/1", handler.PostAJobSuccessPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/j/p/0", handler.PostAJobFailurePageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/email/subscribe", handler.AddEmailSubscriberHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/email/unsubscribe", handler.RemoveEmailSubscriberHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/email/confirm/{token}", handler.ConfirmEmailSubscriberHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/a/e", handler.ApplyForJobPageHandler(svr, app.jobRepo, app.bookmarkRepo), []string{"POST"})
	svr.RegisterRoute("/apply/{token}", handler.ApplyToJobConfirmation(svr, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/s", handler.SubmitJobPostPageHandler(svr, app.jobRepo, app.paymentRepo), []string{"POST"})
	svr.RegisterRoute("/x/s/upsell", handler.SubmitJobPostPaymentUpsellPageHandler(svr, app.jobRepo, app.paymentRepo), []string{"POST"})
	svr.RegisterRoute("/x/s/d/upsell", handler.DeveloperDirectoryUpsellPageHandler(svr, app.jobRepo, app.paymentRepo), []string{"POST"})
	svr.RegisterRoute("/x/s/m", handler.SaveMediaPageHandler(svr), []string{"POST"})
	svr.RegisterRoute("/x/s/m/{id}", handler.RetrieveMediaPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/s/m/meta/{id}", handler.RetrieveMediaMetaPageHandler(svr, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/stripe/checkout/completed", handler.StripePaymentConfirmationWebhookHandler(svr, app.jobRepo, app.recRepo), []string{"POST"})
	svr.RegisterRoute("/x/j/c/{id}", handler.TrackJobClickoutPageHandler(svr, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/r", handler.TrackJobClickoutAndRedirectToJobPage(svr, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/loc/autocomplete", handler.AutocompleteLocation(svr), []string{"GET"})
	svr.RegisterRoute("/x/skill/autocomplete", handler.AutocompleteSkill(svr), []string{"GET"})
	svr.RegisterRoute("/job/{slug}", handler.JobBySlugPageHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute("/company/{slug}", handler.CompanyBySlugPageHandler(svr, app.companyRepo, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/profile/bookmarks", handler.BookmarkListHandler(svr, app.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute("/x/bookmark", handler.BookmarkJobHandler(svr, app.bookmarkRepo, app.jobRepo), []string{"POST", "DELETE"})
}

func registerAuthAndPrivateRoutes(app *appDependencies) {
	svr := app.svr
	svr.RegisterRoute("/auth", handler.GetAuthPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/auth/link", handler.RequestTokenSignOn(svr, app.userRepo, app.jobRepo, app.recRepo), []string{"POST"})
	svr.RegisterRoute("/x/auth/{token}", handler.VerifyTokenSignOn(svr, app.userRepo, app.devRepo, app.recRepo, app.cfg.AdminEmail), []string{"GET"})
	svr.RegisterRoute("/x/u", handler.UpdateJobPageHandler(svr, app.jobRepo), []string{"POST"})
	svr.RegisterRoute("/edit/{token}", handler.EditJobViewPageHandler(svr, app.jobRepo, app.recRepo), []string{"GET"})
	svr.RegisterRoute("/download-cv/{token}", handler.DownloadJobApplicationCvHandler(svr, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/d", handler.DisapproveJobPageHandler(svr, app.jobRepo), []string{"POST"})
}

func registerSocialAndRemoteLandingRoutes(app *appDependencies) {
	svr := app.svr
	cfg := app.cfg
	cat := strings.Title(cfg.SiteJobCategory)
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs", cfg.SiteJobCategory), handler.PermanentRedirectHandler(svr, "/"), []string{"GET"})
	svr.RegisterRoute("/Remote-Jobs", handler.PermanentRedirectHandler(svr, fmt.Sprintf("/Remote-%s-Jobs", cat)), []string{"GET"})
	svr.RegisterRoute("/youtube", handler.PermanentExternalRedirectHandler(svr, fmt.Sprintf("https://www.youtube.com/c/%s", cfg.SiteYoutube)), []string{"GET"})
	svr.RegisterRoute("/telegram", handler.PermanentExternalRedirectHandler(svr, fmt.Sprintf("https://t.me/%s", cfg.SiteTelegramChannel)), []string{"GET"})
	svr.RegisterRoute("/twitter", handler.PermanentExternalRedirectHandler(svr, fmt.Sprintf("https://twitter.com/%s", cfg.SiteTwitter)), []string{"GET"})
	svr.RegisterRoute("/linkedin", handler.PermanentExternalRedirectHandler(svr, fmt.Sprintf("https://www.linkedin.com/company/%s", cfg.SiteLinkedin)), []string{"GET"})
	svr.RegisterRoute("/github", handler.PermanentExternalRedirectHandler(svr, fmt.Sprintf("https://github.com/%s", cfg.SiteGithub)), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-%s-Jobs", cat), handler.LandingPageForLocationHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo, "Remote"), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-%s-Jobs-Paying-{salary}-{currency}-year", cat), handler.LandingPageForLocationHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo, "Remote"), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs-Paying-{salary}-{currency}-year", cat), handler.IndexPageHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-%s-{skill}-Jobs", cat), handler.LandingPageForLocationAndSkillPlaceholderHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo, "Remote"), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-%s-{skill}-Jobs-Paying-{salary}-{currency}-year", cat), handler.LandingPageForLocationAndSkillPlaceholderHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo, "Remote"), []string{"GET"})
}

func registerLocationAndSkillLandingRoutes(app *appDependencies) {
	svr := app.svr
	cat := strings.Title(app.cfg.SiteJobCategory)
	locHandler := handler.LandingPageForLocationPlaceholderHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo)
	skillHandler := handler.LandingPageForSkillPlaceholderHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo)
	bothHandler := handler.LandingPageForSkillAndLocationPlaceholderHandler(svr, app.jobRepo, app.devRepo, app.bookmarkRepo)
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs-In-{location}-Paying-{salary}-{currency}-year", cat), locHandler, []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs-in-{location}-Paying-{salary}-{currency}-year", cat), locHandler, []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs-In-{location}", cat), locHandler, []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs-in-{location}", cat), locHandler, []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs", cat), skillHandler, []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs-Paying-{salary}-{currency}-year", cat), skillHandler, []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs-In-{location}-Paying-{salary}-{currency}-year", cat), bothHandler, []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs-in-{location}-Paying-{salary}-{currency}-year", cat), bothHandler, []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs-In-{location}", cat), bothHandler, []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs-in-{location}", cat), bothHandler, []string{"GET"})
}

func registerSalaryHireAndAdminRoutes(app *appDependencies) {
	svr := app.svr
	cat := strings.Title(app.cfg.SiteJobCategory)
	svr.RegisterRoute(fmt.Sprintf("/%s-Developer-Salary-{location}", cat), handler.SalaryLandingPageLocationPlaceholderHandler(svr, app.jobRepo, app.devRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-%s-Developer-Salary", cat), handler.SalaryLandingPageLocationHandler(svr, app.jobRepo, app.devRepo, "Remote"), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Hire-%s-Developers", cat), handler.PostAJobPageHandler(svr, app.companyRepo, app.jobRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/hire-%s-developers", app.cfg.SiteJobCategory), handler.PermanentRedirectHandler(svr, fmt.Sprintf("Hire-%s-Developers", cat)), []string{"GET"})
	svr.RegisterRoute("/post-a-job", handler.PermanentRedirectHandler(svr, fmt.Sprintf("Hire-%s-Developers", cat)), []string{"GET"})
	svr.RegisterRoute("/ad", handler.PermanentRedirectHandler(svr, fmt.Sprintf("Hire-%s-Developers", cat)), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Hire-Remote-%s-Developers", cat), handler.PostAJobForLocationPageHandler(svr, app.companyRepo, app.jobRepo, "Remote"), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Hire-%s-Developers-In-{location}", cat), handler.PostAJobForLocationFromURLPageHandler(svr, app.companyRepo, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/payment", handler.ShowPaymentPage(svr), []string{"GET"})
	svr.RegisterRoute("/x/payment-intent", handler.GeneratePaymentIntent(svr, app.paymentRepo), []string{"POST"})
	svr.RegisterRoute("/rss", handler.ServeRSSFeed(svr, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/manage/new", handler.PostAJobWithoutPaymentPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/manage/list", handler.ListJobsAsAdminPageHandler(svr, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/manage/job/{slug}", handler.ManageJobBySlugViewPageHandler(svr, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/manage/{token}", handler.ManageJobViewPageHandler(svr, app.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/sp", handler.SubmitJobPostWithoutPaymentHandler(svr, app.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/a", handler.ApproveJobPageHandler(svr, app.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/j/d", handler.PermanentlyDeleteJobByToken(svr, app.jobRepo), []string{"POST"})
}
