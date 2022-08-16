package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-cafe/job-board/internal/company"
	"github.com/golang-cafe/job-board/internal/config"
	"github.com/golang-cafe/job-board/internal/database"

	"github.com/PuerkitoBio/goquery"
	"github.com/gosimple/slug"
	"github.com/segmentio/ksuid"
)

func main() {
	log.Println("inferring companies from jobs")
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("unable to load config %v", err)
	}
	conn, err := database.GetDbConn(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("unable to connect to postgres: %v", err)
	}
	defer database.CloseDbConn(conn)

	companyRepo := company.NewRepository(conn)

	since := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	cs, err := companyRepo.InferCompaniesFromJobs(since)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("inferred %d companies...\n", len(cs))
	for _, c := range cs {
		res, err := http.Get(c.URL)
		if err != nil {
			log.Println(err)
			continue
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			log.Printf("GET %s: status code error: %d %s", c.URL, res.StatusCode, res.Status)
			continue
		}

		doc, err := goquery.NewDocumentFromReader(res.Body)
		if err != nil {
			log.Println(err)
			continue
		}
		description := doc.Find("title").Text()
		twitter := ""
		doc.Find("meta").Each(func(i int, s *goquery.Selection) {
			if name, _ := s.Attr("name"); strings.EqualFold(name, "description") {
				var ok bool
				desc, ok := s.Attr("content")
				if !ok {
					log.Println("unable to retrieve content for description tag for companyURL ", c.URL)
					return
				}
				if desc != "" {
					description = desc
				}
				log.Printf("description: %s\n", description)
			}
			if name, _ := s.Attr("name"); strings.EqualFold(name, "twitter:site") {
				var ok bool
				twtr, ok := s.Attr("content")
				if !ok {
					log.Println("unable to retrieve content for twitter:site")
					return
				}
				if twtr != "" {
					twitter = "https://twitter.com/" + strings.Trim(twtr, "@")
				}
				log.Printf("twitter: %s\n", twitter)
			}
		})
		github := ""
		linkedin := ""
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			if href, ok := s.Attr("href"); ok && strings.Contains(href, "github.com/") {
				github = href
				log.Printf("github: %s\n", github)
			}
			if href, ok := s.Attr("href"); ok && strings.Contains(href, "linkedin.com/") {
				linkedin = href
				log.Printf("linkedin: %s\n", linkedin)
			}
			if twitter == "" {
				if href, ok := s.Attr("href"); ok && strings.Contains(href, "twitter.com/") {
					twitter = href
					log.Printf("twitter: %s\n", twitter)
				}
			}
		})
		if description != "" {
			c.Description = &description
		}
		if twitter != "" {
			c.Twitter = &twitter
		}
		if github != "" {
			c.Github = &github
		}
		if linkedin != "" {
			c.Linkedin = &linkedin
		}
		companyID, err := ksuid.NewRandom()
		if err != nil {
			log.Println(err)
			continue
		}
		newIconID, err := ksuid.NewRandom()
		if err != nil {
			log.Println(err)
			continue
		}
		if err := database.DuplicateImage(conn, c.IconImageID, newIconID.String()); err != nil {
			log.Println(err)
			continue
		}
		c.ID = companyID.String()
		c.Slug = slug.Make(c.Name)
		c.IconImageID = newIconID.String()
		if err := companyRepo.SaveCompany(c); err != nil {
			log.Println(err)
			continue
		}
		log.Println(c.Name)
	}
	if err := companyRepo.DeleteStaleImages(cfg.SiteLogoImageID); err != nil {
		log.Fatal(err)
	}
}
