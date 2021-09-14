package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/0x13a/golang.cafe/pkg/config"
	"github.com/0x13a/golang.cafe/pkg/database"

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

	since := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	cs, err := database.InferCompaniesFromJobs(conn, since)
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
				log.Printf("%s: description: %s\n", c.URL, description)
			}
		})
		if description != "" {
			c.Description = &description
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
		if err := database.SaveCompany(conn, c); err != nil {
			log.Println(err)
			continue
		}
		log.Println(c.Name)
	}
	if err := database.DeleteStaleImages(conn); err != nil {
		log.Fatal(err)
	}
}
