package seo

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"

	"github.com/0x13a/golang.cafe/internal/company"
	"github.com/0x13a/golang.cafe/internal/developer"
	"github.com/0x13a/golang.cafe/internal/database"
)

func StaticPages() []string {
	return []string{
		"hire-golang-developers",
		"privacy-policy",
		"terms-of-service",
		"about",
		"newsletter",
		"blog",
		"support",
		"ksuid",
		"whats-my-ip",
	}
}

type BlogPost struct {
	Title, Path string
}

func BlogPages(blogDir string) ([]BlogPost, error) {
	posts := make([]BlogPost, 0, 100)
	files, err := ioutil.ReadDir(blogDir)
	if err != nil {
		return posts, err
	}
	for _, f := range files {
		posts = append(posts, BlogPost{
			Title: strings.Title(
				strings.ReplaceAll(
					strings.ReplaceAll(f.Name(), ".html", ""),
					"-",
					" ",
				)),
			Path: f.Name(),
		})
	}

	return posts, nil
}

func GeneratePostAJobSEOLandingPages(conn *sql.DB) ([]string, error) {
	var seoLandingPages []string
	locs, err := database.GetSEOLocations(conn)
	if err != nil {
		return seoLandingPages, err
	}
	for _, loc := range locs {
		seoLandingPages = appendPostAJobSEOLandingPageForLocation(seoLandingPages, loc.Name)
	}

	return seoLandingPages, nil
}

func GenerateSalarySEOLandingPages(conn *sql.DB) ([]string, error) {
	var landingPages []string
	locs, err := database.GetSEOLocations(conn)
	if err != nil {
		return landingPages, err
	}
	for _, loc := range locs {
		landingPages = appendSalarySEOLandingPageForLocation(landingPages, loc.Name)
	}

	return landingPages, nil
}

func GenerateCompaniesLandingPages(conn *sql.DB) ([]string, error) {
	var landingPages []string
	locs, err := database.GetSEOLocations(conn)
	if err != nil {
		return landingPages, err
	}
	for _, loc := range locs {
		landingPages = appendCompaniesLandingPagesForLocation(landingPages, loc.Name)
	}

	return landingPages, nil
}

func appendSalarySEOLandingPageForLocation(landingPages []string, loc string) []string {
	tmpl := `Golang-Developer-Salary-%s`
	if strings.ToLower(loc) == "remote" {
		return append(landingPages, `Remote-Golang-Developer-Salary`)
	}
	return append(landingPages, fmt.Sprintf(tmpl, strings.ReplaceAll(loc, " ", "-")))
}

func appendPostAJobSEOLandingPageForLocation(seoLandingPages []string, loc string) []string {
	tmpl := `Hire-Golang-Developers-In-%s`
	if strings.ToLower(loc) == "remote" {
		return append(seoLandingPages, `Hire-Remote-Golang-Developers`)
	}
	return append(seoLandingPages, fmt.Sprintf(tmpl, strings.ReplaceAll(loc, " ", "-")))
}

func appendCompaniesLandingPagesForLocation(landingPages []string, loc string) []string {
	tmpl := `Companies-Using-Golang-In-%s`
	if strings.ToLower(loc) == "remote" {
		return append(landingPages, `Remote-Companies-Using-Golang`)
	}
	return append(landingPages, fmt.Sprintf(tmpl, strings.ReplaceAll(loc, " ", "-")))
}

func appendSearchSEOSalaryLandingPageForLocation(seoLandingPages []database.SEOLandingPage, loc database.SEOLocation) []database.SEOLandingPage {
	salaryBands := []string{"50000", "10000", "150000", "200000"}
	tmp := make([]database.SEOLandingPage, 0, len(salaryBands))
	if loc.Name == "" {
		for _, salaryBand := range salaryBands {
			tmp = append(tmp, database.SEOLandingPage{
				URI: fmt.Sprintf("Golang-Jobs-Paying-%s-USD-year", salaryBand),
			})
		}

		return append(seoLandingPages, tmp...)
	}

	if loc.Population < 1000000 {
		return seoLandingPages
	}

	for _, salaryBand := range salaryBands {
		tmp = append(tmp, database.SEOLandingPage{
			URI: fmt.Sprintf("Golang-Jobs-In-%s-Paying-%s-USD-year", url.PathEscape(strings.ReplaceAll(loc.Name, " ", "-")), salaryBand),
		})
	}

	return append(seoLandingPages, tmp...)
}

