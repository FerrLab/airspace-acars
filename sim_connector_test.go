package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransponderStateString(t *testing.T) {
	tests := []struct {
		name string
		val  float64
		want string
	}{
		{"off", 0, "off"},
		{"stand-by", 1, "stand-by"},
		{"active mode 2", 2, "active"},
		{"active mode 3", 3, "active"},
		{"active mode 4 (MSFS TA/RA)", 4, "active"},
		{"negative treated as off", -1, "active"}, // default case
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, TransponderStateString(tt.val))
		})
	}
}
