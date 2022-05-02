package email

import (
	"encoding/base64"
	"fmt"

	sp "github.com/SparkPost/gosparkpost"
)

type Client struct {
	client        sp.Client
	senderAddress string
	siteName      string
}

func NewClient(apiKey, senderAddress, siteName string) (Client, error) {
	cfg := &sp.Config{
		BaseUrl:    "https://api.eu.sparkpost.com",
		ApiKey:     apiKey,
		ApiVersion: 1,
	}
	var client sp.Client
	err := client.Init(cfg)
	if err != nil {
		return Client{}, err
	}

	return Client{client: client, senderAddress: senderAddress, siteName: siteName}, nil
}

func (e Client) DefaultReplyTo() string {
	return e.senderAddress
}

func (e Client) DefaultSender() string {
	return fmt.Sprintf("%s <%s>", e.siteName, e.senderAddress)
}

func (e Client) DefaultAdminAddress() string {
	return e.senderAddress
}

func (e Client) SendEmail(from, to, replyTo, subject, text string) error {
	if replyTo == "" {
		replyTo = from
	}
	tx := &sp.Transmission{
		Recipients: []string{to},
		Content: sp.Content{
			Text:    text,
			From:    from,
			Subject: subject,
			ReplyTo: replyTo,
		},
		Options: &sp.TxOptions{
			TmplOptions: sp.TmplOptions{
				ClickTracking: new(bool),
				OpenTracking:  new(bool),
			},
		},
	}
	_, _, err := e.client.Send(tx)
	if err != nil {
		return err
	}
	return nil
}

func (e Client) SendHTMLEmail(from, to, replyTo, subject, text string) error {
	if replyTo == "" {
		replyTo = from
	}
	tx := &sp.Transmission{
		Recipients: []string{to},
		Content: sp.Content{
			HTML:    text,
			From:    from,
			Subject: subject,
			ReplyTo: replyTo,
		},
		Options: &sp.TxOptions{
			TmplOptions: sp.TmplOptions{
				ClickTracking: new(bool),
				OpenTracking:  new(bool),
			},
		},
	}
	_, _, err := e.client.Send(tx)
	if err != nil {
		return err
	}
	return nil
}

func (e Client) SendEmailWithPDFAttachment(from, to, replyTo, subject, text string, attachment []byte, fileName string) error {
	a := sp.Attachment{
		MIMEType: "application/pdf",
		Filename: fileName,
		B64Data:  base64.StdEncoding.EncodeToString(attachment),
	}
	tx := &sp.Transmission{
		Recipients: []string{to},
		Content: sp.Content{
			Text:        text,
			From:        from,
			Subject:     subject,
			ReplyTo:     replyTo,
			Attachments: []sp.Attachment{a},
		},
		Options: &sp.TxOptions{
			TmplOptions: sp.TmplOptions{
				ClickTracking: new(bool),
				OpenTracking:  new(bool),
			},
		},
	}
	_, _, err := e.client.Send(tx)
	if err != nil {
		return err
	}
	return nil
}
