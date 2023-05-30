package email

import (
	"encoding/base64"
	"net/smtp"
	"fmt"
	"log"
	"net/http"
)

type Client struct {
	senderAddress  string
	noReplyAddress string
	siteName       string
	client         http.Client
	smtpUser       string
	smtpPassword   string
	smtpHost       string
	baseURL        string
	isLocal        bool
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
	Sender      Address   `json:"sender"`
	To          []Address `json:"to"`
	Subject     string    `json:"subject"`
	ReplyTo     Address   `json:"replyTo,omitempty"`
	TextContent string    `json:"textContent,omitempty"`
	HtmlContent string    `json:"htmlContent,omitempty"`
}

type EmailMessageWithAttachment struct {
	EmailMessage
	Attachment []Attachment `json:"attachment,omitempty"`
}

func NewClient(smtpUser, smtpPassword, smtpHost, senderAddress, noReplyAddress, siteName string, isLocal bool) (Client, error) {
	return Client{
		client:         *http.DefaultClient,
		smtpUser:       smtpUser,
		smtpPassword:   smtpPassword,
		smtpHost:       smtpHost,
		senderAddress:  senderAddress,
		siteName:       siteName,
		noReplyAddress: noReplyAddress,
		isLocal:        isLocal,
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
	if e.isLocal {
		log.Printf(
			"SendHTMLEmail: from: %v, to: %s, replyTo: %v, subject: %s, text: %s",
			from,
			to,
			replyTo,
			subject,
			text,
		)
		return nil
	}
	auth := smtp.PlainAuth("", e.smtpUser, e.smtpPassword, e.smtpHost)
	header := make(map[string]string)
	header["From"] = e.smtpUser
	header["To"] = to.Email
	header["Subject"] = subject
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/plain; charset=\"utf-8\""
	header["Content-Transfer-Encoding"] = "base64"
	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + base64.StdEncoding.EncodeToString([]byte(text))

	err := smtp.SendMail(e.smtpHost+":25", auth, e.smtpUser, []string{to.Email}, []byte(message))
	if err != nil {
		log.Println("error send mail", err.Error())
		return err
	}

	return nil
}
