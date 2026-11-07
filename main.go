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

// appContext aggregates runtime repositories, configuration, and server state for route registration.
type appContext struct {
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

func mustReadStaticFile(path string) []byte {
	content, err := staticFS.ReadFile(path)
	if err != nil {
		log.Fatalf("unable to read %s placeholder file: %v", path, err)
	}
	return content
}

func initAppContext() appContext {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("unable to load config: %+v", err)
	}
	conn, err := database.GetDbConn(
		cfg.DatabaseUser, cfg.DatabasePassword, cfg.DatabaseHost,
		cfg.DatabasePort, cfg.DatabaseName, cfg.DatabaseSSLMode,
	)
	if err != nil {
		log.Fatalf("unable to connect to postgres: %v", err)
	}
	emailClient, err := email.NewClient(
		cfg.SmtpUser, cfg.SmtpPassword, cfg.SmtpHost,
		cfg.SupportEmail, cfg.NoReplyEmail, cfg.SiteName, cfg.Env == "dev",
	)
	if err != nil {
		log.Fatalf("unable to connect to sparkpost API: %v", err)
	}
	svr := server.NewServer(
		cfg, conn, mux.NewRouter(), template.NewTemplate(staticFS),
		emailClient, sessions.NewCookieStore(cfg.SessionKey),
	)
	return appContext{
		cfg:                cfg,
		svr:                svr,
		devRepo:            developer.NewRepository(conn),
		recRepo:            recruiter.NewRepository(conn),
		blogRepo:           blog.NewRepository(conn),
		userRepo:           user.NewRepository(conn),
		companyRepo:        company.NewRepository(conn),
		jobRepo:            job.NewRepository(conn),
		paymentRepo:        payment.NewRepository(cfg.StripeKey, cfg.SiteName, cfg.SiteHost, cfg.URLProtocol),
		bookmarkRepo:       bookmark.NewRepository(conn),
		robotsTxtContent:   mustReadStaticFile("static/robots.txt"),
		securityTxtContent: mustReadStaticFile("static/security.txt"),
		adsTxtContent:      mustReadStaticFile("static/ads.txt"),
	}
}

func registerCoreAndCompanyRoutes(ctx appContext) {
	svr := ctx.svr
	cat := strings.Title(ctx.cfg.SiteJobCategory)
	svr.RegisterRoute("/sitemap.xml", handler.SitemapIndexHandler(svr), []string{"GET"})
	svr.RegisterRoute("/sitemap-{number}.xml", handler.SitemapHandler(svr), []string{"GET"})
	svr.RegisterRoute("/robots.txt", handler.RobotsTXTHandler(svr, ctx.robotsTxtContent), []string{"GET"})
	svr.RegisterRoute("/ads.txt", handler.AdsTXTHandler(svr, ctx.adsTxtContent), []string{"GET"})
	svr.RegisterRoute("/.well-known/security.txt", handler.WellKnownSecurityHandler(svr, ctx.securityTxtContent), []string{"GET"})
	svr.RegisterPathPrefix("/s/", http.StripPrefix("/s/", http.FileServer(http.Dir("./static/assets"))), []string{"GET"})
	svr.RegisterRoute("/about", handler.AboutPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/privacy-policy", handler.PrivacyPolicyPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/terms-of-service", handler.TermsOfServicePageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/", handler.IndexPageHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Companies-Using-%s", cat), handler.CompaniesHandler(svr, ctx.companyRepo, ctx.jobRepo, ctx.devRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-Companies-Using-%s", cat), handler.CompaniesForLocationHandler(svr, ctx.companyRepo, ctx.jobRepo, ctx.devRepo, "Remote"), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Companies-Using-%s-In-{location}", cat), handler.CompaniesHandler(svr, ctx.companyRepo, ctx.jobRepo, ctx.devRepo), []string{"GET"})
}

