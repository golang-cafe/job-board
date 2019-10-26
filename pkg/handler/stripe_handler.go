package handler

import (
	"net/http"
	"log"
	"io/ioutil"

	"github.com/0x13a/golang.cafe/pkg/server"
	"github.com/0x13a/golang.cafe/pkg/payment"
)

func StripePaymentConfirmationWebookHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		const MaxBodyBytes = int64(65536)
		req.Body = http.MaxBytesReader(w, req.Body, MaxBodyBytes)
		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			svr.Log(err, "error reading request body from stripe")
			svr.JSON(w, http.StatusServiceUnavailable, nil)
			return
		}

		stripeSig := req.Header.Get("Stripe-Signature")
		sess, err := payment.HandleCheckoutSessionComplete(body, svr.GetConfig().StripeEndpointSecret, stripeSig)
		if err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		if sess != nil {
			log.Printf("session: %+v\n", sess)
			// fulfil product
			// retrieve job by session id
			// retrieve job token by session id
			// send email "thanks for your payment"
			// mark job as paid
			svr.JSON(w, http.StatusOK, nil)
			return
		}

		svr.JSON(w, http.StatusOK, nil)
	}
}
