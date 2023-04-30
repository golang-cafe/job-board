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
	siteProtocol string
}

func NewRepository(stripeKey, siteName, siteHost, siteProtocol string) *Repository {
	return &Repository{
		stripeKey: stripeKey,
		siteName:  siteName,
		siteHost:  siteHost,
		siteProtocol: siteProtocol,
	}
}

func PlanTypeAndDurationToAmount(planType string, planDuration int64, p1, p2, p3 int64) int64 {
	switch planType {
	case job.JobPlanTypeBasic:
		return p1*planDuration
	case job.JobPlanTypePro:
		return p2*planDuration
	case job.JobPlanTypePlatinum:
		return p3*planDuration
	}

	return 0
}

func PlanTypeAndDurationToDescription(planType string, planDuration int64) string {
	switch planType {
	case job.JobPlanTypeBasic:
		return fmt.Sprintf("Basic Plan x %d months", planDuration)
	case job.JobPlanTypePro:
		return fmt.Sprintf("Pro Plan x %d months", planDuration)
	case job.JobPlanTypePlatinum:
		return fmt.Sprintf("Platinum Plan x %d months", planDuration)
	}

	return ""
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

func (r Repository) CreateJobAdSession(jobRq *job.JobRq, jobToken string, monthlyAmount int64, numMonths int64) (*stripe.CheckoutSession, error) {
	stripe.Key = r.stripeKey
	params := &stripe.CheckoutSessionParams{
		BillingAddressCollection: stripe.String("required"),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Name:     stripe.String(fmt.Sprintf("%s Job Ad %s Plan", r.siteName, strings.Title(jobRq.PlanType))),
				Amount:   stripe.Int64(monthlyAmount),
				Currency: stripe.String("usd"),
				Quantity: stripe.Int64(numMonths),
			},
		},
		SuccessURL:    stripe.String(fmt.Sprintf("%s%s/edit/%s?payment=1&callback=1", r.siteProtocol, r.siteHost, jobToken)),
		CancelURL:     stripe.String(fmt.Sprintf("%s%s/edit/%s?payment=0&callback=1", r.siteProtocol, r.siteHost, jobToken)),
		CustomerEmail: &jobRq.Email,
	}

	session, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("unable to create stripe session: %+v", err)
	}

	return session, nil
}

func (r Repository) CreateDevDirectorySession(email string, userID string, monthlyAmount int64, numMonths int64, isRenew bool) (*stripe.CheckoutSession, error) {
	stripe.Key = r.stripeKey
	successURL := stripe.String(fmt.Sprintf("%s%s/auth?payment=1&email=%s", r.siteProtocol, r.siteHost, email))
	cancelURL := stripe.String(fmt.Sprintf("%s%s/auth?payment=0&email=%s", r.siteProtocol, r.siteHost, email))
	if isRenew {
		successURL = stripe.String(fmt.Sprintf("%s%s/profile/home?payment=1", r.siteProtocol, r.siteHost))
		cancelURL = stripe.String(fmt.Sprintf("%s%s/profile/home?payment=0", r.siteProtocol, r.siteHost))
	}
	params := &stripe.CheckoutSessionParams{
		BillingAddressCollection: stripe.String("required"),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Name:     stripe.String(fmt.Sprintf("%s Developer Directory %d Months Plan", r.siteName, numMonths)),
				Amount:   stripe.Int64(monthlyAmount),
				Currency: stripe.String("usd"),
				Quantity: stripe.Int64(numMonths),
			},
		},
		SuccessURL:    successURL,
		CancelURL:     cancelURL,
		CustomerEmail: &email,
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
