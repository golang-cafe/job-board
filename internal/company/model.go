package company

import (
	"time"
)

const (
	SearchTypeCompany = "company"
)

type Company struct {
	ID                              string
	Name                            string
	URL                             string
	Locations                       string
	IconImageID                     string
	Description                     *string
	LastJobCreatedAt                time.Time
	TotalJobCount                   int
	ActiveJobCount                  int
	Featured                        bool
	Slug                            string
	Twitter                         *string
	Github                          *string
	Linkedin                        *string
	CompanyPageEligibilityExpiredAt time.Time
}
