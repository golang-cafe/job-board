package authoriser

import "github.com/0x13a/golang.cafe/pkg/config"

type Authoriser struct {
	AdminEmail    string
	AdminPassword string
}

type AuthRq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthRes struct {
	Email   string
	IsAdmin bool
	Valid   bool
}

func NewAuthoriser(cfg config.Config) Authoriser {
	return Authoriser{
		AdminEmail:    cfg.AdminEmail,
		AdminPassword: cfg.AdminPassword,
	}
}

func (a Authoriser) ValidAuthRequest(authRq *AuthRq) AuthRes {
	// todo: add proper auth
	if authRq.Email == a.AdminEmail && authRq.Password == a.AdminPassword {
		return AuthRes{Email: authRq.Email, Valid: true, IsAdmin: true}
	}
	return AuthRes{}
}
