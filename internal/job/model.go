package job

import (
	"time"

	"github.com/lib/pq"
)

const (
	jobEventPageView = "page_view"
	jobEventClickout = "clickout"

	SearchTypeJob    = "job"
	SearchTypeSalary = "salary"
)

const (
	JobAdBasic = iota
	JobAdSponsoredBackground
	JobAdSponsoredPinnedFor30Days
	JobAdSponsoredPinnedFor7Days
	JobAdWithCompanyLogo
	JobAdSponsoredPinnedFor60Days
	JobAdSponsoredPinnedFor90Days
)

type Job struct {
	CreatedAt                       int64
	JobTitle                        string
	Company                         string
	SalaryMin                       string
	SalaryMax                       string
	SalaryCurrency                  string
	SalaryPeriod                    string
	SalaryRange                     string
	Location                        string
	Description                     string
	Perks                           string
	InterviewProcess                string
	HowToApply                      string
	Email                           string
	Expired                         bool
	LastWeekClickouts               int
	PlanType                        string
	PlanDuration                    int
	NewsletterEligibilityExpiredAt  time.Time
	BlogEligibilityExpiredAt        time.Time
	SocialMediaEligibilityExpiredAt time.Time
	FrontPageEligibilityExpiredAt   time.Time
	CompanyPageEligibilityExpiredAt time.Time
	PlanExpiredAt                   time.Time
}

type JobRq struct {
	JobTitle          string `json:"job_title"`
	Location          string `json:"job_location"`
	Company           string `json:"company_name"`
	CompanyURL        string `json:"company_url"`
	SalaryMin         string `json:"salary_min"`
	SalaryMax         string `json:"salary_max"`
	SalaryCurrency    string `json:"salary_currency"`
	Description       string `json:"job_description"`
	HowToApply        string `json:"how_to_apply"`
	Perks             string `json:"perks"`
	InterviewProcess  string `json:"interview_process,omitempty"`
	Email             string `json:"company_email"`
	StripeToken       string `json:"stripe_token,omitempty"`
	PlanType          string `json:"plan_type"`
	PlanDurationStr   string `json:"plan_duration"`
	PlanDuration      int
	CurrencyCode      string `json:"currency_code"`
	CompanyIconID     string `json:"company_icon_id,omitempty"`
	SalaryCurrencyISO string `json:"salary_currency_iso"`
	VisaSponsorship   bool   `json:"visa_sponsorship,omitempty"`
}

const (
	JobPlanTypeBasic    = "basic"
	JobPlanTypePro      = "pro"
	JobPlanTypePlatinum = "platinum"
)

type JobRqUpsell struct {
	Token           string `json:"token"`
	Email           string `json:"email"`
	StripeToken     string `json:"stripe_token,omitempty"`
	PlanType        string `json:"plan_type"`
	PlanDuration    int
	PlanDurationStr string `json:"plan_duration"`
}

type JobRqUpdate struct {
	JobTitle         string `json:"job_title"`
	Location         string `json:"job_location"`
	Company          string `json:"company_name"`
	CompanyURL       string `json:"company_url"`
	SalaryMin        string `json:"salary_min"`
	SalaryMax        string `json:"salary_max"`
	SalaryCurrency   string `json:"salary_currency"`
	Description      string `json:"job_description"`
	HowToApply       string `json:"how_to_apply"`
	Perks            string `json:"perks"`
	InterviewProcess string `json:"interview_process"`
	Email            string `json:"company_email"`
	Token            string `json:"token"`
	CompanyIconID    string `json:"company_icon_id,omitempty"`
	SalaryPeriod     string `json:"salary_period"`
}

type JobPost struct {
	ID                              int
	CreatedAt                       int64
	TimeAgo                         string
	JobTitle                        string
	Company                         string
	CompanyURL                      string
	SalaryRange                     string
	Location                        string
	JobDescription                  string
	Perks                           string
	InterviewProcess                string
	HowToApply                      string
	Slug                            string
	SalaryCurrency                  string
	SalaryMin                       int64
	SalaryMax                       int64
	CompanyIconID                   string
	ExternalID                      string
	EditToken                       string
	IsQuickApply                    bool
	ApprovedAt                      *time.Time
	CompanyEmail                    string
	SalaryPeriod                    string
	CompanyURLEnc                   string
	Expired                         bool
	LastWeekClickouts               int
	PlanType                        string
	PlanDuration                    int
	NewsletterEligibilityExpiredAt  time.Time
	BlogEligibilityExpiredAt        time.Time
	SocialMediaEligibilityExpiredAt time.Time
	FrontPageEligibilityExpiredAt   time.Time
	CompanyPageEligibilityExpiredAt time.Time
	PlanExpiredAt                   time.Time
	JobDescriptionHTML              interface{}
	InterviewProcessHTML            interface{}
	PerksHTML                       interface{}
}

type JobPostForEdit struct {
	ID                                                                        int
	JobTitle, Company, CompanyEmail, CompanyURL, Location                     string
	SalaryMin, SalaryMax                                                      int
	SalaryCurrency, JobDescription, Perks, InterviewProcess, HowToApply, Slug string
	CreatedAt                                                                 time.Time
	ApprovedAt                                                                pq.NullTime
	CompanyIconID                                                             string
	ExternalID                                                                string
	SalaryPeriod                                                              string
	PlanType                                                                  string
	PlanDuration                                                              int
	NewsletterEligibilityExpiredAt                                            time.Time
	BlogEligibilityExpiredAt                                                  time.Time
	SocialMediaEligibilityExpiredAt                                           time.Time
	FrontPageEligibilityExpiredAt                                             time.Time
	CompanyPageEligibilityExpiredAt                                           time.Time
	PlanExpiredAt                                                             time.Time
}

type JobStat struct {
	Date      string `json:"date"`
	Clickouts int    `json:"clickouts"`
	PageViews int    `json:"pageviews"`
}

type JobApplyURL struct {
	ID  int
	URL string
}

type Applicant struct {
	Token       string
	Cv          []byte
	Email       string
	CreatedAt   time.Time
	ConfirmedAt pq.NullTime
	CvSize      int
}
