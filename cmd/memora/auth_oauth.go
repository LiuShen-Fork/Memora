package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type oauthSettings struct {
	Name             string
	ClientID         string
	ClientSecret     string
	AuthorizationURL string
	TokenURL         string
	UserInfoURL      string
	Scope            string
}

func (a *App) oauthSettings() (oauthSettings, bool) {
	var enabled any
	if !a.readSetting("system", "auth.oauth.enabled", &enabled) || enabled != true {
		return oauthSettings{}, false
	}
	settings := oauthSettings{
		Name:             settingString(a, "system", "auth.oauth.name"),
		ClientID:         settingString(a, "system", "auth.oauth.clientId"),
		ClientSecret:     settingString(a, "system", "auth.oauth.clientSecret"),
		AuthorizationURL: settingString(a, "system", "auth.oauth.authorizationUrl"),
		TokenURL:         settingString(a, "system", "auth.oauth.tokenUrl"),
		UserInfoURL:      settingString(a, "system", "auth.oauth.userInfoUrl"),
		Scope:            settingString(a, "system", "auth.oauth.scope"),
	}
	if settings.Name == "" {
		settings.Name = "OAuth"
	}
	if settings.Scope == "" {
		settings.Scope = "openid profile email"
	}
	return settings, true
}

func (a *App) oauthAuth(w http.ResponseWriter, r *http.Request) {
	settings, enabled := a.oauthSettings()
	if !enabled {
		errorJSON(w, http.StatusForbidden, "OAuth login is disabled.")
		return
	}
	if settings.ClientID == "" || settings.ClientSecret == "" || settings.AuthorizationURL == "" || settings.TokenURL == "" || settings.UserInfoURL == "" {
		errorJSON(w, http.StatusInternalServerError, "OAuth configuration is incomplete")
		return
	}

	callback := oauthCallbackURL(r)
	if r.URL.Query().Get("code") == "" {
		params := url.Values{
			"client_id":     {settings.ClientID},
			"redirect_uri":  {callback},
			"response_type": {"code"},
			"scope":         {settings.Scope},
			"state":         {a.githubState()},
		}
		http.Redirect(w, r, settings.AuthorizationURL+"?"+params.Encode(), http.StatusFound)
		return
	}
	if !a.verifyGithubState(r.URL.Query().Get("state")) {
		errorJSON(w, http.StatusBadRequest, "Invalid OAuth state")
		return
	}

	token, err := exchangeOAuthCode(r.Context(), settings, r.URL.Query().Get("code"), callback)
	if err != nil {
		errorJSON(w, http.StatusUnauthorized, "OAuth token exchange failed")
		return
	}
	profile, err := fetchOAuthProfile(r.Context(), settings.UserInfoURL, token)
	if err != nil {
		errorJSON(w, http.StatusUnauthorized, "Unable to read OAuth profile")
		return
	}

	email := profileString(profile, "email")
	if email == "" {
		errorJSON(w, http.StatusForbidden, "OAuth account does not expose an email")
		return
	}
	var id int64
	var admin int
	err = a.db.QueryRow(`SELECT id,is_admin FROM users WHERE email=?`, email).Scan(&id, &admin)
	if err != nil {
		name := profileString(profile, "name", "preferred_username", "username", "login")
		subject := profileString(profile, "sub", "id", "user_id")
		name = a.uniqueOAuthUsername(name, subject)
		result, insertErr := a.db.Exec(`INSERT INTO users(name,email,avatar,created_at,is_admin) VALUES(?,?,?,?,0)`, name, email, profileString(profile, "picture", "avatar_url", "avatar"), time.Now().Unix())
		if insertErr != nil {
			errorJSON(w, http.StatusInternalServerError, "Unable to create OAuth account")
			return
		}
		id, _ = result.LastInsertId()
	}
	if admin != 1 {
		errorJSON(w, http.StatusForbidden, "Access denied. Please contact the administrator to activate your account.")
		return
	}
	a.setSession(w, id)
	http.Redirect(w, r, "/", http.StatusFound)
}

func oauthCallbackURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
	}
	return scheme + "://" + r.Host + r.URL.Path
}

func exchangeOAuthCode(ctx context.Context, settings oauthSettings, code, callback string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {settings.ClientID},
		"client_secret": {settings.ClientSecret},
		"code":          {code},
		"redirect_uri":  {callback},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, settings.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := externalHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if json.Unmarshal(body, &result) != nil {
		values, parseErr := url.ParseQuery(string(body))
		if parseErr == nil {
			result.AccessToken = values.Get("access_token")
			result.Error = values.Get("error")
		}
	}
	if resp.StatusCode >= 300 || result.AccessToken == "" {
		return "", fmt.Errorf("oauth token exchange: %s", result.Error)
	}
	return result.AccessToken, nil
}

func fetchOAuthProfile(ctx context.Context, endpoint, token string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Memora/1.0")
	resp, err := externalHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("oauth userinfo: %s", resp.Status)
	}
	var profile map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&profile); err != nil {
		return nil, err
	}
	for _, key := range []string{"user", "data"} {
		if nested, ok := profile[key].(map[string]any); ok {
			return nested, nil
		}
	}
	return profile, nil
}

func profileString(profile map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := profile[key]; ok && value != nil {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func (a *App) uniqueOAuthUsername(name, subject string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "oauth-user"
	}
	var count int
	_ = a.db.QueryRow(`SELECT count(*) FROM users WHERE name=?`, name).Scan(&count)
	if count == 0 {
		return name
	}
	digest := sha256.Sum256([]byte(subject))
	return fmt.Sprintf("%s-%s", name, hex.EncodeToString(digest[:4]))
}