func GenerateSearchSEOLandingPages(conn *sql.DB) ([]database.SEOLandingPage, error) {
	var seoLandingPages []database.SEOLandingPage
	locs, err := database.GetSEOLocations(conn)
	if err != nil {
		return seoLandingPages, err
	}
	skills, err := database.GetSEOskills(conn)
	if err != nil {
		return seoLandingPages, err
	}

	seoLandingPages = appendSearchSEOSalaryLandingPageForLocation(seoLandingPages, database.SEOLocation{})

	for _, loc := range locs {
		seoLandingPages = appendSearchSEOLandingPageForLocationAndSkill(seoLandingPages, loc, database.SEOSkill{})
		seoLandingPages = appendSearchSEOSalaryLandingPageForLocation(seoLandingPages, loc)
	}
	for _, skill := range skills {
		seoLandingPages = appendSearchSEOLandingPageForLocationAndSkill(seoLandingPages, database.SEOLocation{}, skill)
	}

	return seoLandingPages, nil
}

func GenerateDevelopersSkillLandingPages(repo *developer.Repository) ([]string, error) {
	var landingPages []string
	devSkills, err := repo.GetDeveloperSkills()
	if err != nil {
		return landingPages, err
	}
	for _, skill := range devSkills {
		devSkills = append(devSkills, fmt.Sprintf("Golang-%s-Developers", url.PathEscape(skill)))
	}

	return landingPages, nil
}

func GenerateDevelopersLocationPages(conn *sql.DB) ([]string, error) {
	var landingPages []string
	locs, err := database.GetSEOLocations(conn)
	if err != nil {
		return landingPages, err
	}
	for _, loc := range locs {
		landingPages = append(landingPages, fmt.Sprintf("Golang-Developers-In-%s", url.PathEscape(loc.Name)))
	}

	return landingPages, nil
}

func GenerateDevelopersProfileLandingPages(repo *developer.Repository) ([]string, error) {
	var landingPages []string
	profiles, err := repo.GetDeveloperSlugs()
	if err != nil {
		return landingPages, err
	}
	for _, slug := range profiles {
		landingPages = append(landingPages, fmt.Sprintf("developer/%s", url.PathEscape(slug)))
	}

	return landingPages, nil
}

func GenerateCompanyProfileLandingPages(companyRepo *company.Repository) ([]string, error) {
	var landingPages []string
	companies, err := companyRepo.GetCompanySlugs()
	if err != nil {
		return landingPages, err
	}
	for _, slug := range companies {
		landingPages = append(landingPages, fmt.Sprintf("company/%s", url.PathEscape(slug)))
	}

	return landingPages, nil
}

func appendSearchSEOLandingPageForLocationAndSkill(seoLandingPages []database.SEOLandingPage, loc database.SEOLocation, skill database.SEOSkill) []database.SEOLandingPage {
	templateBoth := `Golang-%s-Jobs-In-%s`
	templateSkill := `Golang-%s-Jobs`
	templateLoc := `Golang-Jobs-In-%s`

	templateRemoteLoc := `Remote-Golang-Jobs`
	templateRemoteBoth := `Remote-Golang-%s-Jobs`
	loc.Name = strings.ReplaceAll(loc.Name, " ", "-")
	skill.Name = strings.ReplaceAll(skill.Name, " ", "-")

	// Skill only
	if loc.Name == "" {
		return append(seoLandingPages, database.SEOLandingPage{
			URI:   fmt.Sprintf(templateSkill, url.PathEscape(skill.Name)),
			Skill: skill.Name,
		})
	}

	// Remote is special case
	if loc.Name == "Remote" {
		if skill.Name != "" {
			return append(seoLandingPages, database.SEOLandingPage{
				URI:      fmt.Sprintf(templateRemoteBoth, url.PathEscape(skill.Name)),
				Location: loc.Name,
			})
		} else {
			return append(seoLandingPages, database.SEOLandingPage{
				URI:      templateRemoteLoc,
				Location: loc.Name,
				Skill:    skill.Name,
			})
		}
	}

	// Location only
	if skill.Name == "" {
		return append(seoLandingPages, database.SEOLandingPage{
			URI:      fmt.Sprintf(templateLoc, url.PathEscape(loc.Name)),
			Location: loc.Name,
		})
	}

	// Both
	return append(seoLandingPages, database.SEOLandingPage{
		URI:      fmt.Sprintf(templateBoth, url.PathEscape(skill.Name), url.PathEscape(loc.Name)),
		Skill:    skill.Name,
		Location: loc.Name,
	})
}
