package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"airspace-acars/internal/domain"

	_ "modernc.org/sqlite"
)

// SQLiteAdapter implements the Storage interface with SQLite.
type SQLiteAdapter struct {
	db *sql.DB
}

// NewSQLiteAdapter creates and initializes the SQLite database.
func NewSQLiteAdapter() (*SQLiteAdapter, error) {
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

	return &SQLiteAdapter{db: db}, nil
}

// Close closes the database connection.
func (s *SQLiteAdapter) Close() error {
	return s.db.Close()
}

// SaveFlightData inserts a flight data record as JSON.
func (s *SQLiteAdapter) SaveFlightData(data *domain.FlightData) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal flight data: %w", err)
	}
	_, err = s.db.Exec(`INSERT INTO flight_data (data) VALUES (?)`, string(jsonBytes))
	return err
}

// QueryFlightData returns all recorded flight data rows.
func (s *SQLiteAdapter) QueryFlightData() (*sql.Rows, error) {
	return s.db.Query(`SELECT timestamp, data FROM flight_data ORDER BY id`)
}

// PurgeFlightData deletes all recorded data.
func (s *SQLiteAdapter) PurgeFlightData() error {
	_, err := s.db.Exec(`DELETE FROM flight_data`)
	return err
}
