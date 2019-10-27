package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"compress/gzip"

	"github.com/0x13a/golang.cafe/pkg/database"
	"github.com/0x13a/golang.cafe/pkg/seo"
	"github.com/snabb/sitemap"
)

type Page struct {
	CreatedAt time.Time
	Title     string
}

var blogPosts = []Page{
	{
		CreatedAt: time.Now().UTC(),
		Title:     "my-5-favourite-online-resources-to-learn-golang-from-scratch",
	},
	{
		CreatedAt: time.Now().UTC(),
		Title:     "golang-random-number-generator",
	},
	{
		CreatedAt: time.Now().UTC(),
		Title:     "how-to-print-struct-variables-in-golang",
	},
	{
		CreatedAt: time.Now().UTC(),
		Title:     "golang-base64-encode-example",
	},
	{
		CreatedAt: time.Now().UTC(),
		Title:     "golang-read-file-example",
	},
	{
		CreatedAt: time.Now().UTC(),
		Title:     "golang-sleep-random-time",
	},
	{
		CreatedAt: time.Now().UTC(),
		Title:     "golang-http-server-example",
	},
	{
		CreatedAt: time.Now().UTC(),
		Title:     "golang-int-to-string-conversion-example",
	},
	{
		CreatedAt: time.Now().UTC(),
		Title:     "how-to-validate-url-in-go",
	},
}
var pages = []string{
	"hire-golang-developers",
	"privacy-policy",
	"terms-of-service",
	"about",
	"newsletter",
	"community",
	"ksuid",
}

