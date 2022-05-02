package payment

import (
	"encoding/json"
	"fmt"

	"github.com/golang-cafe/job-board/internal/job"

	stripe "github.com/stripe/stripe-go"
	session "github.com/stripe/stripe-go/checkout/session"
	webhook "github.com/stripe/stripe-go/webhook"

	"strings"
)

type Repository struct {
	stripeKey string
	siteName  string
	siteHost  string
}

func NewRepository(stripeKey, siteName, siteHost string) *Repository {
	return &Repository{
		stripeKey: stripeKey,
		siteName:  siteName,
		siteHost:  siteHost,
	}
}

func AdTypeToAmount(adType int64) int64 {
	switch adType {
	case job.JobAdBasic:
		return 3900
	case job.JobAdSponsoredBackground:
		return 3900
	case job.JobAdSponsoredPinnedFor30Days:
		return 12900
	case job.JobAdSponsoredPinnedFor7Days:
		return 5900
	case job.JobAdWithCompanyLogo:
		return 4900
	case job.JobAdSponsoredPinnedFor60Days:
		return 19900
	}

	return 0
}

func AdTypeToDescription(adType int64) string {
	switch adType {
	case job.JobAdBasic:
		return "Standard Ad"
	case job.JobAdSponsoredBackground:
		return "Sponsored Ad Highlighted Background"
	case job.JobAdSponsoredPinnedFor30Days:
		return "Sponsored Ad Pinned For 30 Days"
	case job.JobAdSponsoredPinnedFor7Days:
		return "Sponsored Ad Pinned For 7 Days"
	case job.JobAdWithCompanyLogo:
		return "Standard Ad With Company Logo"
	case job.JobAdSponsoredPinnedFor60Days:
		return "Sponsored Ad Pinned For 60 Days"
	}

	return ""
}

func isApplicable(jobRq *job.JobRq) bool {
	return jobRq.AdType >= 0 && jobRq.AdType <= 5
}

func (r Repository) CreateGenericSession(email, currency string, amount int) (*stripe.CheckoutSession, error) {
	stripe.Key = r.stripeKey
	params := &stripe.CheckoutSessionParams{
		BillingAddressCollection: stripe.String("required"),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Name:     stripe.String(fmt.Sprintf("%s Sponsored Ad", r.siteName)),
				Amount:   stripe.Int64(int64(amount)),
				Currency: stripe.String(currency),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL:    stripe.String(fmt.Sprintf("https://%s/x/j/p/1", r.siteHost)),
		CancelURL:     stripe.String(fmt.Sprintf("https://%s/x/j/p/0", r.siteHost)),
		CustomerEmail: &email,
	}

	session, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("unable to create stripe session: %+v", err)
	}

	return session, nil
}

func (r Repository) CreateSession(jobRq *job.JobRq, jobToken string) (*stripe.CheckoutSession, error) {
	if !isApplicable(jobRq) {
		return nil, nil
	}
	stripe.Key = r.stripeKey
	params := &stripe.CheckoutSessionParams{
		BillingAddressCollection: stripe.String("required"),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Name:     stripe.String(fmt.Sprintf("%s Sponsored Ad", r.siteName)),
				Amount:   stripe.Int64(AdTypeToAmount(jobRq.AdType)),
				Currency: stripe.String(strings.ToLower(jobRq.CurrencyCode)),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL:    stripe.String(fmt.Sprintf("https://%s/edit/%s?payment=1&callback=1", r.siteHost, jobToken)),
		CancelURL:     stripe.String(fmt.Sprintf("https://%s/edit/%s?payment=0&callback=1", r.siteHost, jobToken)),
		CustomerEmail: &jobRq.Email,
	}

	session, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("unable to create stripe session: %+v", err)
	}

	return session, nil
}

func HandleCheckoutSessionComplete(body []byte, endpointSecret, stripeSig string) (*stripe.CheckoutSession, error) {
	event, err := webhook.ConstructEvent(body, stripeSig, endpointSecret)
	if err != nil {
		return nil, fmt.Errorf("error verifying webhook signature: %v\n", err)
	}
	// Handle the checkout.session.completed event
	if event.Type == "checkout.session.completed" {
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			return nil, fmt.Errorf("error parsing webhook JSON: %v\n", err)
		}
		return &session, nil
	}
	return nil, nil
}
