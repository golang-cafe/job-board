package linkedin

import (
	"golang.org/x/oauth2"
)

const (
	userInfoURL = "https://api.linkedin.com/v2/me"
	postsURL    = "https://api.linkedin.com/rest/posts"

	ScopeMemberSocial       = "w_member_social"       // Create, modify, and delete posts, comments, and reactions on your behalf
	ScopeOrganizationSocial = "w_organization_social" // Post, comment and like posts on behalf of an organization. Restricted to organizations in which the authenticated member has one of the following company page roles:, ADMINISTRATOR, DIRECT_SPONSORED_CONTENT_POSTER, CONTENT_ADMIN
	ScopeLiteProfile        = "r_liteprofile"         // Use your name and photo

	MetaToken = "linkedin_token"
)

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string
}

type LinkedInOAuth struct {
	config *oauth2.Config
}

type LinkedInUser struct {
	ID        string `json:"id"`
	FirstName string `json:"localizedFirstName"`
	LastName  string `json:"localizedLastName"`
}

type LifecycleState string

const (
	LifecyclePublished LifecycleState = "PUBLISHED"
)

type Visibility string

const (
	VisibilityPublic      Visibility = "PUBLIC"
	VisibilityConnections Visibility = "CONNECTIONS"
)

type FeedDistribution string

const (
	FeedDistributionNone     FeedDistribution = "NONE"
	FeedDistributionMainFeed FeedDistribution = "MAIN_FEED"
)

type DistributionStruct struct {
	FeedDistribution               FeedDistribution `json:"feedDistribution"`
	TargetEntities                 []interface{}    `json:"targetEntities,omitempty"`
	ThirdPartyDistributionChannels []interface{}    `json:"thirdPartyDistributionChannels,omitempty"`
}

type TextPostRequest struct {
	Author         string             `json:"author"`
	Commentary     string             `json:"commentary"` // type "little text"?
	Visibility     Visibility         `json:"visibility"`
	Distribution   DistributionStruct `json:"distribution"`
	LifecycleState LifecycleState     `json:"lifecycleState"`
}
