//go:build !windows

package simconnect

import "airspace-acars/internal/domain"

// NewAdapter returns nil on non-Windows platforms where SimConnect is unavailable.
func NewAdapter() domain.SimConnector { return nil }
