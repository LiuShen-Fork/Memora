package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type settingDefault struct {
	Namespace   string
	Key         string
	Type        string
	Default     any
	Public      bool
	Readonly    bool
	Secret      bool
	Enum        []string
	Label       string
	Description string
}

var defaultSettings = []settingDefault{
	{Namespace: "system", Key: "firstLaunch", Type: "boolean", Default: true, Readonly: true, Label: "settings.system.firstLaunch.label", Description: "settings.system.firstLaunch.description"},
	{Namespace: "app", Key: "title", Type: "string", Default: "ChronoFrame", Public: true, Label: "settings.app.title.label", Description: "settings.app.title.description"},
	{Namespace: "app", Key: "slogan", Type: "string", Default: "", Public: true, Label: "settings.app.slogan.label", Description: "settings.app.slogan.description"},
	{Namespace: "app", Key: "author", Type: "string", Default: "", Public: true, Label: "settings.app.author.label", Description: "settings.app.author.description"},
	{Namespace: "app", Key: "avatarUrl", Type: "string", Default: "", Public: true, Label: "settings.app.avatarUrl.label", Description: "settings.app.avatarUrl.description"},
	{Namespace: "app", Key: "appearance.theme", Type: "string", Default: "system", Public: true, Enum: []string{"light", "dark", "system"}, Label: "settings.app.appearance.theme.label", Description: "settings.app.appearance.theme.description"},
	{Namespace: "system", Key: "upload.maxFileSize", Type: "number", Default: 256, Public: true, Label: "settings.app.upload.maxFileSize.label", Description: "settings.app.upload.maxFileSize.description"},
	{Namespace: "system", Key: "upload.duplicateCheck.enabled", Type: "boolean", Default: true, Public: true, Label: "settings.system.upload.duplicateCheck.enabled.label", Description: "settings.system.upload.duplicateCheck.enabled.description"},
	{Namespace: "system", Key: "upload.duplicateCheck.mode", Type: "string", Default: "skip", Public: true, Enum: []string{"warn", "block", "skip"}, Label: "settings.system.upload.duplicateCheck.mode.label", Description: "settings.system.upload.duplicateCheck.mode.description"},
	{Namespace: "system", Key: "webglImageViewerDebug", Type: "boolean", Default: false, Public: true, Label: "settings.system.webglImageViewerDebug.label", Description: "settings.system.webglImageViewerDebug.description"},
	{Namespace: "system", Key: "auth.github.enabled", Type: "boolean", Default: false, Public: true, Label: "settings.system.auth.github.enabled.label", Description: "settings.system.auth.github.enabled.description"},
	{Namespace: "system", Key: "auth.github.clientId", Type: "string", Default: "", Label: "settings.system.auth.github.clientId.label", Description: "settings.system.auth.github.clientId.description"},
	{Namespace: "system", Key: "auth.github.clientSecret", Type: "string", Default: "", Secret: true, Label: "settings.system.auth.github.clientSecret.label", Description: "settings.system.auth.github.clientSecret.description"},
	{Namespace: "privacy", Key: "upload.autoEraseLocation", Type: "boolean", Default: false, Public: true, Label: "settings.privacy.upload.autoEraseLocation.label", Description: "settings.privacy.upload.autoEraseLocation.description"},
	{Namespace: "map", Key: "provider", Type: "string", Default: "maplibre", Public: true, Enum: []string{"mapbox", "maplibre", "amap"}, Label: "settings.map.provider.label", Description: "settings.map.provider.description"},
	{Namespace: "map", Key: "mapbox.token", Type: "string", Default: "", Public: true, Label: "settings.map.mapbox.token.label", Description: "settings.map.mapbox.token.description"},
	{Namespace: "map", Key: "mapbox.style", Type: "string", Default: "", Public: true, Label: "settings.map.mapbox.style.label", Description: "settings.map.mapbox.style.description"},
	{Namespace: "map", Key: "maplibre.token", Type: "string", Default: "", Public: true, Label: "settings.map.maplibre.token.label", Description: "settings.map.maplibre.token.description"},
	{Namespace: "map", Key: "maplibre.style", Type: "string", Default: "", Public: true, Label: "settings.map.maplibre.style.label", Description: "settings.map.maplibre.style.description"},
	{Namespace: "map", Key: "amap.key", Type: "string", Default: "", Public: true, Label: "settings.map.amap.key.label", Description: "settings.map.amap.key.description"},
	{Namespace: "map", Key: "amap.securityCode", Type: "string", Default: "", Public: true, Secret: true, Label: "settings.map.amap.securityCode.label", Description: "settings.map.amap.securityCode.description"},
	{Namespace: "location", Key: "language", Type: "string", Default: "en", Public: true, Label: "settings.location.language.label", Description: "settings.location.language.description"},
	{Namespace: "location", Key: "provider", Type: "string", Default: "nominatim", Public: true, Enum: []string{"nominatim", "mapbox", "amap"}, Label: "settings.location.provider.label", Description: "settings.location.provider.description"},
	{Namespace: "location", Key: "mapbox.token", Type: "string", Default: "", Public: true, Label: "settings.location.mapbox.token.label", Description: "settings.location.mapbox.token.description"},
	{Namespace: "location", Key: "amap.key", Type: "string", Default: "", Secret: true, Label: "settings.location.amap.key.label", Description: "settings.location.amap.key.description"},
	{Namespace: "location", Key: "nominatim.baseUrl", Type: "string", Default: "", Public: true, Label: "settings.location.nominatim.baseUrl.label", Description: "settings.location.nominatim.baseUrl.description"},
	{Namespace: "storage", Key: "provider", Type: "number", Default: nil, Label: "settings.storage_provider.provider.label", Description: "settings.storage_provider.provider.description"},
	{Namespace: "analytics", Key: "headScripts", Type: "string", Default: "", Public: true, Label: "settings.analytics.headScripts.label", Description: "settings.analytics.headScripts.description"},
	{Namespace: "analytics", Key: "bodyScripts", Type: "string", Default: "", Public: true, Label: "settings.analytics.bodyScripts.label", Description: "settings.analytics.bodyScripts.description"},
}

