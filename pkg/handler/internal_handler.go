package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/0x13a/golang.cafe/pkg/database"
	"github.com/0x13a/golang.cafe/pkg/email"
	"github.com/0x13a/golang.cafe/pkg/imagemeta"
	"github.com/0x13a/golang.cafe/pkg/middleware"
	"github.com/0x13a/golang.cafe/pkg/payment"
	"github.com/0x13a/golang.cafe/pkg/server"
	"github.com/gorilla/mux"
	"github.com/nfnt/resize"
	"github.com/segmentio/ksuid"
)

var allowedMediaTypes = []string{"image/png", "image/jpeg", "image/jpg"}

func GenerateKsuIDPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := ksuid.NewRandom()
		if err != nil {
			svr.Render(w, http.StatusInternalServerError, "ksuid.html", map[string]string{"KSUID": ""})
			return
		}
		svr.Render(w, http.StatusOK, "ksuid.html", map[string]string{"KSUID": id.String()})
	}
}

func IPAddressLookup(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ipAddress := strings.Split(r.Header.Get("x-forwarded-for"), ", ")
		if len(ipAddress) < 1 {
			svr.Render(w, http.StatusInternalServerError, "whats-my-ip.html", map[string]string{"IPAddress": ""})
			return
		}
		svr.Render(w, http.StatusOK, "whats-my-ip.html", map[string]string{"IPAddress": ipAddress[0]})
	}
}

func DNSCheckerPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(w, http.StatusOK, "dns-checker.html", nil)
	}
}

func DNSChecker(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dnsType := r.URL.Query().Get("t")
		dnsHost := r.URL.Query().Get("h")
		switch dnsType {
		case "A":
			res, err := net.LookupIP(dnsHost)
			if err != nil || len(res) == 0 {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve A record")
				return
			}
			var buffer bytes.Buffer
			for _, ip := range res {
				buffer.WriteString(fmt.Sprintf("%s\n", ip.String()))
			}
			svr.TEXT(w, http.StatusOK, buffer.String())
		case "PTR":
			res, err := net.LookupAddr(dnsHost)
			if err != nil || len(res) == 0 {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve PTR record")
				return
			}
			var buffer bytes.Buffer
			for _, ptr := range res {
				buffer.WriteString(fmt.Sprintf("%s\n", ptr))
			}
			svr.TEXT(w, http.StatusOK, buffer.String())
		case "MX":
			res, err := net.LookupMX(dnsHost)
			if err != nil {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve CNAME record")
				return
			}
			var buffer bytes.Buffer
			for _, m := range res {
				buffer.WriteString(fmt.Sprintf("%s %v\n", m.Host, m.Pref))
			}
			svr.TEXT(w, http.StatusOK, buffer.String())
		case "CNAME":
			res, err := net.LookupCNAME(dnsHost)
			if err != nil {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve CNAME record")
				return
			}
			svr.TEXT(w, http.StatusOK, res)
		case "NS":
			res, err := net.LookupNS(dnsHost)
			if err != nil || len(res) == 0 {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve NS record")
				return
			}
			var buffer bytes.Buffer
			for _, ns := range res {
				buffer.WriteString(fmt.Sprintf("%s\n", ns.Host))
			}
			svr.TEXT(w, http.StatusOK, buffer.String())
		case "TXT":
			res, err := net.LookupTXT(dnsHost)
			if err != nil || len(res) == 0 {
				svr.TEXT(w, http.StatusInternalServerError, "unable to retrieve TXT record")
				return
			}
			var buffer bytes.Buffer
			for _, t := range res {
				buffer.WriteString(fmt.Sprintf("%s\n", t))
			}
			svr.TEXT(w, http.StatusOK, buffer.String())
		default:
			svr.TEXT(w, http.StatusInternalServerError, "invalid dns record type")
		}

	}
}

func PostAJobSuccessPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(w, http.StatusOK, "post-a-job-success.html", nil)
	}
}

func PostAJobFailurePageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		svr.Render(w, http.StatusOK, "post-a-job-error.html", nil)
	}
}

func ApplyForJobPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// limits upload form size to 5mb
		maxPdfSize := 5 * 1024 * 1024
		r.Body = http.MaxBytesReader(w, r.Body, int64(maxPdfSize))
		cv, header, err := r.FormFile("cv")
		if err != nil {
			svr.Log(err, "unable to read cv file")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		defer cv.Close()
		fileBytes, err := ioutil.ReadAll(cv)
		if err != nil {
			svr.Log(err, "unable to read cv file content")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		contentType := http.DetectContentType(fileBytes)
		if contentType != "application/pdf" {
			svr.Log(errors.New("PDF file is not application/pdf"), fmt.Sprintf("PDF file is not application/json got %s", contentType))
			svr.JSON(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		if header.Size > int64(maxPdfSize) {
			svr.Log(errors.New("PDF file is too large"), fmt.Sprintf("PDF file too large: %d > %d", header.Size, maxPdfSize))
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		externalID := r.FormValue("job-id")
		emailAddr := r.FormValue("email")
		job, err := database.JobPostByExternalIDForEdit(svr.Conn, externalID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job by externalId %d, %v", externalID, err))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		randomToken, err := k.Value()
		if err != nil {
			svr.Log(err, "unable to get token value")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		randomTokenStr, ok := randomToken.(string)
		if !ok {
			svr.Log(err, "unable to assert token value as string")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err = database.ApplyToJob(svr.Conn, job.ID, fileBytes, emailAddr, randomTokenStr)
		if err != nil {
			svr.Log(err, "unable to apply for job while saving to db")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err = svr.GetEmail().SendEmail("Diego from Golang Cafe <team@golang.cafe>", emailAddr, email.GolangCafeEmailAddress, fmt.Sprintf("Confirm your job application with %s", job.Company), fmt.Sprintf("Thanks for applying for the position %s with %s - %s (https://golang.cafe/job/%s). You application request, your email and your CV will expire in 72 hours and will be permanently deleted from the system. Please confirm your application now by following this link https://golang.cafe/apply/%s", job.JobTitle, job.Company, job.Location, job.Slug, randomTokenStr))
		if err != nil {
			svr.Log(err, "unable to send email while applying to job")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		if r.FormValue("notify-jobs") == "true" {
			if err := svr.SaveSubscriber(emailAddr); err != nil {
				svr.Log(err, fmt.Sprintf("unable to save subscriber while saving job application %v", err))
			}
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func ApplyToJobConfirmation(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		token := vars["token"]
		job, applicant, err := database.GetJobByApplyToken(svr.Conn, token)
		if err != nil {
			svr.Render(w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Invalid Job Application",
				"Description": "Oops, seems like the application you are trying to complete is no longer valid. Your application request may be expired or simply the company may not be longer accepting applications.",
			})
			return
		}
		err = svr.GetEmail().SendEmailWithPDFAttachment("Diego from Golang Cafe <team@golang.cafe>", job.HowToApply, applicant.Email, "New Applicant from Golang Cafe", fmt.Sprintf("Hi, there is a new applicant for your position on Golang Cafe: %s with %s - %s (https://golang.cafe/job/%s). Applicant's Email: %s. Please find applicant's CV attached below", job.JobTitle, job.Company, job.Location, job.Slug, applicant.Email), applicant.Cv, "cv.pdf")
		if err != nil {
			svr.Log(err, "unable to send email while applying to job")
			svr.Render(w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Job Application Failure",
				"Description": "Oops, there was a problem while completing yuor application. Please try again later. If the problem persists, please contact team@golang.cafe",
			})
			return
		}
		err = database.ConfirmApplyToJob(svr.Conn, token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to update apply_token with successfull application for token %s", token))
			svr.Render(w, http.StatusBadRequest, "apply-message.html", map[string]interface{}{
				"Title":       "Job Application Failure",
				"Description": "Oops, there was a problem while completing yuor application. Please try again later. If the problem persists, please contact team@golang.cafe",
			})
			return
		}
		svr.Render(w, http.StatusOK, "apply-message.html", map[string]interface{}{
			"Title":       "Job Application Successfull",
			"Description": svr.StringToHTML(fmt.Sprintf("Thank you for applying for <b>%s with %s - %s</b><br /><a href=\"https://golang.cafe/job/%s\">https://golang.cafe/job/%s</a>. <br /><br />Your CV has been forwarded to company HR. If you have further questions please reach out to <code>%s</code>. Please note, your email and CV have been permanently deleted from our systems.", job.JobTitle, job.Company, job.Location, job.Slug, job.Slug, job.HowToApply)),
		})
	}
}

func SubmitJobPostWithoutPaymentHandler(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			jobRq := &database.JobRq{}
			if err := decoder.Decode(&jobRq); err != nil {
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			jobID, err := database.SaveDraft(svr.Conn, jobRq)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to save job request: %#v", jobRq))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			k, err := ksuid.NewRandom()
			if err != nil {
				svr.Log(err, "unable to generate unique token")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			randomToken, err := k.Value()
			if err != nil {
				svr.Log(err, "unable to get token value")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			randomTokenStr, ok := randomToken.(string)
			if !ok {
				svr.Log(err, "unbale to assert token value as string")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			err = database.SaveTokenForJob(svr.Conn, randomTokenStr, jobID)
			if err != nil {
				svr.Log(err, "unable to generate token")
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			svr.JSON(w, http.StatusOK, map[string]interface{}{"token": randomTokenStr})
			return
		},
	)
}

func SubmitJobPostPaymentUpsellPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		jobRq := &database.JobRqUpsell{}
		if err := decoder.Decode(&jobRq); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		// validate currency
		if jobRq.CurrencyCode != "USD" && jobRq.CurrencyCode != "EUR" && jobRq.CurrencyCode != "GBP" {
			jobRq.CurrencyCode = "USD"
		}
		jobID, err := database.JobPostIDByToken(svr.Conn, jobRq.Token)
		if err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		sess, err := payment.CreateSession(svr.GetConfig().StripeKey, &database.JobRq{AdType: jobRq.AdType, CurrencyCode: jobRq.CurrencyCode, Email: jobRq.Email}, jobRq.Token)
		if err != nil {
			svr.Log(err, "unable to create payment session")
		}

		err = svr.GetEmail().SendEmail("Diego from Golang Cafe <team@golang.cafe>", email.GolangCafeEmailAddress, jobRq.Email, "New Upgrade on Golang Cafe", fmt.Sprintf("Hey! There is a new ad upgrade on Golang Cafe. Please check https://golang.cafe/manage/%s", jobRq.Token))
		if err != nil {
			svr.Log(err, "unable to send email to admin while upgrading job ad")
		}
		if sess != nil {
			err = database.InitiatePaymentEvent(svr.Conn, sess.ID, payment.AdTypeToAmount(jobRq.AdType), jobRq.CurrencyCode, payment.AdTypeToDescription(jobRq.AdType), jobRq.AdType, jobRq.Email, jobID)
			if err != nil {
				svr.Log(err, "unable to save payment initiated event")
			}
			svr.JSON(w, http.StatusOK, map[string]string{"s_id": sess.ID})
			return
		}
		svr.JSON(w, http.StatusOK, nil)
		return
	}
}

func GeneratePaymentIntent(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dec := json.NewDecoder(r.Body)
		req := struct {
			Email    string
			Currency string
			Amount   int
		}{}
		if err := dec.Decode(&req); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			fmt.Println("invalid req")
			return
		}
		if req.Currency != "USD" && req.Currency != "EUR" && req.Currency != "GBP" {
			fmt.Println("invalid cur")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		sess, err := payment.CreateGenericSession(svr.GetConfig().StripeKey, req.Email, req.Currency, req.Amount)
		if err != nil {
			fmt.Println("invalid sess")
			svr.Log(err, "unable to create payment session")
		}
		if sess != nil {
			fmt.Println("invalid req")
			svr.JSON(w, http.StatusOK, map[string]string{"s_id": sess.ID})
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func SubmitJobPostPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		jobRq := &database.JobRq{}
		if err := decoder.Decode(&jobRq); err != nil {
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		// validate currency
		if jobRq.CurrencyCode != "USD" && jobRq.CurrencyCode != "EUR" && jobRq.CurrencyCode != "GBP" {
			jobRq.CurrencyCode = "USD"
		}
		jobID, err := database.SaveDraft(svr.Conn, jobRq)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to save job request: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		k, err := ksuid.NewRandom()
		if err != nil {
			svr.Log(err, "unable to generate unique token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		randomToken, err := k.Value()
		if err != nil {
			svr.Log(err, "unable to get token value")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		randomTokenStr, ok := randomToken.(string)
		if !ok {
			svr.Log(err, "unbale to assert token value as string")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		err = database.SaveTokenForJob(svr.Conn, randomTokenStr, jobID)
		if err != nil {
			svr.Log(err, "unbale to generate token")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		sess, err := payment.CreateSession(svr.GetConfig().StripeKey, jobRq, randomTokenStr)
		if err != nil {
			svr.Log(err, "unable to create payment session")
		}
		approveMsg := fmt.Sprintf("Hey! There is a new Ad on Golang Cafe. Please approve https://golang.cafe/manage/%s", randomTokenStr)
		if strings.TrimSpace(jobRq.Feedback) != "" {
			approveMsg = fmt.Sprintf("Hey! There is a new Ad on Golang Cafe. Please approve https://golang.cafe/manage/%s.\nFeedback: %s", randomTokenStr, jobRq.Feedback)
		}
		err = svr.GetEmail().SendEmail("Diego from Golang Cafe <team@golang.cafe>", email.GolangCafeEmailAddress, jobRq.Email, "New Job Ad on Golang Cafe", approveMsg)
		if err != nil {
			svr.Log(err, "unable to send email to admin while posting job ad")
		}
		if sess != nil {
			err = database.InitiatePaymentEvent(svr.Conn, sess.ID, payment.AdTypeToAmount(jobRq.AdType), jobRq.CurrencyCode, payment.AdTypeToDescription(jobRq.AdType), jobRq.AdType, jobRq.Email, jobID)
			if err != nil {
				svr.Log(err, "unable to save payment initiated event")
			}
			svr.JSON(w, http.StatusOK, map[string]string{"s_id": sess.ID})
			return
		}
		svr.JSON(w, http.StatusOK, nil)
		return
	}
}

func RetrieveMediaPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		mediaID := vars["id"]
		media, err := database.GetMediaByID(svr.Conn, mediaID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve media by ID: '%s'", mediaID))
			svr.MEDIA(w, http.StatusNotFound, media.Bytes, media.MediaType)
			return
		}
		height := r.URL.Query().Get("h")
		width := r.URL.Query().Get("w")
		if height == "" && width == "" {
			svr.MEDIA(w, http.StatusOK, media.Bytes, media.MediaType)
			return
		}
		he, err := strconv.Atoi(height)
		if err != nil {
			svr.MEDIA(w, http.StatusOK, media.Bytes, media.MediaType)
			return
		}
		wi, err := strconv.Atoi(width)
		if err != nil {
			svr.MEDIA(w, http.StatusOK, media.Bytes, media.MediaType)
			return
		}
		contentTypeInvalid := true
		for _, allowedMedia := range allowedMediaTypes {
			if allowedMedia == media.MediaType {
				contentTypeInvalid = false
			}
		}
		if contentTypeInvalid {
			svr.Log(errors.New("invalid media content type"), fmt.Sprintf("media file %s is not one of the allowed media types: %+v", media.MediaType, allowedMediaTypes))
			svr.JSON(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		decImage, _, err := image.Decode(bytes.NewReader(media.Bytes))
		if err != nil {
			svr.Log(err, "unable to decode image from bytes")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		m := resize.Resize(uint(wi), uint(he), decImage, resize.Lanczos3)
		resizeImageBuf := new(bytes.Buffer)
		switch media.MediaType {
		case "image/jpg", "image/jpeg":
			if err := jpeg.Encode(resizeImageBuf, m, nil); err != nil {
				svr.Log(err, "unable to encode resizeImage into jpeg")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		case "image/png":
			if err := png.Encode(resizeImageBuf, m); err != nil {
				svr.Log(err, "unable to encode resizeImage into png")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		default:
			svr.MEDIA(w, http.StatusOK, media.Bytes, media.MediaType)
			return
		}
		svr.MEDIA(w, http.StatusOK, resizeImageBuf.Bytes(), media.MediaType)
	}
}

func RetrieveMediaMetaPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		jobID := vars["id"]
		job, err := database.GetJobByExternalID(svr.Conn, jobID)
		if err != nil {
			svr.Log(err, "unable to retrieve job by external ID")
			svr.MEDIA(w, http.StatusNotFound, []byte{}, "image/png")
			return
		}
		media, err := imagemeta.GenerateImageForJob(job)
		mediaBytes, err := ioutil.ReadAll(media)
		if err != nil {
			svr.Log(err, "unable to generate media for job ID")
			svr.MEDIA(w, http.StatusNotFound, mediaBytes, "image/png")
			return
		}
		svr.MEDIA(w, http.StatusOK, mediaBytes, "image/png")
	}
}

func UpdateMediaPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var x, y, wi, he int
		var err error
		x, err = strconv.Atoi(r.URL.Query().Get("x"))
		if err != nil {
			x = 0
		}
		y, err = strconv.Atoi(r.URL.Query().Get("y"))
		if err != nil {
			y = 0
		}
		wi, err = strconv.Atoi(r.URL.Query().Get("w"))
		if err != nil {
			wi = 0
		}
		he, err = strconv.Atoi(r.URL.Query().Get("h"))
		if err != nil {
			he = 0
		}
		vars := mux.Vars(r)
		mediaID := vars["id"]
		// limits upload form size to 5mb
		maxMediaFileSize := 5 * 1024 * 1024
		r.Body = http.MaxBytesReader(w, r.Body, int64(maxMediaFileSize))
		imageFile, header, err := r.FormFile("image")
		if err != nil {
			svr.Log(err, "unable to read media file")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		defer imageFile.Close()
		fileBytes, err := ioutil.ReadAll(imageFile)
		if err != nil {
			svr.Log(err, "unable to read imageFile file content")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		contentType := http.DetectContentType(fileBytes)
		contentTypeInvalid := true
		for _, allowedMedia := range allowedMediaTypes {
			if allowedMedia == contentType {
				contentTypeInvalid = false
			}
		}
		if contentTypeInvalid {
			svr.Log(errors.New("invalid media content type"), fmt.Sprintf("media file %s is not one of the allowed media types: %+v", contentType, allowedMediaTypes))
			svr.JSON(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		if header.Size > int64(maxMediaFileSize) {
			svr.Log(errors.New("media file is too large"), fmt.Sprintf("media file too large: %d > %d", header.Size, maxMediaFileSize))
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		decImage, _, err := image.Decode(bytes.NewReader(fileBytes))
		if err != nil {
			svr.Log(err, "unable to decode image from bytes")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		min := decImage.Bounds().Dy()
		if decImage.Bounds().Dx() < min {
			min = decImage.Bounds().Dx()
		}
		if he == 0 || wi == 0 || he != wi {
			he = min
			wi = min
		}
		cutImage := decImage.(interface {
			SubImage(r image.Rectangle) image.Image
		}).SubImage(image.Rect(x, y, wi, he))
		cutImageBytes := new(bytes.Buffer)
		switch contentType {
		case "image/jpg", "image/jpeg":
			if err := jpeg.Encode(cutImageBytes, cutImage, nil); err != nil {
				svr.Log(err, "unable to encode cutImage into jpeg")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		case "image/png":
			if err := png.Encode(cutImageBytes, cutImage); err != nil {
				svr.Log(err, "unable to encode cutImage into png")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		default:
			svr.Log(errors.New("content type not supported for encoding"), fmt.Sprintf("content type %s not supported for encoding", contentType))
			svr.JSON(w, http.StatusInternalServerError, nil)
		}
		err = database.UpdateMedia(svr.Conn, database.Media{cutImageBytes.Bytes(), contentType}, mediaID)
		if err != nil {
			svr.Log(err, "unable to update media image to db")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func SaveMediaPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var x, y, wi, he int
		var err error
		x, err = strconv.Atoi(r.URL.Query().Get("x"))
		if err != nil {
			x = 0
		}
		y, err = strconv.Atoi(r.URL.Query().Get("y"))
		if err != nil {
			y = 0
		}
		wi, err = strconv.Atoi(r.URL.Query().Get("w"))
		if err != nil {
			wi = 0
		}
		he, err = strconv.Atoi(r.URL.Query().Get("h"))
		if err != nil {
			he = 0
		}
		// limits upload form size to 5mb
		maxMediaFileSize := 5 * 1024 * 1024
		allowedMediaTypes := []string{"image/png", "image/jpeg", "image/jpg"}
		r.Body = http.MaxBytesReader(w, r.Body, int64(maxMediaFileSize))
		cv, header, err := r.FormFile("image")
		if err != nil {
			svr.Log(err, "unable to read media file")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		defer cv.Close()
		fileBytes, err := ioutil.ReadAll(cv)
		if err != nil {
			svr.Log(err, "unable to read cv file content")
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		contentType := http.DetectContentType(fileBytes)
		contentTypeInvalid := true
		for _, allowedMedia := range allowedMediaTypes {
			if allowedMedia == contentType {
				contentTypeInvalid = false
			}
		}
		if contentTypeInvalid {
			svr.Log(errors.New("invalid media content type"), fmt.Sprintf("media file %s is not one of the allowed media types: %+v", contentType, allowedMediaTypes))
			svr.JSON(w, http.StatusUnsupportedMediaType, nil)
			return
		}
		if header.Size > int64(maxMediaFileSize) {
			svr.Log(errors.New("media file is too large"), fmt.Sprintf("media file too large: %d > %d", header.Size, maxMediaFileSize))
			svr.JSON(w, http.StatusRequestEntityTooLarge, nil)
			return
		}
		decImage, _, err := image.Decode(bytes.NewReader(fileBytes))
		if err != nil {
			svr.Log(err, "unable to decode image from bytes")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		min := decImage.Bounds().Dy()
		if decImage.Bounds().Dx() < min {
			min = decImage.Bounds().Dx()
		}
		if he == 0 || wi == 0 || wi != he {
			he = min
			wi = min
		}
		cutImage := decImage.(interface {
			SubImage(r image.Rectangle) image.Image
		}).SubImage(image.Rect(x, y, wi, he))
		cutImageBytes := new(bytes.Buffer)
		switch contentType {
		case "image/jpg", "image/jpeg":
			if err := jpeg.Encode(cutImageBytes, cutImage, nil); err != nil {
				svr.Log(err, "unable to encode cutImage into jpeg")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		case "image/png":
			if err := png.Encode(cutImageBytes, cutImage); err != nil {
				svr.Log(err, "unable to encode cutImage into png")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}
		default:
			svr.Log(errors.New("content type not supported for encoding"), fmt.Sprintf("content type %s not supported for encoding", contentType))
			svr.JSON(w, http.StatusInternalServerError, nil)
		}
		id, err := database.SaveMedia(svr.Conn, database.Media{cutImageBytes.Bytes(), contentType})
		if err != nil {
			svr.Log(err, "unable to save media image to db")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		svr.JSON(w, http.StatusOK, map[string]interface{}{"id": id})
	}
}

func UpdateJobPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		jobRq := &database.JobRqUpdate{}
		if err := decoder.Decode(&jobRq); err != nil {
			svr.Log(err, fmt.Sprintf("unable to parse job request for update: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		jobID, err := database.JobPostIDByToken(svr.Conn, jobRq.Token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", jobRq.Token))
			svr.JSON(w, http.StatusNotFound, nil)
			return
		}
		err = database.UpdateJob(svr.Conn, jobRq, jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to update job request: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}
func PermanentlyDeleteJobByToken(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			jobRq := &database.JobRqUpdate{}
			if err := decoder.Decode(&jobRq); err != nil {
				svr.Log(err, fmt.Sprintf("unable to parse job request for delete: %#v", jobRq))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			jobID, err := database.JobPostIDByToken(svr.Conn, jobRq.Token)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", jobRq.Token))
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			err = database.DeleteJobCascade(svr.Conn, jobID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to permanently delete job: %#v", jobRq))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func ApproveJobPageHandler(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			jobRq := &database.JobRqUpdate{}
			if err := decoder.Decode(&jobRq); err != nil {
				svr.Log(err, fmt.Sprintf("unable to parse job request for update: %#v", jobRq))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			jobID, err := database.JobPostIDByToken(svr.Conn, jobRq.Token)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", jobRq.Token))
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			err = database.ApproveJob(svr.Conn, jobID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to update job request: %#v", jobRq))
				svr.JSON(w, http.StatusBadRequest, nil)
				return
			}
			err = svr.GetEmail().SendEmail("Diego from Golang Cafe <team@golang.cafe>", jobRq.Email, email.GolangCafeEmailAddress, "Your Job Ad on Golang Cafe", fmt.Sprintf("Thanks for using Golang Cafe,\nYour Job Ad has been approved and it's currently live on Golang Cafe - https://golang.cafe.\nYou can edit the Job Ad at any time and check page views and clickouts by following this link\nhttps://golang.cafe/edit/%s\nThe ad expires in 90 days and does not automatically renew. If you wish to sponsor or pin again the job ad you can do so by following the edit link.\nI am always available to answer any questions you may have,\nBest,\nDiego", jobRq.Token))
			if err != nil {
				svr.Log(err, "unable to send email while approving job ad")
			}
			svr.JSON(w, http.StatusOK, nil)
		},
	)
}

func DisapproveJobPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		jobRq := &database.JobRqUpdate{}
		if err := decoder.Decode(&jobRq); err != nil {
			svr.Log(err, fmt.Sprintf("unable to parse job request for update: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		jobID, err := database.JobPostIDByToken(svr.Conn, jobRq.Token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", jobRq.Token))
			svr.JSON(w, http.StatusNotFound, nil)
			return
		}
		err = database.DisapproveJob(svr.Conn, jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to update job request: %#v", jobRq))
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func TrackJobClickoutPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		externalID := vars["id"]
		if externalID == "" {
			svr.Log(errors.New("got empty id for tracking job"), "got empty externalID for tracking")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		job, err := database.GetJobByExternalID(svr.Conn, externalID)
		if err != nil {
			svr.Log(err, "unable to get JobID from externalID")
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		if err := database.TrackJobClickout(svr.Conn, job.ID); err != nil {
			svr.Log(err, fmt.Sprintf("unable to save job clickout for job id %d. %v", job.ID, err))
			svr.JSON(w, http.StatusOK, nil)
			return
		}
		svr.JSON(w, http.StatusOK, nil)
	}
}

func TrackJobClickoutAndRedirectToJobPage(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		externalID := r.URL.Query().Get("j")
		if externalID == "" {
			svr.Log(errors.New("TrackJobClickoutAndRedirectToJobPage: got empty id for tracking job"), "got empty externalID for tracking")
			svr.JSON(w, http.StatusBadRequest, nil)
			return
		}
		reg, _ := regexp.Compile("[^a-zA-Z0-9 ]+")
		job, err := database.GetJobByExternalID(svr.Conn, reg.ReplaceAllString(externalID, ""))
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to get HowToApply from externalID %s", externalID))
			svr.JSON(w, http.StatusInternalServerError, nil)
			return
		}
		if err := database.TrackJobClickout(svr.Conn, job.ID); err != nil {
			svr.Log(err, fmt.Sprintf("unable to save job clickout for job id %d. %v", job.ID, err))
			svr.JSON(w, http.StatusOK, nil)
			return
		}
		svr.Redirect(w, r, http.StatusTemporaryRedirect, job.HowToApply)
	}
}

func EditJobViewPageHandler(svr server.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		token := vars["token"]
		isCallback := r.URL.Query().Get("callback")
		paymentSuccess := r.URL.Query().Get("payment")
		expiredUpsell := r.URL.Query().Get("expired")
		jobID, err := database.JobPostIDByToken(svr.Conn, token)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", token))
			svr.JSON(w, http.StatusNotFound, nil)
			return
		}
		job, err := database.JobPostByIDForEdit(svr.Conn, jobID)
		if err != nil || job == nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job by ID %d", jobID))
			svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job for golang.cafe/edit/%s not found", token))
			return
		}
		clickoutCount, err := database.GetClickoutCountForJob(svr.Conn, jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job clickout count for job id %d", jobID))
		}
		viewCount, err := database.GetViewCountForJob(svr.Conn, jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job view count for job id %d", jobID))
		}
		conversionRate := ""
		if clickoutCount > 0 && viewCount > 0 {
			conversionRate = fmt.Sprintf("%.2f", float64(float64(clickoutCount)/float64(viewCount)*100))
		}
		purchaseEvents, err := database.GetPurchaseEvents(svr.Conn, jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve job payment events for job id %d", jobID))
		}
		stats, err := database.GetStatsForJob(svr.Conn, jobID)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to retrieve stats for job id %d", jobID))
		}
		statsSet, err := json.Marshal(stats)
		if err != nil {
			svr.Log(err, fmt.Sprintf("unable to marshal stats for job id %d", jobID))
		}
		currency, err := svr.GetCurrencyFromRequest(r)
		if err != nil {
			svr.Log(err, "could not find ip address in x-forwarded-for, defaulting currency to USD")
		}
		svr.Render(w, http.StatusOK, "edit.html", map[string]interface{}{
			"Job":                        job,
			"Stats":                      string(statsSet),
			"Purchases":                  purchaseEvents,
			"JobPerksEscaped":            svr.JSEscapeString(job.Perks),
			"JobInterviewProcessEscaped": svr.JSEscapeString(job.InterviewProcess),
			"JobDescriptionEscaped":      svr.JSEscapeString(job.JobDescription),
			"Token":                      token,
			"ViewCount":                  viewCount,
			"ClickoutCount":              clickoutCount,
			"ConversionRate":             conversionRate,
			"IsCallback":                 isCallback,
			"PaymentSuccess":             paymentSuccess,
			"IsUpsell":                   expiredUpsell,
			"Currency":                   currency,
			"StripePublishableKey":       svr.GetConfig().StripePublishableKey,
			"IsUnpinned":                 job.AdType != database.JobAdSponsoredPinnedFor30Days && job.AdType != database.JobAdSponsoredPinnedFor30Days,
		})
	}
}

func ManageJobBySlugViewPageHandler(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			slug := vars["slug"]
			jobPost, err := database.JobPostBySlugAdmin(svr.Conn, slug)
			if err != nil {
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			jobPostToken, err := database.TokenByJobID(svr.Conn, jobPost.ID)
			if err != nil {
				svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job for golang.cafe/manage/job/%s not found", slug))
				return
			}
			svr.Redirect(w, r, http.StatusMovedPermanently, fmt.Sprintf("/manage/%s", jobPostToken))
		},
	)
}

func ManageJobViewPageHandler(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			vars := mux.Vars(r)
			token := vars["token"]
			jobID, err := database.JobPostIDByToken(svr.Conn, token)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to find job post ID by token: %s", token))
				svr.JSON(w, http.StatusNotFound, nil)
				return
			}
			job, err := database.JobPostByIDForEdit(svr.Conn, jobID)
			if err != nil || job == nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve job by ID %d", jobID))
				svr.JSON(w, http.StatusNotFound, fmt.Sprintf("Job for golang.cafe/edit/%s not found", token))
				return
			}
			clickoutCount, err := database.GetClickoutCountForJob(svr.Conn, jobID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve job clickout count for job id %d", jobID))
			}
			viewCount, err := database.GetViewCountForJob(svr.Conn, jobID)
			if err != nil {
				svr.Log(err, fmt.Sprintf("unable to retrieve job view count for job id %d", jobID))
			}
			conversionRate := ""
			if clickoutCount > 0 && viewCount > 0 {
				conversionRate = fmt.Sprintf("%.2f", float64(float64(clickoutCount)/float64(viewCount)*100))
			}
			svr.Render(w, http.StatusOK, "manage.html", map[string]interface{}{
				"Job":                        job,
				"JobPerksEscaped":            svr.JSEscapeString(job.Perks),
				"JobInterviewProcessEscaped": svr.JSEscapeString(job.InterviewProcess),
				"JobDescriptionEscaped":      svr.JSEscapeString(job.JobDescription),
				"Token":                      token,
				"ViewCount":                  viewCount,
				"ClickoutCount":              clickoutCount,
				"ConversionRate":             conversionRate,
			})
		},
	)
}
