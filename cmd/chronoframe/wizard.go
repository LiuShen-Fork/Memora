package main

import (
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func (a *App) wizardRoute(w http.ResponseWriter, r *http.Request, rest string) {
	switch {
	case rest == "schema" && r.Method == http.MethodGet:
		a.wizardSchema(w, r)
	case rest == "admin" && r.Method == http.MethodPost:
		a.wizardAdmin(w, r)
	case rest == "site" && r.Method == http.MethodPost:
		a.wizardSite(w, r)
	case rest == "storage" && r.Method == http.MethodPost:
		a.wizardStorage(w, r)
	case rest == "map" && r.Method == http.MethodPost:
		a.wizardMap(w, r)
	case rest == "complete" && r.Method == http.MethodPost:
		a.setSetting("system", "firstLaunch", false)
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	case rest == "submit" && r.Method == http.MethodPost:
		a.wizardSubmit(w, r)
	default:
		errorJSON(w, http.StatusNotFound, "Not Found")
	}
}

func (a *App) wizardSchema(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	fields := wizardFields(namespace)
	if fields == nil {
		errorJSON(w, http.StatusNotFound, "Unknown wizard namespace")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"namespace": namespace, "fields": fields})
}

func wizardFields(namespace string) []map[string]any {
	field := func(key, typ, label string, value any, ui map[string]any) map[string]any {
		return map[string]any{"namespace": namespace, "key": key, "type": typ, "defaultValue": value, "value": value, "label": label, "ui": ui}
	}
	input := func(required bool, placeholder string) map[string]any {
		return map[string]any{"type": "input", "required": required, "placeholder": placeholder}
	}
	switch namespace {
	case "admin":
		return []map[string]any{
			field("username", "string", "wizard.admin.username.label", env("CFRAME_ADMIN_NAME", "admin"), input(true, "admin")),
			field("email", "string", "wizard.admin.email.label", env("CFRAME_ADMIN_EMAIL", ""), input(true, "admin@example.com")),
			field("password", "string", "wizard.admin.password.label", env("CFRAME_ADMIN_PASSWORD", ""), map[string]any{"type": "password", "required": true}),
			field("confirmPassword", "string", "wizard.admin.confirmPassword.label", env("CFRAME_ADMIN_PASSWORD", ""), map[string]any{"type": "password", "required": true}),
		}
	case "app":
		return []map[string]any{
			field("title", "string", "settings.app.title.label", env("NUXT_PUBLIC_APP_TITLE", "ChronoFrame"), input(true, "ChronoFrame")),
			field("slogan", "string", "settings.app.slogan.label", env("NUXT_PUBLIC_APP_SLOGAN", ""), input(false, "Your gallery slogan")),
			field("author", "string", "settings.app.author.label", env("NUXT_PUBLIC_APP_AUTHOR", ""), input(false, "Your name")),
			field("avatarUrl", "string", "settings.app.avatarUrl.label", env("NUXT_PUBLIC_APP_AVATAR_URL", ""), map[string]any{"type": "url", "required": false, "placeholder": "https://example.com/avatar.jpg"}),
		}
	case "storage":
		providerOptions := []map[string]any{{"label": "settings.storage.provider.options.local.label", "value": "local", "icon": "tabler:server"}, {"label": "settings.storage.provider.options.s3.label", "value": "s3", "icon": "tabler:brand-aws"}, {"label": "settings.storage.provider.options.openlist.label", "value": "openlist", "icon": "tabler:stack"}}
		visible := func(provider string, required bool, password bool) map[string]any {
			typeName := "input"
			if password {
				typeName = "password"
			}
			return map[string]any{"type": typeName, "required": required, "visibleIf": map[string]any{"fieldKey": "provider", "value": provider}}
		}
		return []map[string]any{
			field("provider", "string", "settings.storage.provider.label", "local", map[string]any{"type": "custom", "required": true, "options": providerOptions}),
			field("name", "string", "settings.storage.name.label", "Default Storage", input(true, "Default Storage")),
			field("local.basePath", "string", "settings.storage.local.basePath.label", "./data/storage", visible("local", true, false)),
			field("local.baseUrl", "string", "settings.storage.local.baseUrl.label", "/storage", visible("local", false, false)),
			field("local.prefix", "string", "settings.storage.local.prefix.label", "photos/", visible("local", false, false)),
			field("s3.endpoint", "string", "settings.storage.s3.endpoint.label", "", visible("s3", true, false)),
			field("s3.bucket", "string", "settings.storage.s3.bucket.label", "", visible("s3", true, false)),
			field("s3.region", "string", "settings.storage.s3.region.label", "auto", visible("s3", true, false)),
			field("s3.accessKeyId", "string", "settings.storage.s3.accessKeyId.label", "", visible("s3", true, false)),
			field("s3.secretAccessKey", "string", "settings.storage.s3.secretAccessKey.label", "", visible("s3", true, true)),
			field("s3.prefix", "string", "settings.storage.s3.prefix.label", "photos/", visible("s3", false, false)),
			field("s3.cdnUrl", "string", "settings.storage.s3.cdnUrl.label", "", visible("s3", false, false)),
			field("openlist.baseUrl", "string", "settings.storage.openlist.baseUrl.label", "", visible("openlist", true, false)),
			field("openlist.rootPath", "string", "settings.storage.openlist.rootPath.label", "/photos", visible("openlist", true, false)),
			field("openlist.token", "string", "settings.storage.openlist.token.label", "", visible("openlist", true, true)),
			field("openlist.cdnUrl", "string", "settings.storage.openlist.cdnUrl.label", "", visible("openlist", false, false)),
			field("openlist.uploadEndpoint", "string", "settings.storage.openlist.uploadEndpoint.label", "/api/fs/put", visible("openlist", false, false)),
			field("openlist.downloadEndpoint", "string", "settings.storage.openlist.downloadEndpoint.label", "", visible("openlist", false, false)),
			field("openlist.listEndpoint", "string", "settings.storage.openlist.listEndpoint.label", "", visible("openlist", false, false)),
			field("openlist.deleteEndpoint", "string", "settings.storage.openlist.deleteEndpoint.label", "/api/fs/remove", visible("openlist", false, false)),
			field("openlist.metaEndpoint", "string", "settings.storage.openlist.metaEndpoint.label", "/api/fs/get", visible("openlist", false, false)),
			field("openlist.pathField", "string", "settings.storage.openlist.pathField.label", "path", visible("openlist", false, false)),
		}
	case "map":
		providerOptions := []map[string]any{{"label": "MapBox", "value": "mapbox"}, {"label": "MapLibre", "value": "maplibre"}}
		visible := func(provider, typ string, required bool) map[string]any {
			return map[string]any{"type": typ, "required": required, "visibleIf": map[string]any{"fieldKey": "provider", "value": provider}}
		}
		return []map[string]any{
			field("provider", "string", "settings.map.provider.label", env("NUXT_PUBLIC_MAP_PROVIDER", "maplibre"), map[string]any{"type": "custom", "required": true, "options": providerOptions}),
			field("mapbox.token", "string", "settings.map.mapbox.token.label", env("NUXT_MAPBOX_ACCESS_TOKEN", ""), visible("mapbox", "password", true)),
			field("mapbox.style", "string", "settings.map.mapbox.style.label", env("NUXT_PUBLIC_MAP_MAPBOX_STYLE", ""), visible("mapbox", "input", false)),
			field("maplibre.token", "string", "settings.map.maplibre.token.label", "", visible("maplibre", "password", true)),
			field("maplibre.style", "string", "settings.map.maplibre.style.label", env("NUXT_PUBLIC_MAP_MAPLIBRE_STYLE", ""), visible("maplibre", "input", false)),
		}
	default:
		return nil
	}
}

