//go:build windows

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTrimNullBytes(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{"normal string with nulls", append([]byte("Boeing 737"), make([]byte, 10)...), "Boeing 737"},
		{"empty input", []byte{}, ""},
		{"all nulls", make([]byte, 5), ""},
		{"no nulls", []byte("Airbus A320"), "Airbus A320"},
		{"null at start", append([]byte{0}, []byte("hidden")...), ""},
		{"single char", append([]byte("A"), 0), "A"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, trimNullBytes(tt.input))
		})
	}
}