func (a *App) ensureDefaultSettings() error {
	for _, setting := range defaultSettings {
		var existingEnum sql.NullString
		if err := a.db.QueryRow(`SELECT enum FROM settings WHERE namespace=? AND key=?`, setting.Namespace, setting.Key).Scan(&existingEnum); err == nil {
			// Keep enum metadata in sync when an existing installation gains a
			// provider. Existing values and secrets remain unchanged.
			if len(setting.Enum) > 0 {
				enum, marshalErr := json.Marshal(setting.Enum)
				if marshalErr != nil {
					return marshalErr
				}
				if existingEnum.String != string(enum) {
					if _, updateErr := a.db.Exec(`UPDATE settings SET enum=?,updated_at=unixepoch() WHERE namespace=? AND key=?`, enum, setting.Namespace, setting.Key); updateErr != nil {
						return fmt.Errorf(`update enum %s:%s: %w`, setting.Namespace, setting.Key, updateErr)
					}
				}
			}
			continue
		}
		value := serializeSetting(setting.Type, setting.Default)
		enum, err := json.Marshal(setting.Enum)
		if err != nil {
			return err
		}
		_, err = a.db.Exec(`INSERT INTO settings(namespace,key,type,value,default_value,label,description,is_public,is_readonly,is_secret,enum,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,unixepoch())`, setting.Namespace, setting.Key, setting.Type, value, value, setting.Label, setting.Description, boolInt(setting.Public), boolInt(setting.Readonly), boolInt(setting.Secret), enum)
		if err != nil {
			return fmt.Errorf("default setting %s:%s: %w", setting.Namespace, setting.Key, err)
		}
	}
	return nil
}

// Existing installations may have users but no firstLaunch setting (or may
// have inherited the old default true). Treat those databases as initialized
// so a schema migration does not send an existing library through onboarding.
func (a *App) normalizeFirstLaunch() error {
	var value sql.NullString
	if err := a.db.QueryRow(`SELECT value FROM settings WHERE namespace='system' AND key='firstLaunch'`).Scan(&value); err != nil || !value.Valid {
		return err
	}
	if value.String != "true" && value.String != "1" {
		return nil
	}
	var users int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		return err
	}
	if users == 0 {
		return nil
	}
	_, err := a.db.Exec(`UPDATE settings SET value='false',updated_at=unixepoch() WHERE namespace='system' AND key='firstLaunch'`)
	return err
}

func serializeSetting(typ string, value any) any {
	if value == nil {
		return nil
	}
	if typ == "string" {
		return fmt.Sprint(value)
	}
	if typ == "boolean" {
		return fmt.Sprint(value == true)
	}
	if typ == "number" {
		switch v := value.(type) {
		case int:
			return v
		case int64:
			return v
		case float64:
			return v
		}
	}
	return jsonValue(value)
}

func settingUI(namespace, key, typ string, enum []string) map[string]any {
	uiType := "input"
	switch typ {
	case "boolean":
		uiType = "toggle"
	case "number":
		uiType = "number"
	}
	if len(enum) > 0 {
		uiType = "tabs"
	}
	ui := map[string]any{"type": uiType, "required": false}
	if key == "auth.github.clientSecret" || strings.HasSuffix(key, ".token") {
		ui["type"] = "password"
	}
	if len(enum) > 0 {
		options := make([]map[string]any, 0, len(enum))
		for _, value := range enum {
			options = append(options, map[string]any{"label": value, "value": value})
		}
		ui["options"] = options
	}
	return ui
}
