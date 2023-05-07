package linkedin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang-cafe/job-board/internal/server"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/linkedin"
)

func New(config Config) *LinkedInOAuth {
	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.RedirectURL,
		Scopes:       config.Scopes,
		Endpoint:     linkedin.Endpoint,
	}

	return &LinkedInOAuth{config: oauthConfig}
}

func NewFromServer(svr server.Server) *LinkedInOAuth {
	return New(Config{
		ClientID:     svr.GetConfig().LinkedInClientID,
		ClientSecret: svr.GetConfig().LinkedInClientSecret,
		RedirectURL:  fmt.Sprintf("%s%s/manage/linkedin/callback", svr.GetConfig().URLProtocol, svr.GetConfig().SiteHost),
		Scopes: []string{
			ScopeMemberSocial,
			ScopeLiteProfile,
		},
	})
}

func (lo *LinkedInOAuth) AuthCodeURL(state string) string {
	return lo.config.AuthCodeURL(state)
}

func (lo *LinkedInOAuth) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return lo.config.Exchange(ctx, code)
}

func (lo *LinkedInOAuth) GetUser(ctx context.Context, token *oauth2.Token) (*LinkedInUser, error) {
	client := lo.config.Client(ctx, token)

	resp, err := client.Get(userInfoURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to fetch user info")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var user LinkedInUser
	if err = json.Unmarshal(body, &user); err != nil {
		return nil, err
	}

	return &user, nil
}
