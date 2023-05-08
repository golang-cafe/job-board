package linkedin

import (
	"bytes"
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
			// ScopeOrganizationSocial,
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

func (lo *LinkedInOAuth) SharePost(ctx context.Context, token *oauth2.Token, authorUser *LinkedInUser, text string, visibility Visibility) error {
	// The version of the LinkedIn API this method was written against.
	// Update this when migrating to newer versions.
	linkedInVersion := "202304"

	client := lo.config.Client(ctx, token)

	postRequest := TextPostRequest{
		Author:     fmt.Sprintf("urn:li:person:%s", authorUser.ID),
		Commentary: text,
		Visibility: VisibilityPublic,
		Distribution: DistributionStruct{
			FeedDistribution: FeedDistributionMainFeed,
		},
		LifecycleState: LifecyclePublished,
	}

	postJson, err := json.Marshal(postRequest)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, "POST", postsURL, bytes.NewBuffer(postJson))
	if err != nil {
		return err
	}

	request.Header.Add("LinkedIn-Version", linkedInVersion)
	request.Header.Add("X-Restli-Protocol-Version", "2.0.0")
	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		return errors.New("Unable to create LinkedIn post")
	}

	return nil
}