func (a *App) wizardAdmin(w http.ResponseWriter, r *http.Request) {
	var body struct{ Email, Password, Username string }
	if decodeJSON(r, &body) != nil || !validAdmin(body.Email, body.Password, body.Username) {
		errorJSON(w, http.StatusBadRequest, "Invalid administrator details")
		return
	}
	if err := a.upsertWizardAdmin(body.Email, body.Password, body.Username); err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *App) wizardSite(w http.ResponseWriter, r *http.Request) {
	var site map[string]any
	if decodeJSON(r, &site) != nil || strings.TrimSpace(stringValue(site["title"])) == "" {
		errorJSON(w, http.StatusBadRequest, "Site title is required")
		return
	}
	a.writeSiteSettings(site)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *App) wizardStorage(w http.ResponseWriter, r *http.Request) {
	var body wizardStorageInput
	if decodeJSON(r, &body) != nil || strings.TrimSpace(body.Name) == "" || !validStorageConfig(stringValue(body.Config["provider"]), body.Config) {
		errorJSON(w, http.StatusBadRequest, "Invalid storage configuration")
		return
	}
	id, err := a.insertStorageConfig(body.Name, stringValue(body.Config["provider"]), body.Config)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to save storage configuration")
		return
	}
	a.setSetting("storage", "provider", id)
	a.storage = a.loadStorage()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "id": id})
}