func main() {
	databaseURL := os.Getenv("HEROKU_POSTGRESQL_PINK_URL")
	conn, err := database.GetDbConn(databaseURL)
	if err != nil {
		log.Fatalf("unable to connect to postgres: %v", err)
	}
	database.SaveSEOSkillFromCompany(conn)
	landingPages, err := seo.GenerateSearchSEOLandingPages(conn)
	if err != nil {
		log.Fatalf("unable to generate landing pages %v", err)
	}
	postAJobLandingPages, err := seo.GeneratePostAJobSEOLandingPages(conn)
	if err != nil {
		log.Fatalf("unable to generate landing pages for post a job %v", err)
	}
	salaryLandingPages, err := seo.GenerateSalarySEOLandingPages(conn)
	if err != nil {
		log.Fatalf("unable to generate landing pages for salary %v", err)
	}
	jobs, err := database.JobPostByCreatedAt(conn)
	if err != nil {
		log.Fatalf("unable to retrieve jobs from db: %#v", err)
	}
	index := sitemap.NewSitemapIndex()
	n := time.Now().UTC()

	total := 0
	// job sitemap
	jobSm := sitemap.New()
	for _, j := range jobs {
		t := time.Unix(j.CreatedAt, 0)
		jobSm.Add(&sitemap.URL{
			Loc:        fmt.Sprintf(`https://golang.cafe/job/%s`, j.Slug),
			LastMod:    &t,
			ChangeFreq: sitemap.Daily,
		})
	}
	err = SaveSitemap(jobSm, "static/sitemap-1.xml.gz")
	if err != nil {
		log.Fatalf("unable to save blog sitemap-1.xml.gz: %v", err)
	}
	index.Add(&sitemap.URL{
		Loc:     `https://golang.cafe/sitemap-1.xml.gz`,
		LastMod: &n,
	})
	fmt.Printf("generated %d entries for job sitemap\n", len(jobs))
	total = total + len(jobs)

	// blog sitemap
	blogSm := sitemap.New()
	for _, b := range blogPosts {
		blogSm.Add(&sitemap.URL{
			Loc:        fmt.Sprintf(`https://golang.cafe/blog/%s.html`, b.Title),
			LastMod:    &b.CreatedAt,
			ChangeFreq: sitemap.Weekly,
		})
	}
	err = SaveSitemap(blogSm, "static/sitemap-2.xml.gz")
	if err != nil {
		log.Fatalf("unable to save blog sitemap-2.xml.gz: %v", err)
	}
	index.Add(&sitemap.URL{
		Loc:     `https://golang.cafe/sitemap-2.xml.gz`,
		LastMod: &n,
	})
	fmt.Printf("generated %d entries for blog sitemap\n", len(blogPosts))
	total = total + len(blogPosts)

	// pages sitemap
	pagesSm := sitemap.New()
	pagesSm.Add(&sitemap.URL{
		Loc:        `https://golang.cafe/`,
		LastMod:    &n,
		ChangeFreq: sitemap.Daily,
	})
	for _, p := range pages {
		pagesSm.Add(&sitemap.URL{
			Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p),
			LastMod:    &n,
			ChangeFreq: sitemap.Daily,
		})
	}
	err = SaveSitemap(pagesSm, "static/sitemap-3.xml.gz")
	if err != nil {
		log.Fatalf("unable to save pages sitemap-3.xml.gz: %v", err)
	}
	index.Add(&sitemap.URL{
		Loc:     `https://golang.cafe/sitemap-3.xml.gz`,
		LastMod: &n,
	})
	fmt.Printf("generated %d entries for pages sitemap\n", len(pages)+1)
	total = total + len(pages) + 1

	// post a job landing sitemap
	postJobSm := sitemap.New()
	for _, p := range postAJobLandingPages {
		postJobSm.Add(&sitemap.URL{
			Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p),
			LastMod:    &n,
			ChangeFreq: sitemap.Daily,
		})
	}
	err = SaveSitemap(postJobSm, "static/sitemap-4.xml.gz")
	if err != nil {
		log.Fatalf("unable to postJobSm sitemap-4.xml.gz: %v", err)
	}
	index.Add(&sitemap.URL{
		Loc:     `https://golang.cafe/sitemap-4.xml.gz`,
		LastMod: &n,
	})
	fmt.Printf("generated %d entries for post job landing pages sitemap\n", len(postAJobLandingPages))
	total = total + len(postAJobLandingPages)

	// salary sitemap
	salarySm := sitemap.New()
	for _, p := range salaryLandingPages {
		salarySm.Add(&sitemap.URL{
			Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p),
			LastMod:    &n,
			ChangeFreq: sitemap.Daily,
		})
	}
	err = SaveSitemap(salarySm, "static/sitemap-5.xml.gz")
	if err != nil {
		log.Fatalf("unable to save pages sitemap-5.xml.gz: %v", err)
	}
	index.Add(&sitemap.URL{
		Loc:     `https://golang.cafe/sitemap-5.xml.gz`,
		LastMod: &n,
	})
	fmt.Printf("generated %d entries for salary landing pages sitemap\n", len(salaryLandingPages))
	total = total + len(salaryLandingPages)

	// landing page sitemap
	landingPagesSm := sitemap.New()
	counter := 0
	last := 6
	for _, p := range landingPages {
		landingPagesSm.Add(&sitemap.URL{
			Loc:        fmt.Sprintf(`https://golang.cafe/%s`, p.URI),
			LastMod:    &n,
			ChangeFreq: sitemap.Daily,
		})
		counter++
		if counter == 3000 {
			err = SaveSitemap(landingPagesSm, fmt.Sprintf("static/sitemap-%d.xml.gz", last))
			if err != nil {
				log.Fatalf("unable to save pages sitemap-%d.xml.gz: %v", last, err)
			}
			index.Add(&sitemap.URL{
				Loc:     fmt.Sprintf(`https://golang.cafe/sitemap-%d.xml.gz`, last),
				LastMod: &n,
			})
			fmt.Printf("generated %d entries for landing pages sitemap %d\n", counter, last)
			total = total + counter
			last++
			counter = 0
			landingPagesSm = sitemap.New()
		}
	}
	if counter > 0 {
		err = SaveSitemap(landingPagesSm, fmt.Sprintf("static/sitemap-%d.xml.gz", last+1))
		if err != nil {
			log.Fatalf("unable to save pages sitemap-%d.xml.gz: %v", last+1, err)
		}
		index.Add(&sitemap.URL{
			Loc:     fmt.Sprintf(`https://golang.cafe/sitemap-%d.xml.gz`, last+1),
			LastMod: &n,
		})
		total = total + counter
		fmt.Printf("generated %d entries for landing pages sitemap %d\n", counter, last+1)
	}

	err = SaveSitemapIndex(index)
	if err != nil {
		log.Fatalf("unable to save sitemap index %v", err)
	}
	fmt.Printf("total number of pages generated %d", total)
}

func SaveSitemap(sm *sitemap.Sitemap, loc string) error {
	f, err := os.OpenFile(loc, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	gzipWriter, err := gzip.NewWriterLevel(f, 9)
	if err != nil {
		return err
	}
	_, err = sm.WriteTo(gzipWriter)
	if err != nil {
		return err
	}
	err = gzipWriter.Close()
	if err != nil {
		return err
	}
	time.Sleep(200 * time.Millisecond)
	return nil
}

func SaveSitemapIndex(sm *sitemap.SitemapIndex) error {
	f, err := os.OpenFile("static/sitemap.xml", os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = sm.WriteTo(f)
	if err != nil {
		return err
	}
	return nil
}
