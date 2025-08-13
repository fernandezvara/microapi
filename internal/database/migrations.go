package database

import (
	"database/sql"
)

func Migrate(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS metadata (
		set_name TEXT NOT NULL,
		collection_name TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		PRIMARY KEY (set_name, collection_name)
	);

	-- Index metadata for JSON path indexes
	CREATE TABLE IF NOT EXISTS idx_metadata (
		set_name TEXT NOT NULL,
		collection_name TEXT NOT NULL,
		idx_name TEXT NOT NULL,
		-- comma-separated list of JSON paths (e.g. $.user.email,$.user.id)
		paths TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'creating', -- creating | ready | error
		error TEXT,
		usage_count INTEGER NOT NULL DEFAULT 0,
		last_used_at INTEGER,
		created_at INTEGER NOT NULL,
		PRIMARY KEY (set_name, collection_name, idx_name)
	);

	-- Schemas per collection in JSON Schema format
	CREATE TABLE IF NOT EXISTS schemas (
		set_name TEXT NOT NULL,
		collection_name TEXT NOT NULL,
		schema JSON,
		updated_at INTEGER NOT NULL,
		PRIMARY KEY (set_name, collection_name)
	);
	`)
	return err
}
