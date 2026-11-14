package main

import (
	"fmt"
	"strings"
	"testing"
)

// The wall asks for the newest photos first and a page at a time. That request
// is only cheap when the database can walk an index in that order: the
// alternative is reading every row, sorting all of them, and throwing away all
// but one page — a cost that grows with the collection and is paid again for
// each page scrolled.
func TestTheGalleryWalksTheWallWithoutSortingIt(t *testing.T) {
	app := newTestApp(t)
	for i := 0; i < 40; i++ {
		if _, err := app.db.Exec(
			`INSERT INTO photos(id,title,date_taken,last_modified) VALUES(?,?,?,?)`,
			fmt.Sprintf("photo-%02d", i),
			fmt.Sprintf("Photo %d", i),
			fmt.Sprintf("2024-05-%02dT10:00:00Z", i%28+1),
			"2024-06-01T10:00:00Z",
		); err != nil {
			t.Fatal(err)
		}
	}

	query := photoSelect + ` ORDER BY date_taken DESC, photos.id DESC LIMIT ? OFFSET ?`
	plan := queryPlan(t, app, query, 50, 0)

	if strings.Contains(strings.ToUpper(plan), "TEMP B-TREE") {
		t.Fatalf("the gallery page is sorted before it is sliced:\n%s", plan)
	}
	if !strings.Contains(plan, "USING INDEX") {
		t.Fatalf("the gallery page is not read through an index:\n%s", plan)
	}
}

// queryPlan answers with SQLite's written plan for one statement.
func queryPlan(t *testing.T, app *App, query string, args ...any) string {
	t.Helper()
	rows, err := app.db.Query(`EXPLAIN QUERY PLAN `+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
		plan.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return plan.String()
}
