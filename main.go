package main

import (
	"embed"
	_ "embed"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func init() {
	application.RegisterEvent[*FlightData]("flight-data")
	application.RegisterEvent[bool]("recording-state")
	application.RegisterEvent[string]("connection-state")
	application.RegisterEvent[string]("flight-state")
}

func main() {
	si, err := NewSingleInstance()
	if err != nil {
		slog.Info("another instance is running, bringing to foreground")
		os.Exit(0)
	}
	defer si.Close()

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
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	flightDataService.setApp(app)
	flightService.setApp(app)
	updateService.setApp(app)

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
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

	si.SetOnShow(func() {
		window.Show()
		window.Focus()
	})

	// Hide to tray instead of closing
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	// System tray
	trayMenu := app.NewMenu()
	trayMenu.Add("Show").OnClick(func(ctx *application.Context) {
		window.Show()
		window.Focus()
	})
	trayMenu.AddSeparator()
	trayMenu.Add("Quit").OnClick(func(ctx *application.Context) {
		app.Quit()
	})

	systray := app.SystemTray.New()
	systray.SetIcon(appIcon)
	systray.SetTooltip("Airspace ACARS")
	systray.SetMenu(trayMenu)
	systray.OnClick(func() {
		window.Show()
		window.Focus()
	})

	go func() {
		time.Sleep(time.Second)

		// Auto-update on startup
		updateService.AutoUpdate()

		// Auto-connect to sim
		settings := settingsService.GetSettings()
		if adapter, err := flightDataService.ConnectSim(settings.SimType); err != nil {
			slog.Warn("auto-connect failed", "error", err)
		} else {
			slog.Info("auto-connected", "adapter", adapter)
		}
	}()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
