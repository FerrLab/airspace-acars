package main

import (
	"embed"
	_ "embed"
	"log"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[*FlightData]("flight-data")
	application.RegisterEvent[bool]("recording-state")
}

func main() {
	db, err := initDB()
	if err != nil {
		log.Fatal("failed to init database:", err)
	}
	defer db.Close()

	mockAuth := NewMockAuthServer()
	addr, err := mockAuth.Start()
	if err != nil {
		log.Fatal("failed to start mock auth server:", err)
	}
	slog.Info("mock auth server running", "addr", addr)

	authService := &AuthService{mockServerAddr: addr}
	settingsService := NewSettingsService()
	flightDataService := NewFlightDataService(db)

	app := application.New(application.Options{
		Name:        "Airspace ACARS",
		Description: "Flight Simulator ACARS Desktop Application",
		Services: []application.Service{
			application.NewService(authService),
			application.NewService(settingsService),
			application.NewService(flightDataService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	flightDataService.setApp(app)

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
