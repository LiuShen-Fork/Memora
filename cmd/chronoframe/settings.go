package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) settingsRoute(w http.ResponseWriter, r *http.Request, rest string) {
	if rest == "/all" && r.Method == "GET" {
		a.allSettings(w, r)
		return
	}
	if rest == "/schema" && r.Method == "GET" {
		writeJSON(w, 200, map[string]any{"fields": []any{}})
		return
	}
	if rest == "/fields" && r.Method == "GET" {
		writeJSON(w, 200, []any{})
		return
	}
	if !a.requireAdmin(w, r) {
		return
	}
	if r.Method == "PUT" && rest == "/batch" {
		var b map[string]any
		_ = decodeJSON(r, &b)
		for key, v := range b {
			parts := strings.SplitN(key, ":", 2)
			if len(parts) == 2 {
				a.setSetting(parts[0], parts[1], v)
			}
		}
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) >= 2 && r.Method == "GET" {
		var value any
		if a.readSetting(parts[0], parts[1], &value) {
			writeJSON(w, 200, value)
		} else {
			errorJSON(w, 404, "Setting not found")
		}
		return
	}
	if len(parts) >= 2 && r.Method == "PUT" {
		var v any
		_ = decodeJSON(r, &v)
		a.setSetting(parts[0], parts[1], v)
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
func (a *App) allSettings(w http.ResponseWriter, _ *http.Request) {
	rows, _ := a.db.Query(`SELECT namespace,key,type,value,default_value,is_public FROM settings`)
	data := map[string]map[string]any{}
	for rows != nil && rows.Next() {
		var ns, key, typ string
		var value, def sql.NullString
		var pub int
		_ = rows.Scan(&ns, &key, &typ, &value, &def, &pub)
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
	if rows != nil {
		rows.Close()
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
	b := jsonValue(value)
	typ := "json"
	switch value.(type) {
	case string:
		typ = "string"
		b = fmt.Sprint(value)
	case bool:
		typ = "boolean"
		b = fmt.Sprint(value)
	case float64, int, int64:
		typ = "number"
	}
	_, _ = a.db.Exec(`INSERT INTO settings(namespace,key,type,value,updated_at) VALUES(?,?,?,?,unixepoch()) ON CONFLICT(namespace,key) DO UPDATE SET type=excluded.type,value=excluded.value,updated_at=unixepoch()`, ns, key, typ, b)
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

func (a *App) wizardRoute(w http.ResponseWriter, r *http.Request, rest string) {
	if rest == "schema" && r.Method == "GET" {
		writeJSON(w, 200, map[string]any{"namespace": r.URL.Query().Get("namespace"), "fields": []any{}})
		return
	}
	if rest == "complete" && r.Method == "POST" {
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	if rest == "submit" && r.Method == "POST" {
		var b struct {
			Admin   struct{ Email, Password, Username string }
			Site    map[string]any
			Storage map[string]any
			Map     map[string]any
		}
		if decodeJSON(r, &b) != nil {
			errorJSON(w, 400, "Invalid request")
			return
		}
		if b.Admin.Email != "" {
			hash, _ := bcrypt.GenerateFromPassword([]byte(b.Admin.Password), bcrypt.DefaultCost)
			_, _ = a.db.Exec(`INSERT INTO users(name,email,password,is_admin,created_at) VALUES(?,?,?,?,unixepoch()) ON CONFLICT(email) DO UPDATE SET name=excluded.name,password=excluded.password,is_admin=1`, b.Admin.Username, b.Admin.Email, string(hash), 1)
		}
		writeJSON(w, 200, map[string]any{"success": true})
		return
	}
	writeJSON(w, 200, map[string]any{"success": true})
}
