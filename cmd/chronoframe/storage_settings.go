package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type storageConfigRecord struct {
	ID        int64          `json:"id"`
	Name      string         `json:"name"`
	Provider  string         `json:"provider"`
	Config    map[string]any `json:"config"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

func (a *App) storageConfigRoute(w http.ResponseWriter, r *http.Request, rest string) {
	if !a.requireAdmin(w, r) {
		return
	}

	idText := strings.Trim(rest, "/")
	if idText == "" {
		switch r.Method {
		case http.MethodGet:
			a.listStorageConfigs(w)
		case http.MethodPost:
			a.createStorageConfig(w, r)
		default:
			errorJSON(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		}
		return
	}

	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id < 1 {
		errorJSON(w, http.StatusBadRequest, "Invalid storage configuration ID")
		return
	}
	switch r.Method {
	case http.MethodGet:
		record, err := a.findStorageConfig(id)
		if err != nil {
			errorJSON(w, http.StatusNotFound, "Storage configuration not found")
			return
		}
		writeJSON(w, http.StatusOK, record)
	case http.MethodPut:
		a.updateStorageConfig(w, r, id)
	case http.MethodDelete:
		a.deleteStorageConfig(w, id)
	default:
		errorJSON(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (a *App) listStorageConfigs(w http.ResponseWriter) {
	rows, err := a.db.Query(`SELECT id,name,provider,config,created_at,updated_at FROM settings_storage_providers ORDER BY id`)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to list storage configurations")
		return
	}
	defer rows.Close()

	configs := []storageConfigRecord{}
	for rows.Next() {
		record, err := scanStorageConfig(rows)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, "Unable to read storage configuration")
			return
		}
		configs = append(configs, record)
	}
	writeJSON(w, http.StatusOK, configs)
}

func (a *App) createStorageConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string         `json:"name"`
		Provider string         `json:"provider"`
		Config   map[string]any `json:"config"`
	}
	if decodeJSON(r, &body) != nil || strings.TrimSpace(body.Name) == "" || !validStorageConfig(body.Provider, body.Config) {
		errorJSON(w, http.StatusBadRequest, "Invalid storage configuration")
		return
	}
	id, err := a.insertStorageConfig(body.Name, body.Provider, body.Config)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to create storage configuration")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (a *App) insertStorageConfig(name, provider string, config map[string]any) (int64, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return 0, err
	}
	result, err := a.db.Exec(`INSERT INTO settings_storage_providers(name,provider,config,created_at,updated_at) VALUES(?,?,?,?,?)`, name, provider, configJSON, time.Now().Unix(), time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (a *App) findStorageConfig(id int64) (storageConfigRecord, error) {
	row := a.db.QueryRow(`SELECT id,name,provider,config,created_at,updated_at FROM settings_storage_providers WHERE id=?`, id)
	return scanStorageConfig(row)
}

type storageConfigScanner interface {
	Scan(...any) error
}

func scanStorageConfig(scanner storageConfigScanner) (storageConfigRecord, error) {
	var record storageConfigRecord
	var configJSON string
	var createdAt, updatedAt int64
	if err := scanner.Scan(&record.ID, &record.Name, &record.Provider, &configJSON, &createdAt, &updatedAt); err != nil {
		return storageConfigRecord{}, err
	}
	if err := json.Unmarshal([]byte(configJSON), &record.Config); err != nil {
		return storageConfigRecord{}, err
	}
	record.CreatedAt = time.Unix(createdAt, 0)
	record.UpdatedAt = time.Unix(updatedAt, 0)
	return record, nil
}

func (a *App) updateStorageConfig(w http.ResponseWriter, r *http.Request, id int64) {
	existing, err := a.findStorageConfig(id)
	if err != nil {
		errorJSON(w, http.StatusNotFound, "Storage configuration not found")
		return
	}
	var body struct {
		Name     *string        `json:"name"`
		Provider string         `json:"provider"`
		Config   map[string]any `json:"config"`
	}
	if decodeJSON(r, &body) != nil {
		errorJSON(w, http.StatusBadRequest, "Invalid storage configuration")
		return
	}
	provider := existing.Provider
	if body.Provider != "" {
		provider = body.Provider
	}
	for key, value := range body.Config {
		existing.Config[key] = value
	}
	if !validStorageConfig(provider, existing.Config) {
		errorJSON(w, http.StatusBadRequest, "Invalid storage configuration")
		return
	}
	name := existing.Name
	if body.Name != nil && strings.TrimSpace(*body.Name) != "" {
		name = *body.Name
	}
	configJSON, _ := json.Marshal(existing.Config)
	if _, err := a.db.Exec(`UPDATE settings_storage_providers SET name=?,provider=?,config=?,updated_at=? WHERE id=?`, name, provider, configJSON, time.Now().Unix(), id); err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to update storage configuration")
		return
	}
	var active string
	_ = a.db.QueryRow(`SELECT value FROM settings WHERE namespace='storage' AND key='provider'`).Scan(&active)
	if strings.Trim(active, `"`) == strconv.FormatInt(id, 10) && a.storage != nil {
		a.storage = a.loadStorage()
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (a *App) deleteStorageConfig(w http.ResponseWriter, id int64) {
	var active string
	_ = a.db.QueryRow(`SELECT value FROM settings WHERE namespace='storage' AND key='provider'`).Scan(&active)
	if active == strconv.FormatInt(id, 10) || strings.Trim(active, `"`) == strconv.FormatInt(id, 10) {
		errorJSON(w, http.StatusConflict, "Cannot delete the active storage configuration")
		return
	}
	result, err := a.db.Exec(`DELETE FROM settings_storage_providers WHERE id=?`, id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to delete storage configuration")
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		errorJSON(w, http.StatusNotFound, "Storage configuration not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func validStorageConfig(provider string, config map[string]any) bool {
	if config == nil || (provider != "local" && provider != "s3" && provider != "openlist") {
		return false
	}
	configProvider, _ := config["provider"].(string)
	if configProvider != provider {
		return false
	}
	required := func(keys ...string) bool {
		for _, key := range keys {
			if value, ok := config[key].(string); !ok || strings.TrimSpace(value) == "" {
				return false
			}
		}
		return true
	}
	switch provider {
	case "local":
		return required("basePath")
	case "s3":
		return required("endpoint", "bucket", "accessKeyId", "secretAccessKey")
	case "openlist":
		return required("baseUrl", "rootPath", "token")
	default:
		return false
	}
}

var _ storageConfigScanner = (*sql.Row)(nil)
