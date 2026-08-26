package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type githubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func (a *App) githubAuth(w http.ResponseWriter, r *http.Request) {
	enabled := envBool("NUXT_PUBLIC_OAUTH_GITHUB_ENABLED", false)
	var configured any
	if a.readSetting("system", "auth.github.enabled", &configured) {
		if value, ok := configured.(bool); ok {
			enabled = enabled || value
		}
	}
	if !enabled {
		errorJSON(w, http.StatusForbidden, "GitHub OAuth login is disabled.")
		return
	}
	clientID := a.cfg.GithubClientID
	clientSecret := a.cfg.GithubClientSecret
	if value := settingString(a, "system", "auth.github.clientId"); value != "" {
		clientID = value
	}
	if value := settingString(a, "system", "auth.github.clientSecret"); value != "" {
		clientSecret = value
	}
	if clientID == "" || clientSecret == "" {
		errorJSON(w, http.StatusInternalServerError, "GitHub OAuth credentials are missing")
		return
	}
	if r.URL.Query().Get("code") == "" {
		callback := githubCallbackURL(r)
		state := a.githubState()
		params := url.Values{"client_id": {clientID}, "redirect_uri": {callback}, "scope": {"read:user user:email"}, "state": {state}}
		http.Redirect(w, r, "https://github.com/login/oauth/authorize?"+params.Encode(), http.StatusFound)
		return
	}
	if !a.verifyGithubState(r.URL.Query().Get("state")) {
		errorJSON(w, http.StatusBadRequest, "Invalid OAuth state")
		return
	}
	token, err := exchangeGithubCode(r.Context(), clientID, clientSecret, r.URL.Query().Get("code"), githubCallbackURL(r))
	if err != nil {
		errorJSON(w, http.StatusUnauthorized, "GitHub OAuth exchange failed")
		return
	}
	user, err := fetchGithubUser(r.Context(), token)
	if err != nil {
		errorJSON(w, http.StatusUnauthorized, "Unable to read GitHub profile")
		return
	}
	if user.Email == "" {
		user.Email, _ = fetchGithubPrimaryEmail(r.Context(), token)
	}
	if user.Email == "" {
		errorJSON(w, http.StatusForbidden, "GitHub account does not expose a verified email")
		return
	}
	var id int64
	var admin int
	err = a.db.QueryRow(`SELECT id,is_admin FROM users WHERE email=?`, user.Email).Scan(&id, &admin)
	if err != nil {
		name := user.Name
		if strings.TrimSpace(name) == "" {
			name = user.Login
		}
		name = a.uniqueUsername(name, user.ID)
		result, insertErr := a.db.Exec(`INSERT INTO users(name,email,avatar,created_at,is_admin) VALUES(?,?,?,?,0)`, name, user.Email, user.AvatarURL, time.Now().Unix())
		if insertErr != nil {
			errorJSON(w, http.StatusInternalServerError, "Unable to create GitHub account")
			return
		}
		id, _ = result.LastInsertId()
		admin = 0
	}
	if admin != 1 {
		errorJSON(w, http.StatusForbidden, "Access denied. Please contact the administrator to activate your account.")
		return
	}
	a.setSession(w, id)
	http.Redirect(w, r, "/", http.StatusFound)
}

func settingString(a *App, namespace, key string) string {
	var value any
	if a.readSetting(namespace, key, &value) {
		if result, ok := value.(string); ok {
			return result
		}
	}
	return ""
}

func (a *App) uniqueUsername(name string, githubID int64) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "github-user"
	}
	var count int
	_ = a.db.QueryRow(`SELECT count(*) FROM users WHERE name=?`, name).Scan(&count)
	if count == 0 {
		return name
	}
	return fmt.Sprintf("%s-%d", name, githubID)
}

func githubCallbackURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
	}
	return scheme + "://" + r.Host + r.URL.Path
}

func (a *App) githubState() string {
	payload := strconv.FormatInt(time.Now().Unix(), 10)
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, a.cfg.SessionKey)
	mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *App) verifyGithubState(state string) bool {
	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return false
	}
	mac := hmac.New(sha256.New, a.cfg.SessionKey)
	mac.Write([]byte(parts[0]))
	if !hmac.Equal(mac.Sum(nil), mustDecode(parts[1])) {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	created, err := strconv.ParseInt(string(payload), 10, 64)
	return err == nil && time.Since(time.Unix(created, 0)) >= 0 && time.Since(time.Unix(created, 0)) < 10*time.Minute
}

func exchangeGithubCode(ctx context.Context, clientID, clientSecret, code, callback string) (string, error) {
	form := url.Values{"client_id": {clientID}, "client_secret": {clientSecret}, "code": {code}, "redirect_uri": {callback}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
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
	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 || result.AccessToken == "" {
		return "", fmt.Errorf("github token exchange: %s", result.Error)
	}
	return result.AccessToken, nil
}

func fetchGithubUser(ctx context.Context, token string) (githubUser, error) {
	var user githubUser
	err := githubJSON(ctx, token, "https://api.github.com/user", &user)
	return user, err
}

func fetchGithubPrimaryEmail(ctx context.Context, token string) (string, error) {
	var emails []githubEmail
	if err := githubJSON(ctx, token, "https://api.github.com/user/emails", &emails); err != nil {
		return "", err
	}
	for _, email := range emails {
		if email.Primary && email.Verified {
			return email.Email, nil
		}
	}
	for _, email := range emails {
		if email.Verified {
			return email.Email, nil
		}
	}
	return "", nil
}

func githubJSON(ctx context.Context, token, endpoint string, output any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Memora/1.0")
	resp, err := externalHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("github api: %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(output)
}
