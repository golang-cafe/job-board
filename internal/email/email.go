package email

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

type Client struct {
	senderAddress  string
	noReplyAddress string
	siteName       string
	client         http.Client
	apiKey         string
	baseURL        string
}

type Attachment struct {
	Name    string `json:"name"`
	B64Data string `json:"content"`
}

type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

type EmailMessage struct {
	Sender      Address     `json:"sender"`
	To          []Address   `json:"to"`
	Subject     string      `json:"subject"`
	ReplyTo     Address     `json:"replyTo,omitempty"`
	TextContent string      `json:"textContent,omitempty"`
	HtmlContent string      `json:"htmlContent,omitempty"`
	Attachment  *Attachment `json:"attachment,omitempty"`
}

func NewClient(apiKey, senderAddress, noReplyAddress, siteName string) (Client, error) {
	return Client{
		client:         *http.DefaultClient,
		apiKey:         apiKey,
		senderAddress:  senderAddress,
		siteName:       siteName,
		noReplyAddress: noReplyAddress,
		baseURL:        "https://api.sendinblue.com"}, nil
}

func (e Client) DefaultReplyTo() string {
	return e.senderAddress
}

func (e Client) DefaultSenderName() string {
	return e.siteName
}

func (e Client) SupportSenderAddress() string {
	return e.senderAddress
}

func (e Client) NoReplySenderAddress() string {
	return e.noReplyAddress
}

func (e Client) DefaultAdminAddress() string {
	return e.senderAddress
}

func (e Client) SendHTMLEmail(from, to, replyTo Address, subject, text string) error {
	msg := EmailMessage{
		Sender:      from,
		ReplyTo:     replyTo,
		Subject:     subject,
		To:          []Address{to},
		HtmlContent: text,
	}
	reqData, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, e.baseURL+"/v3/smtp/email", bytes.NewReader(reqData))
	if err != nil {
		return err
	}
	req.Header.Add("api-key", e.apiKey)
	req.Header.Add("content-type", "application/json")
	res, err := e.client.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode >= http.StatusBadRequest {
		errBody, err := ioutil.ReadAll(res.Body)
		if err != nil {
			errBody = []byte(`unable to read body`)
		}
		return errors.New(fmt.Sprintf("got status code %d when sending email: err %s", res.StatusCode, string(errBody)))
	}
	return nil
}

func (e Client) SendEmailWithPDFAttachment(from, to, replyTo Address, subject, text string, attachment []byte, fileName string) error {
	msg := EmailMessage{
		Sender:      from,
		ReplyTo:     replyTo,
		Subject:     subject,
		To:          []Address{to},
		HtmlContent: text,
		Attachment: &Attachment{
			Name:    fileName,
			B64Data: base64.StdEncoding.EncodeToString(attachment),
		},
	}
	reqData, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, e.baseURL+"/v3/smtp/email", bytes.NewReader(reqData))
	if err != nil {
		return err
	}
	req.Header.Add("api-key", e.apiKey)
	req.Header.Add("content-type", "application/json")
	res, err := e.client.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode >= http.StatusBadRequest {
		errBody, err := ioutil.ReadAll(res.Body)
		if err != nil {
			errBody = []byte(`unable to read body`)
		}
		return errors.New(fmt.Sprintf("got status code %d when sending email: err %s", res.StatusCode, string(errBody)))
	}
	return nil
}
