package database

import (
	"database/sql"
	"fmt"
	"log/slog"

	"microapi/internal/config"

	_ "modernc.org/sqlite"
)

func Open(cfg *config.Config) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)", cfg.DBPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// For SQLite, a small number of open connections is recommended
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	slog.Info("opened database", slog.String("path", cfg.DBPath))
	return db, nil
}
