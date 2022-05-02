package seo

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/golang-cafe/job-board/internal/blog"
	"github.com/golang-cafe/job-board/internal/company"
	"github.com/golang-cafe/job-board/internal/database"
	"github.com/golang-cafe/job-board/internal/developer"
)

func StaticPages(siteJobCategory string) []string {
	return []string{
		"hire-" + siteJobCategory + "-developers",
		"privacy-policy",
		"terms-of-service",
		"about",
		"newsletter",
		"blog",
		"support",
	}
}

type BlogPost struct {
	Title, Path string
}

func BlogPages(blogRepo *blog.Repository) ([]BlogPost, error) {
	posts := make([]BlogPost, 0, 100)
	blogs, err := blogRepo.GetAllPublished()
	if err != nil {
		return posts, err
	}
	for _, b := range blogs {
		posts = append(posts, BlogPost{
			Title: b.Title,
			Path:  b.Slug,
		})
	}

	return posts, nil
}

func GeneratePostAJobSEOLandingPages(conn *sql.DB, siteJobCategory string) ([]string, error) {
	siteJobCategory = strings.Title(siteJobCategory)
	var seoLandingPages []string
	locs, err := database.GetSEOLocations(conn)
	if err != nil {
		return seoLandingPages, err
	}
	for _, loc := range locs {
		seoLandingPages = appendPostAJobSEOLandingPageForLocation(siteJobCategory, seoLandingPages, loc.Name)
	}

	return seoLandingPages, nil
}

func GenerateSalarySEOLandingPages(conn *sql.DB, siteJobCategory string) ([]string, error) {
	siteJobCategory = strings.Title(siteJobCategory)
	var landingPages []string
	locs, err := database.GetSEOLocations(conn)
	if err != nil {
		return landingPages, err
	}
	for _, loc := range locs {
		landingPages = appendSalarySEOLandingPageForLocation(siteJobCategory, landingPages, loc.Name)
	}

	return landingPages, nil
}

func GenerateCompaniesLandingPages(conn *sql.DB, siteJobCategory string) ([]string, error) {
	siteJobCategory = strings.Title(siteJobCategory)
	var landingPages []string
	locs, err := database.GetSEOLocations(conn)
	if err != nil {
		return landingPages, err
	}
	for _, loc := range locs {
		landingPages = appendCompaniesLandingPagesForLocation(siteJobCategory, landingPages, loc.Name)
	}

	return landingPages, nil
}

func appendSalarySEOLandingPageForLocation(siteJobCategory string, landingPages []string, loc string) []string {
	tmpl := `%s-Developer-Salary-%s`
	if strings.ToLower(loc) == "remote" {
		return append(landingPages, fmt.Sprintf(`Remote-%s-Developer-Salary`, siteJobCategory))
	}
	return append(landingPages, fmt.Sprintf(tmpl, siteJobCategory, strings.ReplaceAll(loc, " ", "-")))
}

func appendPostAJobSEOLandingPageForLocation(siteJobCategory string, seoLandingPages []string, loc string) []string {
	tmpl := `Hire-%s-Developers-In-%s`
	if strings.ToLower(loc) == "remote" {
		return append(seoLandingPages, fmt.Sprintf(`Hire-Remote-%s-Developers`, siteJobCategory))
	}
	return append(seoLandingPages, fmt.Sprintf(tmpl, siteJobCategory, strings.ReplaceAll(loc, " ", "-")))
}

func appendCompaniesLandingPagesForLocation(siteJobCategory string, landingPages []string, loc string) []string {
	tmpl := `Companies-Using-%s-In-%s`
	if strings.ToLower(loc) == "remote" {
		return append(landingPages, fmt.Sprintf(`Remote-Companies-Using-%s`, siteJobCategory))
	}
	return append(landingPages, fmt.Sprintf(tmpl, siteJobCategory, strings.ReplaceAll(loc, " ", "-")))
}

