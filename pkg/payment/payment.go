package payment

import (
	"encoding/json"
	"fmt"

	"github.com/0x13a/golang.cafe/pkg/database"

	stripe "github.com/stripe/stripe-go"
	charge "github.com/stripe/stripe-go/charge"
	session "github.com/stripe/stripe-go/checkout/session"
	webhook "github.com/stripe/stripe-go/webhook"

	"strings"
)

func AdTypeToAmount(adType int64) int64 {
	switch adType {
	case database.JobAdBasic:
		return 3900
	case database.JobAdSponsoredBackground:
		return 3900
	case database.JobAdSponsoredPinnedFor30Days:
		return 12900
	case database.JobAdSponsoredPinnedFor7Days:
		return 5900
	case database.JobAdWithCompanyLogo:
		return 4900
	case database.JobAdSponsoredPinnedFor60Days:
		return 19900
	}

	return 0
}

func AdTypeToDescription(adType int64) string {
	switch adType {
	case database.JobAdBasic:
		return "Standard Ad"
	case database.JobAdSponsoredBackground:
		return "Sponsored Ad Highlighted Background"
	case database.JobAdSponsoredPinnedFor30Days:
		return "Sponsored Ad Pinned For 30 Days"
	case database.JobAdSponsoredPinnedFor7Days:
		return "Sponsored Ad Pinned For 7 Days"
	case database.JobAdWithCompanyLogo:
		return "Standard Ad With Company Logo"
	case database.JobAdSponsoredPinnedFor60Days:
		return "Sponsored Ad Pinned For 60 Days"
	}

	return ""
}

func ProcessPaymentIfApplicable(stripeKey string, jobRq *database.JobRq) error {
	if !isApplicable(jobRq) {
		return nil
	}
	stripe.Key = stripeKey
	chargeParams := &stripe.ChargeParams{
		Amount:       stripe.Int64(AdTypeToAmount(jobRq.AdType)),
		Currency:     stripe.String(strings.ToLower(jobRq.CurrencyCode)),
		Description:  stripe.String("Golang Cafe Sponsored Ad"),
		ReceiptEmail: &jobRq.Email,
	}
	chargeParams.SetSource(jobRq.StripeToken)
	_, err := charge.New(chargeParams)
	return err
}

func isApplicable(jobRq *database.JobRq) bool {
	return jobRq.AdType >= 0 && jobRq.AdType <= 4
}

func CreateGenericSession(stripeKey, email, currency string, amount int) (*stripe.CheckoutSession, error) {
	stripe.Key = stripeKey
	params := &stripe.CheckoutSessionParams{
		BillingAddressCollection: stripe.String("required"),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			&stripe.CheckoutSessionLineItemParams{
				Name:     stripe.String("Golang Cafe Sponsored Ad"),
				Amount:   stripe.Int64(int64(amount)),
				Currency: stripe.String(currency),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL:    stripe.String("https://golang.cafe/x/j/p/1"),
		CancelURL:     stripe.String("https://golang.cafe/x/j/p/0"),
		CustomerEmail: &email,
	}

	session, err := session.New(params)
	if err != nil {
		return nil, fmt.Errorf("unable to create stripe session: %+v", err)
	}

	return session, nil
}
func CreateSession(stripeKey string, jobRq *database.JobRq, jobToken string) (*stripe.CheckoutSession, error) {
	if !isApplicable(jobRq) {
		return nil, nil
	}
	stripe.Key = stripeKey
	params := &stripe.CheckoutSessionParams{
		BillingAddressCollection: stripe.String("required"),
		PaymentMethodTypes: stripe.StringSlice([]string{
			"card",
		}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			&stripe.CheckoutSessionLineItemParams{
				Name:     stripe.String("Golang Cafe Sponsored Ad"),
				Amount:   stripe.Int64(AdTypeToAmount(jobRq.AdType)),
				Currency: stripe.String(strings.ToLower(jobRq.CurrencyCode)),
				Quantity: stripe.Int64(1),
			},
		},
		SuccessURL:    stripe.String(fmt.Sprintf("https://golang.cafe/edit/%s?payment=1&callback=1", jobToken)),
		CancelURL:     stripe.String(fmt.Sprintf("https://golang.cafe/edit/%s?payment=0&callback=1", jobToken)),
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
