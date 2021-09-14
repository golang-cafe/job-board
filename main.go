package main

import (
	"log"
	"net/http"

	"github.com/0x13a/golang.cafe/pkg/config"
	"github.com/0x13a/golang.cafe/pkg/database"
	"github.com/0x13a/golang.cafe/pkg/email"
	"github.com/0x13a/golang.cafe/pkg/handler"
	"github.com/0x13a/golang.cafe/pkg/ipgeolocation"
	"github.com/0x13a/golang.cafe/pkg/server"
	"github.com/0x13a/golang.cafe/pkg/template"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("unable to load config: %+v", err)
	}
	conn, err := database.GetDbConn(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("unable to connect to postgres: %v", err)
	}
	emailClient, err := email.NewClient(cfg.EmailAPIKey)
	if err != nil {
		log.Fatalf("unable to connect to sparkpost API: %v", err)
	}
	ipGeolocation, err := ipgeolocation.NewIPGeoLocation(cfg.IPGeoLocationGeoliteFile, cfg.IPGeoLocationCurrencyMapFile)
	if err != nil {
		log.Fatalf("unable to load ipgeolocation")
	}
	defer ipGeolocation.Close()
	sessionStore := sessions.NewCookieStore(cfg.SessionKey)

	svr := server.NewServer(
		cfg,
		conn,
		mux.NewRouter(),
		template.NewTemplate(),
		emailClient,
		ipGeolocation,
		sessionStore,
	)

	svr.RegisterRoute("/sitemap.xml", handler.SitemapIndexHandler(svr), []string{"GET"})
	svr.RegisterRoute("/sitemap-{number}.xml", handler.SitemapHandler(svr), []string{"GET"})
	svr.RegisterRoute("/robots.txt", handler.RobotsTxtHandler, []string{"GET"})
	svr.RegisterRoute("/.well-known/security.txt", handler.WellKnownSecurityHandler, []string{"GET"})

	svr.RegisterPathPrefix("/s/", http.StripPrefix("/s/", http.FileServer(http.Dir("./static/assets"))), []string{"GET"})
	svr.RegisterPathPrefix("/blog/", http.StripPrefix("/blog/", handler.DisableDirListing(http.FileServer(http.Dir("./static/blog")))), []string{"GET"})
	svr.RegisterPathPrefix("/blog", handler.BlogListHandler(svr, "./static/blog"), []string{"GET"})
	svr.RegisterPathPrefix("/changelog/", http.StripPrefix("/changelog/", http.FileServer(http.Dir("./static/changelog"))), []string{"GET"})

	svr.RegisterRoute("/about", handler.AboutPageHandler, []string{"GET"})
	svr.RegisterRoute("/privacy-policy", handler.PrivacyPolicyPageHandler, []string{"GET"})
	svr.RegisterRoute("/terms-of-service", handler.TermsOfServicePageHandler, []string{"GET"})

	svr.RegisterRoute("/", handler.IndexPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/Companies-Using-Golang", handler.CompaniesHandler(svr), []string{"GET"})
	svr.RegisterRoute("/Remote-Companies-Using-Golang", handler.CompaniesForLocationHandler(svr, "Remote"), []string{"GET"})
	svr.RegisterRoute("/Companies-Using-Golang-In-{location}", handler.CompaniesHandler(svr), []string{"GET"})

	// developers pages
	svr.RegisterRoute("/Golang-Developers", handler.DevelopersHandler(svr), []string{"GET"})
	svr.RegisterRoute("/Golang-Developers-In-{location}", handler.DevelopersHandler(svr), []string{"GET"})
	svr.RegisterRoute("/Golang-{tag}-Developers", handler.DevelopersHandler(svr), []string{"GET"})
	svr.RegisterRoute("/Golang-{tag}-Developers-In-{location}", handler.DevelopersHandler(svr), []string{"GET"})
	svr.RegisterRoute("/Submit-Developer-Profile", handler.SubmitDeveloperProfileHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/sdp", handler.SaveDeveloperProfileHandler(svr), []string{"POST"})
	svr.RegisterRoute("/x/udp", handler.UpdateDeveloperProfileHandler(svr), []string{"POST"})
	svr.RegisterRoute("/x/ddp", handler.DeleteDeveloperProfileHandler(svr), []string{"POST"})
	svr.RegisterRoute("/x/smdp/{id}", handler.SendMessageDeveloperProfileHandler(svr), []string{"POST"})
	svr.RegisterRoute("/edit/profile/{id}", handler.EditDeveloperProfileHandler(svr), []string{"GET"})
	svr.RegisterRoute("/developer/{slug}", handler.ViewDeveloperProfileHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/auth/message/{id}", handler.DeliverMessageDeveloperProfileHandler(svr), []string{"GET"})

	// tasks
	svr.RegisterRoute("/x/task/weekly-newsletter", handler.TriggerWeeklyNewsletter(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/ads-manager", handler.TriggerAdsManager(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/twitter-scheduler", handler.TriggerTwitterScheduler(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/telegram-scheduler", handler.TriggerTelegramScheduler(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/company-update", handler.TriggerCompanyUpdate(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/sitemap-update", handler.TriggerSitemapUpdate(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/cloudflare-stats-export", handler.TriggerCloudflareStatsExport(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/expired-jobs", handler.TriggerExpiredJobsTask(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/update-last-week-clickouts", handler.TriggerUpdateLastWeekClickouts(svr), []string{"POST"})
	svr.RegisterRoute("/x/task/monthly-highlights", handler.TriggerMonthlyHighlights(svr), []string{"POST"})

	// view newsletter
	svr.RegisterRoute("/newsletter", handler.ViewNewsletterPageHandler(svr), []string{"GET"})

	// view support
	svr.RegisterRoute("/support", handler.ViewSupportPageHandler(svr), []string{"GET"})

	// generate ksuid
	svr.RegisterRoute("/ksuid", handler.GenerateKsuIDPageHandler(svr), []string{"GET"})

	// IP address Lookup
	svr.RegisterRoute("/whats-my-ip", handler.IPAddressLookup(svr), []string{"GET"})

	// DNS Checker
	svr.RegisterRoute("/dns-checker", handler.DNSCheckerPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/dns", handler.DNSChecker(svr), []string{"GET"})

	// post a job succeeded
	svr.RegisterRoute("/x/j/p/1", handler.PostAJobSuccessPageHandler(svr), []string{"GET"})

	// post a job failed
	svr.RegisterRoute("/x/j/p/0", handler.PostAJobFailurePageHandler(svr), []string{"GET"})

	// newsletter member save
	svr.RegisterRoute("/x/email/subscribe", handler.AddEmailSubscriberHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/email/unsubscribe", handler.RemoveEmailSubscriberHandler(svr), []string{"GET"})
	svr.RegisterRoute("/x/email/confirm/{token}", handler.ConfirmEmailSubscriberHandler(svr), []string{"GET"})

	// apply for job
	svr.RegisterRoute("/x/a/e", handler.ApplyForJobPageHandler(svr), []string{"POST"})

	// apply to job confirmation
	svr.RegisterRoute("/apply/{token}", handler.ApplyToJobConfirmation(svr), []string{"GET"})

	// submit job post
	svr.RegisterRoute("/x/s", handler.SubmitJobPostPageHandler(svr), []string{"POST"})

	// re-submit job post payment for upsell
	svr.RegisterRoute("/x/s/upsell", handler.SubmitJobPostPaymentUpsellPageHandler(svr), []string{"POST"})

	// save media file
	svr.RegisterRoute("/x/s/m", handler.SaveMediaPageHandler(svr), []string{"POST"})

	// update media file
	svr.RegisterRoute("/x/s/m/{id}", handler.UpdateMediaPageHandler(svr), []string{"PUT"})

	// retrieve media file
	svr.RegisterRoute("/x/s/m/{id}", handler.RetrieveMediaPageHandler(svr), []string{"GET"})

	// retrieve meta image media file
	svr.RegisterRoute("/x/s/m/meta/{id}", handler.RetrieveMediaMetaPageHandler(svr), []string{"GET"})

	// stripe payment confirmation webhook
	svr.RegisterRoute("/x/stripe/checkout/completed", handler.StripePaymentConfirmationWebookHandler(svr), []string{"POST"})

	// send feedback message
	svr.RegisterRoute("/x/s/message", handler.SendFeedbackMessage(svr), []string{"POST"})

	// track job clickout
	svr.RegisterRoute("/x/j/c/{id}", handler.TrackJobClickoutPageHandler(svr), []string{"GET"})

	// track job clickout + redirect to job page
	svr.RegisterRoute("/x/r", handler.TrackJobClickoutAndRedirectToJobPage(svr), []string{"GET"})

	// autocomplete locations
	svr.RegisterRoute("/x/loc/autocomplete", handler.AutocompleteLocation(svr), []string{"GET"})

	// autocomplete skills
	svr.RegisterRoute("/x/skill/autocomplete", handler.AutocompleteSkill(svr), []string{"GET"})

	// view job by slug
	svr.RegisterRoute("/job/{slug}", handler.JobBySlugPageHandler(svr), []string{"GET"})

	// view company by slug
	svr.RegisterRoute("/company/{slug}", handler.CompanyBySlugPageHandler(svr), []string{"GET"})

	//
	// auth routes
	//

	// sign on page
	svr.RegisterRoute("/auth", handler.GetAuthPageHandler(svr), []string{"GET"})

	// sign on email link
	svr.RegisterRoute("/x/auth/link", handler.RequestTokenSignOn(svr), []string{"POST"})
	svr.RegisterRoute("/x/auth/{token}", handler.VerifyTokenSignOn(svr, cfg.AdminEmail), []string{"GET"})

	//
	// private routes
	// at the moment only protected by static token
	//

	// @private: update job by token
	svr.RegisterRoute("/x/u", handler.UpdateJobPageHandler(svr), []string{"POST"})

	// @private: view edit job by token
	svr.RegisterRoute("/edit/{token}", handler.EditJobViewPageHandler(svr), []string{"GET"})

	// @private: disapprove job by token
	svr.RegisterRoute("/x/d", handler.DisapproveJobPageHandler(svr), []string{"POST"})

	//
	// landing page routes
	//

	// Aliases
	svr.RegisterRoute("/Golang-Jobs", handler.PermanentRedirectHandler(svr, "/"), []string{"GET"})
	svr.RegisterRoute("/Remote-Jobs", handler.PermanentRedirectHandler(svr, "/Remote-Golang-Jobs"), []string{"GET"})
	svr.RegisterRoute("/youtube", handler.PermanentExternalRedirectHandler(svr, "https://www.youtube.com/c/GolangCafe"), []string{"GET"})
	svr.RegisterRoute("/telegram", handler.PermanentExternalRedirectHandler(svr, "https://t.me/golangcafe"), []string{"GET"})
	svr.RegisterRoute("/twitter", handler.PermanentExternalRedirectHandler(svr, "https://twitter.com/GolangCafe"), []string{"GET"})
	svr.RegisterRoute("/linkedin", handler.PermanentExternalRedirectHandler(svr, "https://www.linkedin.com/company/15868466"), []string{"GET"})
	svr.RegisterRoute("/github", handler.PermanentExternalRedirectHandler(svr, "https://github.com/golang-cafe/golang.cafe"), []string{"GET"})

	// Remote Landing Page No Skill
	svr.RegisterRoute("/Remote-Golang-Jobs", handler.LandingPageForLocationHandler(svr, "Remote"), []string{"GET"})

	// Remote Landing Page Skill
	svr.RegisterRoute("/Remote-Golang-{skill}-Jobs", handler.LandingPageForLocationAndSkillPlaceholderHandler(svr, "Remote"), []string{"GET"})

	// Location Only Landing Page
	svr.RegisterRoute("/Golang-Jobs-In-{location}", handler.LandingPageForLocationPlaceholderHandler(svr), []string{"GET"})
	svr.RegisterRoute("/Golang-Jobs-in-{location}", handler.LandingPageForLocationPlaceholderHandler(svr), []string{"GET"})

	// Skill Only Landing Page
	svr.RegisterRoute("/Golang-{skill}-Jobs", handler.LandingPageForSkillPlaceholderHandler(svr), []string{"GET"})

	// Skill And Location Landing Page
	svr.RegisterRoute("/Golang-{skill}-Jobs-In-{location}", handler.LandingPageForSkillAndLocationPlaceholderHandler(svr), []string{"GET"})
	svr.RegisterRoute("/Golang-{skill}-Jobs-in-{location}", handler.LandingPageForSkillAndLocationPlaceholderHandler(svr), []string{"GET"})

	// Golang Salary for location
	svr.RegisterRoute("/Golang-Developer-Salary-{location}", handler.SalaryLandingPageLocationPlaceholderHandler(svr), []string{"GET"})
	// Golang Salary for remote
	svr.RegisterRoute("/Remote-Golang-Developer-Salary", handler.SalaryLandingPageLocationHandler(svr, "Remote"), []string{"GET"})

	// hire golang developers pages
	svr.RegisterRoute("/Hire-Golang-Developers", handler.PostAJobPageHandler(svr), []string{"GET"})
	svr.RegisterRoute("/hire-golang-developers", handler.PermanentRedirectHandler(svr, "Hire-Golang-Developers"), []string{"GET"})
	svr.RegisterRoute("/post-a-job", handler.PermanentRedirectHandler(svr, "Hire-Golang-Developers"), []string{"GET"})
	svr.RegisterRoute("/ad", handler.PermanentRedirectHandler(svr, "Hire-Golang-Developers"), []string{"GET"})
	svr.RegisterRoute("/Hire-Remote-Golang-Developers", handler.PostAJobForLocationPageHandler(svr, "Remote"), []string{"GET"})
	svr.RegisterRoute("/Hire-Golang-Developers-In-{location}", handler.PostAJobForLocationFromURLPageHandler(svr), []string{"GET"})

	// generic payment intent view
	svr.RegisterRoute("/x/payment", handler.ShowPaymentPage(svr), []string{"GET"})
	// generic payment intent processing
	svr.RegisterRoute("/x/payment-intent", handler.GeneratePaymentIntent(svr), []string{"POST"})

	// RSS feed
	svr.RegisterRoute("/rss", handler.ServeRSSFeed(svr), []string{"GET"})

	//
	// admin routes
	// protected by jwt auth
	//

	// @admin: submit job without payment view
	svr.RegisterRoute("/manage/new", handler.PostAJobWithoutPaymentPageHandler(svr), []string{"GET"})

	// @admin: list/search jobs as admin
	svr.RegisterRoute("/manage/list", handler.ListJobsAsAdminPageHandler(svr), []string{"GET"})

	// @admin: view job as admin (alias to manage/edit/{token})
	svr.RegisterRoute("/manage/job/{slug}", handler.ManageJobBySlugViewPageHandler(svr), []string{"GET"})

	// @admin: view manage job page
	svr.RegisterRoute("/manage/{token}", handler.ManageJobViewPageHandler(svr), []string{"GET"})

	// @admin: submit job without payment
	svr.RegisterRoute("/x/sp", handler.SubmitJobPostWithoutPaymentHandler(svr), []string{"POST"})

	// @admin: approve job
	svr.RegisterRoute("/x/a", handler.ApproveJobPageHandler(svr), []string{"POST"})

	// @admin: permanently delete job and all child resources (image, clickouts, edit token)
	svr.RegisterRoute("/x/j/d", handler.PermanentlyDeleteJobByToken(svr), []string{"POST"})

	log.Fatal(svr.Run())
}
