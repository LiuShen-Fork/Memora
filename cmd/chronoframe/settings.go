package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var processStartedAt = time.Now()

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
	if r.Method == http.MethodPut && rest == "/batch" {
		if !a.requireAdmin(w, r) {
			return
		}
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
		updated := 0
		errors := []map[string]string{}
		for _, update := range body.Updates {
			if err := a.updateSetting(update.Namespace, update.Key, update.Value, nil, false); err != nil {
				errors = append(errors, map[string]string{"namespace": update.Namespace, "key": update.Key, "error": err.Error()})
				continue
			}
			updated++
		}
		if len(errors) > 0 {
			writeJSON(w, http.StatusOK, map[string]any{"success": false, "updated": updated, "errors": errors})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "updated": updated})
		return
	}
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 1 && r.Method == http.MethodGet {
		if !a.requireAdmin(w, r) {
			return
		}
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
		if !a.requireAdmin(w, r) {
			return
		}
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
func (a *App) allSettings(w http.ResponseWriter, r *http.Request) {
	data, err := a.publicSettingsSnapshot(r.Context())
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to read public settings")
		return
	}
	// The snapshot is invalidated whenever a setting changes, so a short
	// browser cache avoids repeating the same startup request while retaining
	// prompt updates after administration changes.
	w.Header().Set("Cache-Control", "private, max-age=30, stale-while-revalidate=300")
	writeJSON(w, 200, map[string]any{"timestamp": time.Now().UnixMilli(), "data": data})
}

func (a *App) publicSettingsSnapshot(ctx context.Context) (map[string]map[string]any, error) {
	a.settingsMu.RLock()
	if a.publicSettings != nil {
		data := cloneSettings(a.publicSettings)
		a.settingsMu.RUnlock()
		return data, nil
	}
	a.settingsMu.RUnlock()

	rows, err := a.db.QueryContext(ctx, `SELECT namespace,key,type,value FROM settings WHERE is_public=1 OR (namespace='system' AND key='firstLaunch')`)
	if err != nil {
		return nil, err
	}
	data := map[string]map[string]any{}
	for rows.Next() {
		var ns, key, typ string
		var value sql.NullString
		if err := rows.Scan(&ns, &key, &typ, &value); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if data[ns] == nil {
			data[ns] = map[string]any{}
		}
		if !value.Valid {
			data[ns][key] = nil
		} else {
			data[ns][key] = parseSetting(typ, value.String)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	a.settingsMu.Lock()
	if a.publicSettings == nil {
		a.publicSettings = data
	}
	data = cloneSettings(a.publicSettings)
	a.settingsMu.Unlock()
	return data, nil
}

func cloneSettings(source map[string]map[string]any) map[string]map[string]any {
	copy := make(map[string]map[string]any, len(source))
	for namespace, values := range source {
		copy[namespace] = make(map[string]any, len(values))
		for key, value := range values {
			copy[namespace][key] = value
		}
	}
	return copy
}

func (a *App) invalidateSettingsCache() {
	a.settingsMu.Lock()
	a.publicSettings = nil
	a.settingsMu.Unlock()
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
		if err == nil && ns == "storage" && key == "provider" && a.storage != nil {
			a.storage = a.loadStorage()
		}
		if err == nil {
			a.invalidateSettingsCache()
		}
		return err
	}
	_, err := a.db.Exec(`UPDATE settings SET value=?,updated_at=unixepoch(),updated_by=? WHERE namespace=? AND key=?`, serialized, updatedBy, ns, key)
	if err == nil && ns == "storage" && key == "provider" && a.storage != nil {
		a.storage = a.loadStorage()
	}
	if err == nil {
		a.invalidateSettingsCache()
	}
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
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var total, today, week, month int
	_ = a.db.QueryRow(`SELECT count(*) FROM photos`).Scan(&total)
	_ = a.db.QueryRow(`SELECT count(*) FROM photos WHERE date_taken >= datetime('now','start of day')`).Scan(&today)
	_ = a.db.QueryRow(`SELECT count(*) FROM photos WHERE date_taken >= datetime('now','-7 days')`).Scan(&week)
	_ = a.db.QueryRow(`SELECT count(*) FROM photos WHERE date_taken >= date('now','start of month')`).Scan(&month)

	var totalSize, averageSize, maxSize sql.NullFloat64
	_ = a.db.QueryRow(`SELECT COALESCE(sum(file_size),0), COALESCE(avg(file_size),0), COALESCE(max(file_size),0) FROM photos`).Scan(&totalSize, &averageSize, &maxSize)

	type trend struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	}
	trendByDate := map[string]int{}
	rows, err := a.db.Query(`SELECT date(date_taken), count(*) FROM photos WHERE date_taken >= datetime('now','-6 days','start of day') GROUP BY date(date_taken)`)
	if err == nil {
		for rows.Next() {
			var date string
			var count int
			if rows.Scan(&date, &count) == nil {
				trendByDate[date] = count
			}
		}
		_ = rows.Close()
	}
	trends := make([]trend, 0, 7)
	for i := 0; i < 7; i++ {
		day := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		trends = append(trends, trend{Date: day, Count: trendByDate[day]})
	}

	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	runningOn := runtime.GOOS
	if _, err := os.Stat(`/.dockerenv`); err == nil {
		runningOn = "docker"
	}
	writeJSON(w, 200, map[string]any{
		"uptime":     time.Since(processStartedAt).Seconds(),
		"runningOn":  runningOn,
		"memory":     map[string]any{"used": memory.Alloc, "total": memory.Sys},
		"photos":     map[string]any{"total": total, "today": today, "thisWeek": week, "thisMonth": month},
		"workerPool": a.queuePoolStats(),
		"storage":    map[string]any{"totalSize": nullFloat(totalSize), "averageSize": nullFloat(averageSize), "maxSize": nullFloat(maxSize)},
		"trends":     trends,
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func nullFloat(value sql.NullFloat64) float64 {
	if value.Valid {
		return value.Float64
	}
	return 0
}
func (a *App) systemLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.require(r); !ok {
		errorJSON(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	initial, offset, err := a.logs.Tail(400)
	if err != nil {
		initial, offset = a.logs.Snapshot(), 0
	}
	for _, line := range initial {
		fmt.Fprintf(w, "data: %s\n\n", line)
	}
	flusher, canFlush := w.(http.Flusher)
	if canFlush {
		flusher.Flush()
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-deadline.C:
			return
		case <-ticker.C:
			lines, next, readErr := a.logs.Since(offset)
			if readErr != nil {
				continue
			}
			for _, line := range lines {
				fmt.Fprintf(w, "data: %s\n\n", line)
			}
			offset = next
			if canFlush {
				flusher.Flush()
			}
		}
	}
}
