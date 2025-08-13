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
	`)
	return err
}
