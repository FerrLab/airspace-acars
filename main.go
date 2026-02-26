package main

import (
	"embed"
	_ "embed"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[*FlightData]("flight-data")
	application.RegisterEvent[bool]("recording-state")
	application.RegisterEvent[bool]("connection-state")
	application.RegisterEvent[string]("flight-state")
}

func main() {
	db, err := initDB()
	if err != nil {
		log.Fatal("failed to init database:", err)
	}
	defer db.Close()

	settingsService := NewSettingsService()
	authService := &AuthService{httpClient: &http.Client{Timeout: 30 * time.Second}, settings: settingsService}
	flightDataService := NewFlightDataService(db)
	flightService := NewFlightService(authService, flightDataService)
	chatService := NewChatService(authService)
	audioService := NewAudioService(authService)
	updateService := &UpdateService{}

	app := application.New(application.Options{
		Name:        "Airspace ACARS",
		Description: "Flight Simulator ACARS Desktop Application",
		Services: []application.Service{
			application.NewService(authService),
			application.NewService(settingsService),
			application.NewService(flightDataService),
			application.NewService(flightService),
			application.NewService(chatService),
			application.NewService(audioService),
			application.NewService(updateService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	flightDataService.setApp(app)
	flightService.setApp(app)

	go func() {
		time.Sleep(time.Second)
		settings := settingsService.GetSettings()
		if err := flightDataService.ConnectSim(settings.SimType); err != nil {
			slog.Warn("auto-connect failed", "error", err)
		}
	}()

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Airspace ACARS",
		Width:  1100,
		Height: 700,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(10, 10, 10),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
