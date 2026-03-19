package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"

	"go.opentelemetry.io/otel/codes"
)

var audioTracer = observability.Tracer("audio")

// InitAudioCache sets up the audio cache directory.
func (a *App) InitAudioCache() {
	a.audioCacheDir = filepath.Join(os.TempDir(), "airspace-audio")
	os.MkdirAll(a.audioCacheDir, 0o755)
}

// FetchSoundInstructions retrieves audio instructions from the API and pre-downloads files.
func (a *App) FetchSoundInstructions() ([]domain.SoundInstruction, error) {
	_, span := audioTracer.Start(context.Background(), "audio.fetch_instructions")
	defer span.End()

	body, _, err := a.Airspace.DoRequest("GET", "/api/acars/sound", nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var resp struct {
		Instructions []domain.SoundInstruction `json:"instructions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("parse sound instructions: %w", err)
	}

	for i, inst := range resp.Instructions {
		if inst.Type == "play" && inst.URL != "" {
			filename, err := a.downloadAndCacheAudio(inst.URL)
			if err != nil {
				slog.Warn("failed to download audio", "url", inst.URL, "error", err)
				continue
			}
			resp.Instructions[i].LocalFile = filename
		}
	}

	return resp.Instructions, nil
}

// GetAudioData reads a cached audio file and returns it as base64.
func (a *App) GetAudioData(filename string) (*domain.AudioData, error) {
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return nil, fmt.Errorf("invalid filename")
	}

	path := filepath.Join(a.audioCacheDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read audio file: %w", err)
	}

	contentType := "audio/mpeg"
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".wav":
		contentType = "audio/wav"
	case ".ogg":
		contentType = "audio/ogg"
	}

	return &domain.AudioData{
		Data:        base64.StdEncoding.EncodeToString(data),
		ContentType: contentType,
	}, nil
}

// ClearCache removes all cached audio files.
func (a *App) ClearCache() {
	a.audioMu.Lock()
	defer a.audioMu.Unlock()

	entries, err := os.ReadDir(a.audioCacheDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		os.Remove(filepath.Join(a.audioCacheDir, e.Name()))
	}
}

func (a *App) downloadAndCacheAudio(audioURL string) (string, error) {
	a.audioMu.Lock()
	defer a.audioMu.Unlock()

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(audioURL)))[:16]

	matches, _ := filepath.Glob(filepath.Join(a.audioCacheDir, hash+".*"))
	if len(matches) > 0 {
		return filepath.Base(matches[0]), nil
	}

	resp, err := a.audioClient.Get(audioURL)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")
	ext := ".mp3"
	if strings.Contains(contentType, "wav") {
		ext = ".wav"
	} else if strings.Contains(contentType, "ogg") {
		ext = ".ogg"
	}

	filename := hash + ext
	path := filepath.Join(a.audioCacheDir, filename)

	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("write file: %w", err)
	}

	return filename, nil
}
