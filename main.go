package main

import (
	"log"
	"net/http"

	"github.com/0x13a/golang.cafe/pkg/authoriser"
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
	sessionStore := sessions.NewCookieStore(cfg.SessionKey)
	auth := authoriser.NewAuthoriser(cfg)

	svr := server.NewServer(
		cfg,
		conn,
		mux.NewRouter(),
		template.NewTemplate(),
		emailClient,
		ipgeolocation.NewIPGeoLocation(cfg.IPGeoLocationApiKey, cfg.IPGeoLocationURI),
		sessionStore,
	)

	svr.RegisterRoute("/sitemap.xml", handler.SitemapIndexHandler, []string{"GET"})
	svr.RegisterRoute("/sitemap-{n}.xml.gz", handler.SitemapHandler, []string{"GET"})
	svr.RegisterRoute("/robots.txt", handler.RobotsTxtHandler, []string{"GET"})
	svr.RegisterRoute("/.well-known/security.txt", handler.WellKnownSecurityHandler, []string{"GET"})

	svr.RegisterPathPrefix("/s/", http.StripPrefix("/s/", http.FileServer(http.Dir("./static/assets"))), []string{"GET"})
	svr.RegisterPathPrefix("/blog/", http.StripPrefix("/blog/", http.FileServer(http.Dir("./static/blog"))), []string{"GET"})

	svr.RegisterRoute("/about", handler.AboutPageHandler, []string{"GET"})
	svr.RegisterRoute("/privacy-policy", handler.PrivacyPolicyPageHandler, []string{"GET"})
	svr.RegisterRoute("/terms-of-service", handler.TermsOfServicePageHandler, []string{"GET"})

	svr.RegisterRoute("/", handler.IndexPageHandler(svr), []string{"GET"})

	// view newsletter
	svr.RegisterRoute("/newsletter", handler.ViewNewsletterPageHandler(svr), []string{"GET"})

	// view community
	svr.RegisterRoute("/community", handler.ViewCommunityNewsletterPageHandler(svr), []string{"GET"})

	// generate ksuid
	svr.RegisterRoute("/ksuid", handler.GenerateKsuIDPageHandler(svr), []string{"GET"})

	// post a job succeeded
	svr.RegisterRoute("/x/j/p/1", handler.PostAJobSuccessPageHandler(svr), []string{"GET"})

	// post a job failed
	svr.RegisterRoute("/x/j/p/0", handler.PostAJobFailurePageHandler(svr), []string{"GET"})

	// newsletter member save
	svr.RegisterRoute("/x/n/m/s", handler.SaveMemberToNewsletterPageHandler(svr), []string{"GET"})

	// community interest member save
	svr.RegisterRoute("/x/c/m/s", handler.SaveMemberToCommunityNewsletterPageHandler(svr), []string{"GET"})

	// apply for job
	svr.RegisterRoute("/x/a/e", handler.ApplyForJobPageHandler(svr), []string{"POST"})

	// apply to job confirmation
	svr.RegisterRoute("/apply/{token}", handler.ApplyToJobConfirmation(svr), []string{"GET"})

	// submit job post
	svr.RegisterRoute("/x/s", handler.SubmitJobPostPageHandler(svr), []string{"POST"})

	// save media file
	svr.RegisterRoute("/x/s/m", handler.SaveMediaPageHandler(svr), []string{"POST"})

	// update media file
	svr.RegisterRoute("/x/s/m/{id}", handler.UpdateMediaPageHandler(svr), []string{"PUT"})

	// retrieve media file
	svr.RegisterRoute("/x/s/m/{id}", handler.RetrieveMediaPageHandler(svr), []string{"GET"})

	// stripe payment confirmation webhook
	svr.RegisterRoute("/x/stripe/webhook", handler.StripePaymentConfirmationWebookHandler(svr), []string{"POST"})

	// track job clickout
	svr.RegisterRoute("/x/j/c/{id}", handler.TrackJobClickoutPageHandler(svr), []string{"GET"})

	// view job by slug
	svr.RegisterRoute("/job/{slug}", handler.JobBySlugPageHandler(svr), []string{"GET"})

	//
	// auth routes
	//

	svr.RegisterRoute("/auth", handler.GetAuthPageHandler(svr), []string{"GET"})

	svr.RegisterRoute("/x/auth", handler.PostAuthPageHandler(svr, auth), []string{"POST"})

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

	//
	// deprecated routes
	//

	// @deprecated: view job by timestamp id
	svr.RegisterRoute("/j/{id}", handler.JobByTimestampIDPageHandler(svr), []string{"GET"})

	log.Fatal(svr.Run())
}
