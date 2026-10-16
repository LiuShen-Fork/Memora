package main

import (
	"path/filepath"
	"testing"
)

// newSchemaTestApp opens a database without running ensureSchema, so a test can
// lay down a table in an older shape and then check what an upgrade does to it.
func newSchemaTestApp(t *testing.T) *App {
	t.Helper()
	dataDir := t.TempDir()
	cfg := Config{DBPath: filepath.Join(dataDir, "app.sqlite3"), DataDir: dataDir}
	db, err := openDatabase(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &App{cfg: cfg, db: db, logs: NewLogBuffer()}
}

func TestEnsureColumnAddsColumnAndKeepsExistingRows(t *testing.T) {
	app := newSchemaTestApp(t)
	if _, err := app.db.Exec(`CREATE TABLE albums (id INTEGER PRIMARY KEY, title TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.Exec(`INSERT INTO albums(id,title) VALUES(1,'kept')`); err != nil {
		t.Fatal(err)
	}
	added, err := app.ensureColumn("albums", "position", "REAL NOT NULL DEFAULT 0")
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatal("expected ensureColumn to report that it added the column")
	}
	// The upgrade path is additive: rows that were already there keep their
	// values and pick up the column default.
	var title string
	var position float64
	if err := app.db.QueryRow(`SELECT title,position FROM albums WHERE id=1`).Scan(&title, &position); err != nil {
		t.Fatal(err)
	}
	if title != "kept" || position != 0 {
		t.Fatalf("row = (%q,%v), want (kept,0)", title, position)
	}
	// Every startup re-runs the whole list, so a step that has already been
	// applied has to be a silent no-op.
	again, err := app.ensureColumn("albums", "position", "REAL NOT NULL DEFAULT 0")
	if err != nil {
		t.Fatal(err)
	}
	if again {
		t.Fatal("expected a second ensureColumn to report no change")
	}
}

func TestApplySchemaStepBackfillsOnlyOnce(t *testing.T) {
	app := newSchemaTestApp(t)
	if _, err := app.db.Exec(`CREATE TABLE albums (id INTEGER PRIMARY KEY, created_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := app.db.Exec(`INSERT INTO albums(id,created_at) VALUES(1,100),(2,200),(3,300)`); err != nil {
		t.Fatal(err)
	}
	step := schemaStep{
		table:      "albums",
		column:     "position",
		definition: "REAL NOT NULL DEFAULT 0",
		backfill:   `UPDATE albums SET position = (SELECT COUNT(*) FROM albums AS a2 WHERE a2.created_at > albums.created_at) * 1000`,
	}
	if err := app.applySchemaStep(step); err != nil {
		t.Fatal(err)
	}
	// Ascending position has to reproduce the created_at DESC order the table
	// used before the column existed.
	rows, err := app.db.Query(`SELECT id FROM albums ORDER BY position ASC`)
	if err != nil {
		t.Fatal(err)
	}
	order := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		order = append(order, id)
	}
	_ = rows.Close()
	if len(order) != 3 || order[0] != 3 || order[1] != 2 || order[2] != 1 {
		t.Fatalf("backfilled order = %v, want [3 2 1]", order)
	}
	// A re-run must leave a user's own ordering alone rather than recompute it.
	if _, err := app.db.Exec(`UPDATE albums SET position=42 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	if err := app.applySchemaStep(step); err != nil {
		t.Fatal(err)
	}
	var position float64
	if err := app.db.QueryRow(`SELECT position FROM albums WHERE id=2`).Scan(&position); err != nil {
		t.Fatal(err)
	}
	if position != 42 {
		t.Fatalf("position = %v after a second run, want the user's 42", position)
	}
}
