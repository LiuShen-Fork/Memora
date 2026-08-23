package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) settingsRoute(w http.ResponseWriter, r *http.Request, rest string) {
	if strings.HasPrefix(rest, "/storage-config") {
		a.storageConfigRoute(w, r, strings.TrimPrefix(rest, "/storage-config"))
		return
	}
	if rest == "/all" && r.Method == http.MethodGet {
		a.allSettings(w, r)
		return
	}
	if rest == "/schema" && r.Method == http.MethodGet {
		if !a.requireAdmin(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, a.settingSchema())
		return
	}
	if rest == "/fields" && r.Method == http.MethodGet {
		if !a.requireAdmin(w, r) {
			return
		}
		namespace := r.URL.Query().Get("namespace")
		fields := a.settingFields(namespace)
		if fields == nil {
			errorJSON(w, http.StatusNotFound, "Namespace not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"namespace": namespace, "fields": fields})
		return
	}
	if !a.requireAdmin(w, r) {
		return
	}
	if r.Method == http.MethodPut && rest == "/batch" {
		var body struct {
			Updates []struct {
				Namespace string `json:"namespace"`
				Key       string `json:"key"`
				Value     any    `json:"value"`
			} `json:"updates"`
		}
		if decodeJSON(r, &body) != nil {
			errorJSON(w, http.StatusBadRequest, "Invalid settings update")
			return
		}
		for _, update := range body.Updates {
			if err := a.updateSetting(update.Namespace, update.Key, update.Value, nil, false); err != nil {
				errorJSON(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
		return
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodGet {
		settings := a.namespaceSettings(parts[0])
		if settings == nil {
			errorJSON(w, http.StatusNotFound, "Namespace not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"namespace": parts[0], "settings": settings})
		return
	}
	if len(parts) >= 2 && r.Method == http.MethodGet {
		var value any
		if !a.readSetting(parts[0], strings.Join(parts[1:], "."), &value) {
			errorJSON(w, http.StatusNotFound, "Setting not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"namespace": parts[0], "key": strings.Join(parts[1:], "."), "value": value})
		return
	}
	if len(parts) >= 2 && r.Method == http.MethodPut {
		var body struct {
			Value any `json:"value"`
		}
		if decodeJSON(r, &body) != nil {
			errorJSON(w, http.StatusBadRequest, "Invalid setting update")
			return
		}
		key := strings.Join(parts[1:], ".")
		if err := a.updateSetting(parts[0], key, body.Value, nil, false); err != nil {
			errorJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"namespace": parts[0], "key": key, "value": body.Value})
		return
	}
	errorJSON(w, http.StatusMethodNotAllowed, "Method Not Allowed")
}

func (a *App) settingSchema() []map[string]any {
	result := make([]map[string]any, 0, len(defaultSettings))
	for _, setting := range defaultSettings {
		var value any
		a.readSetting(setting.Namespace, setting.Key, &value)
		result = append(result, map[string]any{"namespace": setting.Namespace, "key": setting.Key, "type": setting.Type, "value": value, "defaultValue": setting.Default, "label": setting.Label, "description": setting.Description, "isPublic": setting.Public, "isReadonly": setting.Readonly, "isSecret": setting.Secret, "enum": setting.Enum})
	}
	return result
}

func (a *App) settingFields(namespace string) []map[string]any {
	fields := []map[string]any{}
	for _, setting := range defaultSettings {
		if setting.Namespace != namespace {
			continue
		}
		var value any
		a.readSetting(setting.Namespace, setting.Key, &value)
		fields = append(fields, map[string]any{"namespace": setting.Namespace, "key": setting.Key, "type": setting.Type, "value": value, "defaultValue": setting.Default, "label": setting.Label, "description": setting.Description, "isPublic": setting.Public, "isReadonly": setting.Readonly, "isSecret": setting.Secret, "enum": setting.Enum, "ui": settingUI(setting.Namespace, setting.Key, setting.Type, setting.Enum)})
	}
	return fields
}

func (a *App) namespaceSettings(namespace string) map[string]any {
	fields := a.settingFields(namespace)
	if fields == nil || len(fields) == 0 {
		return nil
	}
	result := map[string]any{}
	for _, field := range fields {
		result[field["key"].(string)] = field["value"]
	}
	return result
}
func (a *App) allSettings(w http.ResponseWriter, _ *http.Request) {
	rows, err := a.db.Query(`SELECT namespace,key,type,value,default_value,is_public FROM settings WHERE is_public=1 OR (namespace='system' AND key='firstLaunch')`)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to read public settings")
		return
	}
	defer rows.Close()
	data := map[string]map[string]any{}
	for rows.Next() {
		var ns, key, typ string
		var value, def sql.NullString
		var pub int
		if err := rows.Scan(&ns, &key, &typ, &value, &def, &pub); err != nil {
			errorJSON(w, http.StatusInternalServerError, "Unable to read public settings")
			return
		}
		if data[ns] == nil {
			data[ns] = map[string]any{}
		}
		data[ns][key] = parseSetting(typ, func() string {
			if value.Valid {
				return value.String
			}
			return def.String
		}())
	}
	if data["app"] == nil {
		data["app"] = map[string]any{}
	}
	if _, ok := data["app"]["title"]; !ok {
		data["app"]["title"] = env("NUXT_PUBLIC_APP_TITLE", "ChronoFrame")
	}
	writeJSON(w, 200, map[string]any{"timestamp": time.Now().UnixMilli(), "data": data})
}
func parseSetting(typ, value string) any {
	switch typ {
	case "number":
		n, _ := strconv.ParseFloat(value, 64)
		return n
	case "boolean":
		return value == "true" || value == "1"
	case "json":
		var v any
		if json.Unmarshal([]byte(value), &v) == nil {
			return v
		}
	}
	return value
}
func (a *App) readSetting(ns, key string, out *any) bool {
	var typ string
	var value, def sql.NullString
	if a.db.QueryRow(`SELECT type,value,default_value FROM settings WHERE namespace=? AND key=?`, ns, key).Scan(&typ, &value, &def) != nil {
		return false
	}
	v := value.String
	if !value.Valid {
		v = def.String
	}
	*out = parseSetting(typ, v)
	return true
}
func (a *App) setSetting(ns, key string, value any) {
	_ = a.updateSetting(ns, key, value, nil, true)
}

func (a *App) updateSetting(ns, key string, value any, updatedBy any, sudo bool) error {
	var typ string
	var readonly int
	var enumJSON sql.NullString
	if err := a.db.QueryRow(`SELECT type,is_readonly,enum FROM settings WHERE namespace=? AND key=?`, ns, key).Scan(&typ, &readonly, &enumJSON); err != nil {
		return fmt.Errorf("setting %s:%s not found", ns, key)
	}
	if readonly == 1 && !sudo {
		return fmt.Errorf("setting %s:%s is readonly", ns, key)
	}
	if enumJSON.Valid && enumJSON.String != "null" && enumJSON.String != "[]" {
		var allowed []string
		if json.Unmarshal([]byte(enumJSON.String), &allowed) == nil && value != nil && !containsString(allowed, fmt.Sprint(value)) {
			return fmt.Errorf("invalid value for setting %s:%s", ns, key)
		}
	}
	serialized := serializeSetting(typ, value)
	if updatedBy == nil {
		_, err := a.db.Exec(`UPDATE settings SET value=?,updated_at=unixepoch() WHERE namespace=? AND key=?`, serialized, ns, key)
		return err
	}
	_, err := a.db.Exec(`UPDATE settings SET value=?,updated_at=unixepoch(),updated_by=? WHERE namespace=? AND key=?`, serialized, updatedBy, ns, key)
	return err
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func (a *App) systemStats(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	var total int
	a.db.QueryRow(`SELECT count(*) FROM photos`).Scan(&total)
	writeJSON(w, 200, map[string]any{"total": total, "dateRange": nil, "storage": map[string]any{}})
}
func (a *App) systemLogs(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	for _, line := range a.logs.Snapshot() {
		fmt.Fprintf(w, "data: %s\n\n", line)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	select {
	case <-r.Context().Done():
	case <-time.After(30 * time.Second):
	}
}
