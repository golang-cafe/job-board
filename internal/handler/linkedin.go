package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/golang-cafe/job-board/internal/linkedin"
	"github.com/golang-cafe/job-board/internal/middleware"
	"github.com/golang-cafe/job-board/internal/server"
	"github.com/segmentio/ksuid"
	"golang.org/x/oauth2"
)

const sessionName = "linkedin"
const oauthStateKey = "linkedin_oauth_state"

func LinkedInAuthManage(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			linkedIn := linkedin.NewFromServer(svr)
			tokenJson, err := svr.MetaRepo.GetValue(linkedin.MetaToken)
			if err != nil {
				svr.Log(err, "GetValue for LinkedIn MetaToken")
			}

			token := oauth2.Token{}
			user := &linkedin.LinkedInUser{}
			if tokenJson != "" {
				err = json.Unmarshal([]byte(tokenJson), &token)
				if err != nil {
					svr.Log(err, "Unmarshalling LinkedIn token")
				}

				if token.AccessToken != "" {
					user, err = linkedIn.GetUser(context.Background(), &token)
					if err != nil {
						svr.Log(err, "LinkedIn GetUser")
					}
				}
			}

			err = svr.Render(r, w, http.StatusOK, "manage-linkedin.html", map[string]interface{}{
				"LinkedInUser": user,
				"Token":        token,
			})
			if err != nil {
				svr.Log(err, "Error rendering LinkedInAuthManage")
			}
		})
}

func LinkedInAuthInit(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			// Generate a oauth state token, to guard against CSRF
			session, err := svr.SessionStore.Get(r, sessionName)
			if err != nil {
				svr.Log(err, "SessionStore.Get")
			}

			// Generate a unique state token
			oauthState := ksuid.New()
			session.Values[oauthStateKey] = oauthState.String()
			err = session.Save(r, w)
			if err != nil {
				svr.Log(err, "Error saving session")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}

			linkedIn := linkedin.NewFromServer(svr)
			authCodeUrl := linkedIn.AuthCodeURL(oauthState.String())
			svr.Redirect(w, r, http.StatusTemporaryRedirect, authCodeUrl)
		})
}

func LinkedInAuthCallback(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			linkedIn := linkedin.NewFromServer(svr)

			session, err := svr.SessionStore.Get(r, sessionName)
			if err != nil {
				svr.Log(err, "SessionStore.Get")
			}
			oauthState, ok := session.Values[oauthStateKey]
			state := r.URL.Query().Get("state")

			if !ok || state != oauthState {
				svr.JSON(w, http.StatusForbidden, nil)
				return
			}

			code := r.URL.Query().Get("code")
			token, err := linkedIn.Exchange(context.Background(), code)
			if err != nil {
				svr.Log(err, "LinkedIn Exchange")
				svr.JSON(w, http.StatusInternalServerError, nil)
				return
			}

			tokenJson, err := json.Marshal(token)
			if err != nil {
				svr.Log(err, "Marshalling LinkedIn token")
			}

			err = svr.MetaRepo.SetValue(linkedin.MetaToken, string(tokenJson))
			if err != nil {
				svr.Log(err, "SetValue for LinkedIn MetaToken")
			}

			svr.Redirect(w, r, http.StatusTemporaryRedirect, "/manage/linkedin")
		})
}

func LinkedInAuthDisconnect(svr server.Server) http.HandlerFunc {
	return middleware.AdminAuthenticatedMiddleware(
		svr.SessionStore,
		svr.GetJWTSigningKey(),
		func(w http.ResponseWriter, r *http.Request) {
			err := svr.MetaRepo.Delete(linkedin.MetaToken)
			if err != nil {
				svr.Log(err, "Delete for LinkedIn MetaToken")
			}

			svr.Redirect(w, r, http.StatusTemporaryRedirect, "/manage/linkedin")
		})
}
