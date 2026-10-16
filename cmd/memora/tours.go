package main

import (
	"net/http"
)

// tourIDs are the guides an administrator can be walked through, in the order
// the interface lists them. A record is kept per page rather than for the site
// as a whole, so a guide that a later version rewrites can be seen again.
var tourIDs = []string{"dashboard", "photos", "albums", "general", "storage", "map", "privacy", "system", "home"}

var tourStatuses = []string{"pending", "completed", "skipped"}

// toursRoute reports and records which guides an administrator has already
// seen. Only an administrator is ever guided, so only an administrator may read
// or write the records.
func (a *App) toursRoute(w http.ResponseWriter, r *http.Request) {
	if !a.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
	case http.MethodPut:
		var body struct {
			Action string `json:"action"`
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if decodeJSON(r, &body) != nil {
			errorJSON(w, http.StatusBadRequest, "Invalid request")
			return
		}
		switch body.Action {
		case "reset":
			if _, err := a.db.Exec(`DELETE FROM tour_progress`); err != nil {
				errorJSON(w, http.StatusInternalServerError, "Unable to save the tour record")
				return
			}
		case "set":
			if !containsString(tourIDs, body.ID) || !containsString(tourStatuses, body.Status) {
				errorJSON(w, http.StatusBadRequest, "Invalid tour")
				return
			}
			if _, err := a.db.Exec(`INSERT INTO tour_progress(tour_id,status,updated_at) VALUES(?,?,unixepoch()) ON CONFLICT(tour_id) DO UPDATE SET status=excluded.status,updated_at=excluded.updated_at`, body.ID, body.Status); err != nil {
				errorJSON(w, http.StatusInternalServerError, "Unable to save the tour record")
				return
			}
		default:
			errorJSON(w, http.StatusBadRequest, "Invalid request")
			return
		}
	default:
		errorJSON(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	records, err := a.tourRecords()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "Unable to read the tour record")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

// tourRecords is every guide with the state it is in. A guide that was never
// answered is waiting to be shown, which is also what a reset leaves behind.
func (a *App) tourRecords() (map[string]string, error) {
	records := make(map[string]string, len(tourIDs))
	for _, id := range tourIDs {
		records[id] = "pending"
	}
	rows, err := a.db.Query(`SELECT tour_id,status FROM tour_progress`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		// A tour that a later version no longer offers is not reported.
		if _, known := records[id]; known {
			records[id] = status
		}
	}
	return records, rows.Err()
}
