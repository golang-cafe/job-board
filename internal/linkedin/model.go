package linkedin

import (
	"golang.org/x/oauth2"
)

const (
	userInfoURL = "https://api.linkedin.com/v2/me"
	postsURL    = "https://api.linkedin.com/v2/ugcPosts"

	ScopeMemberSocial = "w_member_social" // Create, modify, and delete posts, comments, and reactions on your behalf
	ScopeLiteProfile  = "r_liteprofile"   // Use your name and photo

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

type ShareMediaCategory string

const (
	ShareMediaCategoryNone    ShareMediaCategory = "NONE"
	ShareMediaCategoryArticle ShareMediaCategory = "ARTICLE"
	ShareMediaCategoryImage   ShareMediaCategory = "IMAGE"
)

type ShareCommentary struct {
	Text string `json:"text"`
}

type ShareContent struct {
	ShareCommentary    ShareCommentary    `json:"shareCommentary"`
	ShareMediaCategory ShareMediaCategory `json:"shareMediaCategory"`
}

type SpecificContent struct {
	ShareContent ShareContent `json:"com.linkedin.ugc.ShareContent"`
}

type VisibilityStruct struct {
	Visibility Visibility `json:"com.linkedin.ugc.MemberNetworkVisibility"`
}

type PostRequest struct {
	Author          string           `json:"author"`
	LifecycleState  LifecycleState   `json:"lifecycleState"`
	SpecificContent SpecificContent  `json:"specificContent"`
	Visibility      VisibilityStruct `json:"visibility"`
}
