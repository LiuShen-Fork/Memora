package main

import "fmt"

// schemaStep adds one column to an existing table, optionally backfilling the
// rows that were already there.
type schemaStep struct {
	table      string
	column     string
	definition string
	// backfill runs once, immediately after the column is added, to give the
	// rows that were already there a sensible value. A fresh install adds the
	// column to an empty table, where it is a no-op.
	backfill string
}

// migrateSchema brings a database created by an earlier build up to the schema
// this build expects.
//
// Every step must be additive and idempotent. An install is upgraded in place
// on startup, so a step that drops, rewrites, or renames data would break the
// promise that a Memora database only ever moves forward.
//
// Columns added after a table first shipped belong here, not in the CREATE
// TABLE statements in ensureSchema. Those statements describe each table as it
// was first created, and the list below is the single place a later column is
// declared, so a fresh install and an upgraded one cannot drift apart.
func (a *App) migrateSchema() error {
	steps := []schemaStep{}
	for _, step := range steps {
		if err := a.applySchemaStep(step); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) applySchemaStep(step schemaStep) error {
	added, err := a.ensureColumn(step.table, step.column, step.definition)
	if err != nil {
		return err
	}
	if !added || step.backfill == "" {
		return nil
	}
	if _, err := a.db.Exec(step.backfill); err != nil {
		return fmt.Errorf("backfill %s.%s: %w", step.table, step.column, err)
	}
	return nil
}

// ensureColumn adds a column that an existing table is missing and reports
// whether it had to.
//
// ensureSchema can only issue CREATE TABLE IF NOT EXISTS, which does nothing
// once a table exists, so a database written by an older build never gains the
// columns a newer build reads. Asking the table what it already has keeps the
// upgrade in process and additive, with no rebuild step for the operator.
//
// Table and column names are compile-time constants supplied by this package,
// never request data, so building the statement with Sprintf is safe; SQLite
// does not accept bound parameters for identifiers.
func (a *App) ensureColumn(table, column, definition string) (bool, error) {
	exists, err := a.columnExists(table, column)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	if _, err := a.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)); err != nil {
		return false, fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return true, nil
}

func (a *App) columnExists(table, column string) (bool, error) {
	rows, err := a.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
