package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"airspace-acars/internal/domain"
	"airspace-acars/observability"
)

// InitAudioCache sets up the audio cache directory.
func (a *App) InitAudioCache() {
	a.audioCacheDir = filepath.Join(os.TempDir(), "airspace-audio")
	os.MkdirAll(a.audioCacheDir, 0o755)
}

// FetchSoundInstructions retrieves audio instructions from the API and pre-downloads files.
func (a *App) FetchSoundInstructions() ([]domain.SoundInstruction, error) {
	_, span := observability.Start(context.Background(), "audio.fetch_instructions")
	defer span.Finish()

	body, status, err := a.Airspace.DoRequest("GET", "/api/v2/acars/sound", nil)
	if err == nil {
		err = domain.NewStatusError("GET", "/api/v2/acars/sound", status, body)
	}
	if err != nil {
		if noActiveBooking(err) {
			// This poll runs on a timer whether or not the pilot has taken a
			// booking yet, and a 404 is the server's normal answer when they
			// have not. Reporting it would repeat the "762 events for no
			// discord pipe found" mistake: see Span.Expected.
			span.Expected(err)
		} else {
			span.Fail(err)
		}
		return nil, err
	}

	var resp struct {
		Instructions []domain.SoundInstruction `json:"instructions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		span.Fail(err)
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

// noActiveBooking reports whether err is the sound endpoint's 404 for a
// pilot with no active booking - the only non-2xx status that endpoint
// returns by design, as opposed to an auth failure or a server fault.
func noActiveBooking(err error) bool {
	var se *domain.StatusError
	return errors.As(err, &se) && se.Status == http.StatusNotFound
}
