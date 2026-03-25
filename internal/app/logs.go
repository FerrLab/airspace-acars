package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
)

// TailLogs returns the last n lines from the structured log file (stdout.log).
func (a *App) TailLogs(n int) ([]string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	logPath := filepath.Join(filepath.Dir(exe), "stdout.log")

	f, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	// Read all lines into a ring buffer of size n.
	ring := make([]string, 0, n)
	scanner := bufio.NewScanner(f)
	// Allow up to 1MB per line for large JSON log entries.
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		if len(ring) < n {
			ring = append(ring, scanner.Text())
		} else {
			copy(ring, ring[1:])
			ring[n-1] = scanner.Text()
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read log file: %w", err)
	}

	return ring, nil
}
