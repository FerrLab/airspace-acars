package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBeta(t *testing.T) {
	s := &UpdateService{}

	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{"dev build", "dev", false},
		{"stable release", "1.0.0", false},
		{"beta release", "1.0.0-beta.1", true},
		{"beta in middle", "2.0.0-beta.3", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := Version
			Version = tt.version
			defer func() { Version = orig }()

			assert.Equal(t, tt.want, s.isBeta())
		})
	}
}

func TestIsStableRelease(t *testing.T) {
	s := &UpdateService{}

	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{"dev is not stable", "dev", false},
		{"beta is not stable", "1.0.0-beta.1", false},
		{"release is stable", "1.0.0", true},
		{"patch release is stable", "1.2.3", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := Version
			Version = tt.version
			defer func() { Version = orig }()

			assert.Equal(t, tt.want, s.isStableRelease())
		})
	}
}

func TestComparableVersion(t *testing.T) {
	s := &UpdateService{}

	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"dev returns 0.0.0", "dev", "0.0.0"},
		{"release passes through", "1.2.3", "1.2.3"},
		{"beta passes through", "1.0.0-beta.1", "1.0.0-beta.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := Version
			Version = tt.version
			defer func() { Version = orig }()

			assert.Equal(t, tt.want, s.comparableVersion())
		})
	}
}
