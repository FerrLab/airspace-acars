package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func initDB() (*sql.DB, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("get config dir: %w", err)
	}

	dbDir := filepath.Join(configDir, "airspace-acars")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dbPath := filepath.Join(dbDir, "flight_data.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Migrate: drop old column-per-field schema if it exists
	var colCount int
	row := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('flight_data') WHERE name = 'altitude'`)
	if err := row.Scan(&colCount); err == nil && colCount > 0 {
		db.Exec(`DROP TABLE flight_data`)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS flight_data (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		data TEXT NOT NULL
	)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	return db, nil
}
