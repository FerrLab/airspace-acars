package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type AudioService struct {
	auth       *AuthService
	httpClient *http.Client
	cacheDir   string
	mu         sync.Mutex
}

type SoundInstruction struct {
	Type       string `json:"type"`
	URL        string `json:"url,omitempty"`
	LocalFile  string `json:"localFile,omitempty"`
	DurationMs int    `json:"duration_ms"`
}

type soundResponse struct {
	Instructions []SoundInstruction `json:"instructions"`
}

type AudioData struct {
	Data        string `json:"data"`
	ContentType string `json:"contentType"`
}

func NewAudioService(auth *AuthService) *AudioService {
	cacheDir := filepath.Join(os.TempDir(), "airspace-audio")
	os.MkdirAll(cacheDir, 0o755)

	return &AudioService{
		auth:       auth,
		httpClient: &http.Client{Timeout: 15_000_000_000}, // 15 seconds
		cacheDir:   cacheDir,
	}
}

func (a *AudioService) FetchSoundInstructions() ([]SoundInstruction, error) {
	body, _, err := a.auth.doRequest("GET", "/api/acars/sound", nil)
	if err != nil {
		return nil, err
	}

	var resp soundResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse sound instructions: %w", err)
	}

	// Pre-download any audio files with URLs
	for i, inst := range resp.Instructions {
		if inst.Type == "play" && inst.URL != "" {
			filename, err := a.downloadAndCache(inst.URL)
			if err != nil {
				slog.Warn("failed to download audio", "url", inst.URL, "error", err)
				continue
			}
			resp.Instructions[i].LocalFile = filename
		}
	}

	return resp.Instructions, nil
}

func (a *AudioService) GetAudioData(filename string) (*AudioData, error) {
	// Sanitize filename to prevent path traversal
	if strings.Contains(filename, "/") || strings.Contains(filename, "\\") || strings.Contains(filename, "..") {
		return nil, fmt.Errorf("invalid filename")
	}

	path := filepath.Join(a.cacheDir, filename)
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

	return &AudioData{
		Data:        base64.StdEncoding.EncodeToString(data),
		ContentType: contentType,
	}, nil
}

func (a *AudioService) ClearCache() {
	a.mu.Lock()
	defer a.mu.Unlock()

	entries, err := os.ReadDir(a.cacheDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		os.Remove(filepath.Join(a.cacheDir, e.Name()))
	}
}

func (a *AudioService) downloadAndCache(audioURL string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Use URL hash as filename base
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(audioURL)))[:16]

	// Check if already cached
	matches, _ := filepath.Glob(filepath.Join(a.cacheDir, hash+".*"))
	if len(matches) > 0 {
		return filepath.Base(matches[0]), nil
	}

	resp, err := a.httpClient.Get(audioURL)
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
	path := filepath.Join(a.cacheDir, filename)

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