func (a *App) wizardMap(w http.ResponseWriter, r *http.Request) {
	var body wizardMapInput
	if decodeJSON(r, &body) != nil || !validMap(body) {
		errorJSON(w, http.StatusBadRequest, "Invalid map configuration")
		return
	}
	a.writeMapSettings(body)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

type wizardStorageInput struct {
	Name   string         `json:"name"`
	Config map[string]any `json:"config"`
}

type wizardMapInput struct {
	Provider string `json:"provider"`
	Token    string `json:"token"`
	Style    string `json:"style"`
}

func (a *App) wizardSubmit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Admin   struct{ Email, Password, Username string } `json:"admin"`
		Site    map[string]any                             `json:"site"`
		Storage struct {
			Name   string         `json:"name"`
			Config map[string]any `json:"config"`
		} `json:"storage"`
		Map wizardMapInput `json:"map"`
	}
	if decodeJSON(r, &body) != nil || !validAdmin(body.Admin.Email, body.Admin.Password, body.Admin.Username) || strings.TrimSpace(stringValue(body.Site["title"])) == "" {
		errorJSON(w, http.StatusBadRequest, "Invalid setup data")
		return
	}
	provider := stringValue(body.Storage.Config["provider"])
	if strings.TrimSpace(body.Storage.Name) == "" || !validStorageConfig(provider, body.Storage.Config) || !validMap(body.Map) {
		errorJSON(w, http.StatusBadRequest, "Invalid setup data")
		return
	}
	if err := a.upsertWizardAdmin(body.Admin.Email, body.Admin.Password, body.Admin.Username); err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	a.writeSiteSettings(body.Site)
	id, err := a.insertStorageConfig(body.Storage.Name, provider, body.Storage.Config)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to save storage configuration")
		return
	}
	a.setSetting("storage", "provider", id)
	a.writeMapSettings(body.Map)
	a.setSetting("system", "firstLaunch", false)
	a.storage = a.loadStorage()

	var userID int64
	if err := a.db.QueryRow(`SELECT id FROM users WHERE email=?`, body.Admin.Email).Scan(&userID); err == nil {
		a.setSession(w, userID)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *App) upsertWizardAdmin(email, password, username string) error {
	var existingEmail string
	err := a.db.QueryRow(`SELECT email FROM users ORDER BY id LIMIT 1`).Scan(&existingEmail)
	if err == nil && existingEmail != email {
		return errWizardUserExists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = a.db.Exec(`INSERT INTO users(name,email,password,is_admin,created_at) VALUES(?,?,?,?,unixepoch()) ON CONFLICT(email) DO UPDATE SET name=excluded.name,password=excluded.password,is_admin=1`, username, email, string(hash), 1)
	return err
}

func (a *App) writeSiteSettings(site map[string]any) {
	for _, key := range []string{"title", "slogan", "avatarUrl", "author"} {
		if value, ok := site[key]; ok {
			a.setSetting("app", key, value)
		}
	}
}

func (a *App) writeMapSettings(input wizardMapInput) {
	a.setSetting("map", "provider", input.Provider)
	a.setSetting("map", input.Provider+".token", input.Token)
	if input.Style != "" {
		a.setSetting("map", input.Provider+".style", input.Style)
	}
}

func validAdmin(email, password, username string) bool {
	return strings.Contains(email, "@") && len(password) >= 6 && len(strings.TrimSpace(username)) >= 2
}

func validMap(input wizardMapInput) bool {
	return (input.Provider == "mapbox" || input.Provider == "maplibre") && strings.TrimSpace(input.Token) != ""
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

var errWizardUserExists = &wizardError{"User already exists"}

type wizardError struct{ message string }

func (e *wizardError) Error() string { return e.message }