func appendSearchSEOSalaryLandingPageForLocation(siteJobCategory string, seoLandingPages []database.SEOLandingPage, loc database.SEOLocation) []database.SEOLandingPage {
	salaryBands := []string{"50000", "10000", "150000", "200000"}
	tmp := make([]database.SEOLandingPage, 0, len(salaryBands))
	if loc.Name == "" {
		for _, salaryBand := range salaryBands {
			tmp = append(tmp, database.SEOLandingPage{
				URI: fmt.Sprintf("%s-Jobs-Paying-%s-USD-year", siteJobCategory, salaryBand),
			})
		}

		return append(seoLandingPages, tmp...)
	}

	if loc.Population < 1000000 {
		return seoLandingPages
	}

	for _, salaryBand := range salaryBands {
		tmp = append(tmp, database.SEOLandingPage{
			URI: fmt.Sprintf("%s-Jobs-In-%s-Paying-%s-USD-year", siteJobCategory, url.PathEscape(strings.ReplaceAll(loc.Name, " ", "-")), salaryBand),
		})
	}

	return append(seoLandingPages, tmp...)
}

func GenerateSearchSEOLandingPages(conn *sql.DB, siteJobCategory string) ([]database.SEOLandingPage, error) {
	siteJobCategory = strings.Title(siteJobCategory)
	var seoLandingPages []database.SEOLandingPage
	locs, err := database.GetSEOLocations(conn)
	if err != nil {
		return seoLandingPages, err
	}
	skills, err := database.GetSEOskills(conn)
	if err != nil {
		return seoLandingPages, err
	}

	seoLandingPages = appendSearchSEOSalaryLandingPageForLocation(siteJobCategory, seoLandingPages, database.SEOLocation{})

	for _, loc := range locs {
		seoLandingPages = appendSearchSEOLandingPageForLocationAndSkill(siteJobCategory, seoLandingPages, loc, database.SEOSkill{})
		seoLandingPages = appendSearchSEOSalaryLandingPageForLocation(siteJobCategory, seoLandingPages, loc)
	}
	for _, skill := range skills {
		seoLandingPages = appendSearchSEOLandingPageForLocationAndSkill(siteJobCategory, seoLandingPages, database.SEOLocation{}, skill)
	}

	return seoLandingPages, nil
}

func GenerateDevelopersSkillLandingPages(repo *developer.Repository, siteJobCategory string) ([]string, error) {
	siteJobCategory = strings.Title(siteJobCategory)
	var landingPages []string
	devSkills, err := repo.GetDeveloperSkills()
	if err != nil {
		return landingPages, err
	}
	for _, skill := range devSkills {
		devSkills = append(devSkills, fmt.Sprintf("%s-%s-Developers", siteJobCategory, url.PathEscape(skill)))
	}

	return landingPages, nil
}

func GenerateDevelopersLocationPages(conn *sql.DB, siteJobCategory string) ([]string, error) {
	siteJobCategory = strings.Title(siteJobCategory)
	var landingPages []string
	locs, err := database.GetSEOLocations(conn)
	if err != nil {
		return landingPages, err
	}
	for _, loc := range locs {
		landingPages = append(landingPages, fmt.Sprintf("%s-Developers-In-%s", siteJobCategory, url.PathEscape(loc.Name)))
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

func appendSearchSEOLandingPageForLocationAndSkill(siteJobCategory string, seoLandingPages []database.SEOLandingPage, loc database.SEOLocation, skill database.SEOSkill) []database.SEOLandingPage {
	templateBoth := siteJobCategory + `-%s-Jobs-In-%s`
	templateSkill := siteJobCategory + `-%s-Jobs`
	templateLoc := siteJobCategory + `-Jobs-In-%s`

	templateRemoteLoc := `Remote-` + siteJobCategory + `-Jobs`
	templateRemoteBoth := `Remote-` + siteJobCategory + `-%s-Jobs`
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