func registerDeveloperDirectoryRoutes(ctx appContext) {
	svr := ctx.svr
	cat := strings.Title(ctx.cfg.SiteJobCategory)
	svr.RegisterRoute(fmt.Sprintf("/%s-Developers", cat), handler.DevelopersHandler(svr, ctx.devRepo, ctx.recRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Developers-In-{location}", cat), handler.DevelopersHandler(svr, ctx.devRepo, ctx.recRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{tag}-Developers", cat), handler.DevelopersHandler(svr, ctx.devRepo, ctx.recRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{tag}-Developers-In-{location}", cat), handler.DevelopersHandler(svr, ctx.devRepo, ctx.recRepo), []string{"GET"})
	svr.RegisterRoute("/Submit-Developer-Profile", handler.PermanentRedirectHandler(svr, fmt.Sprintf("/Join-%s-Community", cat)), []string{"GET"})
	svr.RegisterRoute("/community", handler.PermanentRedirectHandler(svr, fmt.Sprintf("/Join-%s-Community", cat)), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Join-%s-Community", cat), handler.SubmitDeveloperProfileHandler(svr, ctx.devRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Hire-From-%s-Community", cat), handler.SubmitRecruiterProfileHandler(svr, ctx.devRepo), []string{"GET"})
	svr.RegisterRoute("/x/srp", handler.SaveRecruiterProfileHandler(svr, ctx.recRepo, ctx.userRepo, ctx.paymentRepo), []string{"POST"})
	svr.RegisterRoute("/x/sdp", handler.SaveDeveloperProfileWithFaceCheckHandler(svr, ctx.devRepo), []string{"POST"})
	svr.RegisterRoute("/x/sdm", handler.SaveDeveloperMetadataHandler(svr, ctx.devRepo), []string{"POST"})
	svr.RegisterRoute("/x/udp", handler.UpdateDeveloperProfileHandler(svr, ctx.devRepo), []string{"POST"})
	svr.RegisterRoute("/x/udm", handler.UpdateDeveloperMetadataHandler(svr, ctx.devRepo), []string{"POST"})
	svr.RegisterRoute("/x/ddm", handler.DeleteDeveloperMetadataHandler(svr, ctx.devRepo), []string{"POST"})
	svr.RegisterRoute("/x/ddp", handler.DeleteDeveloperProfileHandler(svr, ctx.devRepo, ctx.userRepo), []string{"POST"})
	svr.RegisterRoute("/x/smdp/{id}", handler.SendMessageDeveloperProfileHandler(svr, ctx.devRepo), []string{"POST"})
	svr.RegisterRoute("/developer/{slug}", handler.ViewDeveloperProfileHandler(svr, ctx.devRepo, ctx.recRepo), []string{"GET"})
	svr.RegisterRoute("/x/auth/message/{id}", handler.DeliverMessageDeveloperProfileHandler(svr, ctx.devRepo), []string{"GET"})
}

func registerBlogAndProfileRoutes(ctx appContext) {
	svr := ctx.svr
	svr.RegisterRoute("/profile/home", handler.ProfileHomepageHandler(svr, ctx.devRepo, ctx.recRepo), []string{"GET"})
	svr.RegisterRoute("/profile/{id}/edit", handler.EditProfileHandler(svr, ctx.devRepo, ctx.recRepo), []string{"GET"})
	svr.RegisterRoute("/profile/blog/create", handler.CreateDraftBlogPostHandler(svr, ctx.blogRepo), []string{"GET"})
	svr.RegisterRoute("/profile/blog/list", handler.GetUserBlogPostsHandler(svr, ctx.blogRepo), []string{"GET"})
	svr.RegisterRoute("/profile/blog/{id}/edit", handler.EditBlogPostHandler(svr, ctx.blogRepo), []string{"GET"})
	svr.RegisterRoute("/x/profile/blog/create", handler.CreateBlogPostHandler(svr, ctx.blogRepo), []string{"POST"})
	svr.RegisterRoute("/x/profile/blog/{id}/publish", handler.PublishBlogPostHandler(svr, ctx.blogRepo), []string{"POST"})
	svr.RegisterRoute("/x/profile/blog/{id}/unpublish", handler.UnpublishBlogPostHandler(svr, ctx.blogRepo), []string{"POST"})
	svr.RegisterRoute("/x/profile/blog/{id}/update", handler.UpdateBlogPostHandler(svr, ctx.blogRepo), []string{"POST"})
	svr.RegisterRoute("/blog/{slug}", handler.GetBlogPostBySlugHandler(svr, ctx.blogRepo), []string{"GET"})
	svr.RegisterRoute("/blog", handler.GetAllPublishedBlogPostsHandler(svr, ctx.blogRepo), []string{"GET"})
	svr.RegisterRoute("/profile/jobs", handler.RecruiterJobPosts(svr, ctx.devRepo, ctx.recRepo, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/profile/sent", handler.SentMessages(svr, ctx.devRepo), []string{"GET"})
	svr.RegisterRoute("/profile/messages", handler.ReceivedMessages(svr, ctx.devRepo), []string{"GET"})
}

func registerTaskRoutes(ctx appContext) {
	svr := ctx.svr
	svr.RegisterRoute("/x/task/weekly-newsletter", handler.TriggerWeeklyNewsletter(svr, ctx.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/ads-manager", handler.TriggerAdsManager(svr, ctx.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/twitter-scheduler", handler.TriggerTwitterScheduler(svr, ctx.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/telegram-scheduler", handler.TriggerTelegramScheduler(svr, ctx.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/company-update", handler.TriggerCompanyUpdate(svr, ctx.companyRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/sitemap-update", handler.TriggerSitemapUpdate(svr, ctx.devRepo, ctx.jobRepo, ctx.blogRepo, ctx.companyRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/cloudflare-stats-export", handler.TriggerCloudflareStatsExport(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/expired-jobs", handler.TriggerExpiredJobsTask(svr, ctx.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/update-last-week-clickouts", handler.TriggerUpdateLastWeekClickouts(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/monthly-highlights", handler.TriggerMonthlyHighlights(svr, ctx.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/task/fx-rate-update", handler.TriggerFXRateUpdate(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/expire-sign-on-tokens", handler.TriggerExpiredUserSignOnTokensTask(svr, ctx.userRepo), []string{"POST"})
}

func registerJobAndMediaRoutes(ctx appContext) {
	svr := ctx.svr
	svr.RegisterRoute("/newsletter", handler.ViewNewsletterPageHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute("/support", handler.ViewSupportPageHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute("/x/j/p/1", handler.PostAJobSuccessPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/j/p/0", handler.PostAJobFailurePageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/email/subscribe", handler.AddEmailSubscriberHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/email/unsubscribe", handler.RemoveEmailSubscriberHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/email/confirm/{token}", handler.ConfirmEmailSubscriberHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/a/e", handler.ApplyForJobPageHandler(svr, ctx.jobRepo, ctx.bookmarkRepo), []string{"POST"})
	svr.RegisterRoute("/apply/{token}", handler.ApplyToJobConfirmation(svr, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/s", handler.SubmitJobPostPageHandler(svr, ctx.jobRepo, ctx.paymentRepo), []string{"POST"})
	svr.RegisterRoute("/x/s/upsell", handler.SubmitJobPostPaymentUpsellPageHandler(svr, ctx.jobRepo, ctx.paymentRepo), []string{"POST"})
	svr.RegisterRoute("/x/s/d/upsell", handler.DeveloperDirectoryUpsellPageHandler(svr, ctx.jobRepo, ctx.paymentRepo), []string{"POST"})
	svr.RegisterRoute("/x/s/m", handler.SaveMediaPageHandler(svr), []string{"POST"})
	svr.RegisterRoute("/x/s/m/{id}", handler.RetrieveMediaWithPrivacyBlurHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/s/m/meta/{id}", handler.RetrieveMediaMetaPageHandler(svr, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/stripe/checkout/completed", handler.StripePaymentConfirmationWebhookHandler(svr, ctx.jobRepo, ctx.recRepo), []string{"POST"})
	svr.RegisterRoute("/x/j/c/{id}", handler.TrackJobClickoutPageHandler(svr, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/r", handler.TrackJobClickoutAndRedirectToJobPage(svr, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/loc/autocomplete", handler.AutocompleteLocation(svr), []string{"GET"})
	svr.RegisterRoute("/x/skill/autocomplete", handler.AutocompleteSkill(svr), []string{"GET"})
	svr.RegisterRoute("/job/{slug}", handler.JobBySlugPageHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute("/company/{slug}", handler.CompanyBySlugPageHandler(svr, ctx.companyRepo, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/profile/bookmarks", handler.BookmarkListHandler(svr, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute("/x/bookmark", handler.BookmarkJobHandler(svr, ctx.bookmarkRepo, ctx.jobRepo), []string{"POST", "DELETE"})
}

func registerAuthAndPrivateRoutes(ctx appContext) {
	svr := ctx.svr
	svr.RegisterRoute("/auth", handler.GetAuthPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/auth/link", handler.RequestTokenSignOn(svr, ctx.userRepo, ctx.jobRepo, ctx.recRepo), []string{"POST"})
	svr.RegisterRoute("/x/auth/{token}", handler.VerifyTokenSignOn(svr, ctx.userRepo, ctx.devRepo, ctx.recRepo, ctx.cfg.AdminEmail), []string{"GET"})
	svr.RegisterRoute("/x/u", handler.UpdateJobPageHandler(svr, ctx.jobRepo), []string{"POST"})
	svr.RegisterRoute("/edit/{token}", handler.EditJobViewPageHandler(svr, ctx.jobRepo, ctx.recRepo), []string{"GET"})
	svr.RegisterRoute("/download-cv/{token}", handler.DownloadJobApplicationCvHandler(svr, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/d", handler.DisapproveJobPageHandler(svr, ctx.jobRepo), []string{"POST"})
}

func registerAliasAndRemoteLandingRoutes(ctx appContext) {
	svr := ctx.svr
	cfg := ctx.cfg
	cat := strings.Title(cfg.SiteJobCategory)
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs", cfg.SiteJobCategory), handler.PermanentRedirectHandler(svr, "/"), []string{"GET"})
	svr.RegisterRoute("/Remote-Jobs", handler.PermanentRedirectHandler(svr, fmt.Sprintf("/Remote-%s-Jobs", cat)), []string{"GET"})
	svr.RegisterRoute("/youtube", handler.PermanentExternalRedirectHandler(svr, fmt.Sprintf("https://www.youtube.com/c/%s", cfg.SiteYoutube)), []string{"GET"})
	svr.RegisterRoute("/telegram", handler.PermanentExternalRedirectHandler(svr, fmt.Sprintf("https://t.me/%s", cfg.SiteTelegramChannel)), []string{"GET"})
	svr.RegisterRoute("/twitter", handler.PermanentExternalRedirectHandler(svr, fmt.Sprintf("https://twitter.com/%s", cfg.SiteTwitter)), []string{"GET"})
	svr.RegisterRoute("/linkedin", handler.PermanentExternalRedirectHandler(svr, fmt.Sprintf("https://www.linkedin.com/company/%s", cfg.SiteLinkedin)), []string{"GET"})
	svr.RegisterRoute("/github", handler.PermanentExternalRedirectHandler(svr, fmt.Sprintf("https://github.com/%s", cfg.SiteGithub)), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-%s-Jobs", cat), handler.LandingPageForLocationHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo, "Remote"), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-%s-Jobs-Paying-{salary}-{currency}-year", cat), handler.LandingPageForLocationHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo, "Remote"), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs-Paying-{salary}-{currency}-year", cat), handler.IndexPageHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-%s-{skill}-Jobs", cat), handler.LandingPageForLocationAndSkillPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo, "Remote"), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-%s-{skill}-Jobs-Paying-{salary}-{currency}-year", cat), handler.LandingPageForLocationAndSkillPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo, "Remote"), []string{"GET"})
}

func registerLocationAndSkillLandingRoutes(ctx appContext) {
	svr := ctx.svr
	cat := strings.Title(ctx.cfg.SiteJobCategory)
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs-In-{location}-Paying-{salary}-{currency}-year", cat), handler.LandingPageForLocationPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs-in-{location}-Paying-{salary}-{currency}-year", cat), handler.LandingPageForLocationPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs-In-{location}", cat), handler.LandingPageForLocationPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Jobs-in-{location}", cat), handler.LandingPageForLocationPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs", cat), handler.LandingPageForSkillPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs-Paying-{salary}-{currency}-year", cat), handler.LandingPageForSkillPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs-In-{location}-Paying-{salary}-{currency}-year", cat), handler.LandingPageForSkillAndLocationPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs-in-{location}-Paying-{salary}-{currency}-year", cat), handler.LandingPageForSkillAndLocationPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs-In-{location}", cat), handler.LandingPageForSkillAndLocationPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-{skill}-Jobs-in-{location}", cat), handler.LandingPageForSkillAndLocationPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo, ctx.bookmarkRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/%s-Developer-Salary-{location}", cat), handler.SalaryLandingPageLocationPlaceholderHandler(svr, ctx.jobRepo, ctx.devRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Remote-%s-Developer-Salary", cat), handler.SalaryLandingPageLocationHandler(svr, ctx.jobRepo, ctx.devRepo, "Remote"), []string{"GET"})
}

func registerHireAndAdminRoutes(ctx appContext) {
	svr := ctx.svr
	cat := strings.Title(ctx.cfg.SiteJobCategory)
	svr.RegisterRoute(fmt.Sprintf("/Hire-%s-Developers", cat), handler.PostAJobPageHandler(svr, ctx.companyRepo, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/hire-%s-developers", ctx.cfg.SiteJobCategory), handler.PermanentRedirectHandler(svr, fmt.Sprintf("Hire-%s-Developers", cat)), []string{"GET"})
	svr.RegisterRoute("/post-a-job", handler.PermanentRedirectHandler(svr, fmt.Sprintf("Hire-%s-Developers", cat)), []string{"GET"})
	svr.RegisterRoute("/ad", handler.PermanentRedirectHandler(svr, fmt.Sprintf("Hire-%s-Developers", cat)), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Hire-Remote-%s-Developers", cat), handler.PostAJobForLocationPageHandler(svr, ctx.companyRepo, ctx.jobRepo, "Remote"), []string{"GET"})
	svr.RegisterRoute(fmt.Sprintf("/Hire-%s-Developers-In-{location}", cat), handler.PostAJobForLocationFromURLPageHandler(svr, ctx.companyRepo, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/payment", handler.ShowPaymentPage(svr), []string{"GET"})
	svr.RegisterRoute("/x/payment-intent", handler.GeneratePaymentIntent(svr, ctx.paymentRepo), []string{"POST"})
	svr.RegisterRoute("/rss", handler.ServeRSSFeed(svr, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/manage/new", handler.PostAJobWithoutPaymentPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/manage/list", handler.ListJobsAsAdminPageHandler(svr, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/manage/job/{slug}", handler.ManageJobBySlugViewPageHandler(svr, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/manage/{token}", handler.ManageJobViewPageHandler(svr, ctx.jobRepo), []string{"GET"})
	svr.RegisterRoute("/x/sp", handler.SubmitJobPostWithoutPaymentHandler(svr, ctx.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/a", handler.ApproveJobPageHandler(svr, ctx.jobRepo), []string{"POST"})
	svr.RegisterRoute("/x/j/d", handler.PermanentlyDeleteJobByToken(svr, ctx.jobRepo), []string{"POST"})
}

func main() {
	ctx := initAppContext()
	registerCoreAndCompanyRoutes(ctx)
	registerDeveloperDirectoryRoutes(ctx)
	registerBlogAndProfileRoutes(ctx)
	registerTaskRoutes(ctx)
	registerJobAndMediaRoutes(ctx)
	registerAuthAndPrivateRoutes(ctx)
	registerAliasAndRemoteLandingRoutes(ctx)
	registerLocationAndSkillLandingRoutes(ctx)
	registerHireAndAdminRoutes(ctx)
	log.Fatal(ctx.svr.Run())
}
