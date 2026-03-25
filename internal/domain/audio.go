package domain

type SoundInstruction struct {
	Type       string `json:"type"`
	URL        string `json:"url,omitempty"`
	LocalFile  string `json:"localFile,omitempty"`
	DurationMs int    `json:"duration_ms"`
}

type AudioData struct {
	Data        string `json:"data"`
	ContentType string `json:"contentType"`
}
