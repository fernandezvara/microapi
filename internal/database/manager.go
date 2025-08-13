package database

import (
	"database/sql"
	"fmt"
	"time"
)

func tableName(set string) string { return fmt.Sprintf("data_%s", set) }

// TableName exposes the underlying physical table name for a set.
func TableName(set string) string { return tableName(set) }

func EnsureSetTable(db *sql.DB, set string) error {
	_, err := db.Exec(fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS %s (
		id TEXT PRIMARY KEY,
		collection TEXT NOT NULL,
		data JSON NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_%s_collection ON %s(collection);
	CREATE INDEX IF NOT EXISTS idx_%s_collection_created ON %s(collection, created_at DESC);
	`, tableName(set), set, tableName(set), set, tableName(set)))
	return err
}

func EnsureCollectionMetadata(db *sql.DB, set, collection string) error {
	_, err := db.Exec(`INSERT OR IGNORE INTO metadata (set_name, collection_name, created_at) VALUES (?, ?, ?)`, set, collection, time.Now().Unix())
	return err
}
