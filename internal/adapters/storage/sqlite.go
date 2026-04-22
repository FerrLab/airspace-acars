package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"airspace-acars/internal/domain"

	_ "modernc.org/sqlite"
)

// SQLiteAdapter implements the Storage interface with SQLite.
type SQLiteAdapter struct {
	db    *sql.DB
	writeMu sync.Mutex
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

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS position_outbox (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		booking_id TEXT    NOT NULL,
		payload    TEXT    NOT NULL,
		created_at INTEGER NOT NULL
	)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create position_outbox: %w", err)
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_outbox_booking ON position_outbox(booking_id, id)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create idx_outbox_booking: %w", err)
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

// EnqueuePosition persists one report to the outbox for later delivery.
func (s *SQLiteAdapter) EnqueuePosition(bookingID string, payload []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(
		`INSERT INTO position_outbox (booking_id, payload, created_at) VALUES (?, ?, ?)`,
		bookingID, string(payload), time.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("enqueue position: %w", err)
	}
	return nil
}

// PeekOutboxBatch reads up to limit rows for the given booking in insertion order.
// Rows remain until DeleteOutboxBatch is called with their ids.
func (s *SQLiteAdapter) PeekOutboxBatch(bookingID string, limit int) ([]int64, [][]byte, error) {
	rows, err := s.db.Query(
		`SELECT id, payload FROM position_outbox WHERE booking_id = ? ORDER BY id ASC LIMIT ?`,
		bookingID, limit,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("peek outbox: %w", err)
	}
	defer rows.Close()

	var ids []int64
	var payloads [][]byte
	for rows.Next() {
		var id int64
		var payload string
		if err := rows.Scan(&id, &payload); err != nil {
			return nil, nil, fmt.Errorf("scan outbox row: %w", err)
		}
		ids = append(ids, id)
		payloads = append(payloads, []byte(payload))
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iter outbox rows: %w", err)
	}
	return ids, payloads, nil
}

// DeleteOutboxBatch removes rows by id. Used after successful POST.
func (s *SQLiteAdapter) DeleteOutboxBatch(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	// Build "DELETE ... WHERE id IN (?, ?, ...)" with positional args.
	args := make([]interface{}, len(ids))
	placeholders := make([]byte, 0, len(ids)*2)
	for i, id := range ids {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args[i] = id
	}
	query := "DELETE FROM position_outbox WHERE id IN (" + string(placeholders) + ")"
	if _, err := s.db.Exec(query, args...); err != nil {
		return fmt.Errorf("delete outbox batch: %w", err)
	}
	return nil
}

// CountOutbox returns how many rows are queued for the given booking.
func (s *SQLiteAdapter) CountOutbox(bookingID string) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM position_outbox WHERE booking_id = ?`,
		bookingID,
	).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count outbox: %w", err)
	}
	return n, nil
}
