package domain

type Settings struct {
	Theme            string `json:"theme"`
	SimType          string `json:"simType"`
	XPlaneHost       string `json:"xplaneHost"`
	XPlanePort       int    `json:"xplanePort"`
	APIBaseURL       string `json:"apiBaseURL"`
	LocalMode        bool   `json:"localMode"`
	ChatSound        string `json:"chatSound"`
	DiscordPresence  bool   `json:"discordPresence"`
	Language         string `json:"language"`
	AutoStartFlight  bool   `json:"autoStartFlight"`
	AutoFinishFlight bool   `json:"autoFinishFlight"`
}

func DefaultSettings() Settings {
	return Settings{
		Theme:            "dark",
		SimType:          "auto",
		XPlaneHost:       "127.0.0.1",
		XPlanePort:       49000,
		APIBaseURL:       "https://airspace.ferrlab.com",
		ChatSound:        "default",
		DiscordPresence:  true,
		Language:         "en",
		AutoStartFlight:  true,
		AutoFinishFlight: true,
	}
}
