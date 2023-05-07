package linkedin

import (
	"golang.org/x/oauth2"
)

const (
	userInfoURL = "https://api.linkedin.com/v2/me"
	// postsURL    = "https://api.linkedin.com/v2/posts"

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
