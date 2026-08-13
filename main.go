package main

import (
	"context"
	"embed"
	_ "embed"
	"log"
	"log/slog"
	"time"

	"airspace-acars/internal/adapters/airspace"
	discordadapter "airspace-acars/internal/adapters/discord"
	simconnectadapter "airspace-acars/internal/adapters/simconnect"
	"airspace-acars/internal/adapters/storage"
	wailsadapter "airspace-acars/internal/adapters/wails"
	"airspace-acars/internal/adapters/xplane"
	"airspace-acars/internal/app"
	"airspace-acars/internal/domain"
	"airspace-acars/observability"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func init() {
	application.RegisterEvent[*domain.FlightData]("flight-data")
	application.RegisterEvent[bool]("recording-state")
	application.RegisterEvent[string]("connection-state")
	application.RegisterEvent[string]("flight-state")
	application.RegisterEvent[bool]("update-check-done")
	application.RegisterEvent[string]("auto-flight-start")
	application.RegisterEvent[bool]("request-window-close")
}

func main() {
	si, err := NewSingleInstance()
	if err != nil {
		slog.Info("another instance is running, bringing to foreground")
		return
	}
	defer si.Close()

	shutdown, err := observability.Init("airspace-acars", domain.Version)
	if err != nil {
		log.Fatal("failed to init observability:", err)
	}
	defer shutdown(context.Background())

	// --- Inject embedded DLL into SimConnect adapter ---
	simconnectadapter.EmbeddedDLL = embeddedSimConnectDLL

	// --- Create adapters ---
	airspaceAdapter := airspace.NewAdapter()
	wailsEmitter := wailsadapter.NewAdapter()
	discordAdapter := discordadapter.NewAdapter()

	db, err := storage.NewSQLiteAdapter()
	if err != nil {
		log.Fatal("failed to init database:", err)
	}
	defer db.Close()

	// --- Create app instance (inject adapters) ---
	appInstance := app.NewApp(
		airspaceAdapter,
		wailsEmitter,
		db,
		discordAdapter,
		func() domain.SimConnector { return simconnectadapter.NewAdapter() },
		func(host string, port int) domain.SimConnector { return xplane.NewAdapter(host, port) },
	)

	// Initialize settings and audio cache
	appInstance.InitSettings()
	appInstance.InitAudioCache()

	// --- Create service wrappers (User Action Port for Wails) ---
	flightDataSvc := &FlightDataService{app: appInstance}
	flightSvc := &FlightService{app: appInstance}
	authSvc := &AuthService{app: appInstance}
	chatSvc := &ChatService{app: appInstance}
	audioSvc := &AudioService{app: appInstance}
	settingsSvc := &SettingsService{app: appInstance}
	updateSvc := &UpdateService{app: appInstance}
	discordSvc := &DiscordService{app: appInstance}

	// --- Create Wails application ---
	wailsApp := application.New(application.Options{
		Name:        "Airspace ACARS",
		Description: "Flight Simulator ACARS Desktop Application",
		Services: []application.Service{
			application.NewService(flightDataSvc),
			application.NewService(flightSvc),
			application.NewService(authSvc),
			application.NewService(chatSvc),
			application.NewService(audioSvc),
			application.NewService(settingsSvc),
			application.NewService(updateSvc),
			application.NewService(discordSvc),
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

	// Wire the Wails app back to the emitter and set quit function
	wailsEmitter.SetApp(wailsApp)
	appInstance.QuitFunc = func() { wailsApp.Quit() }

	// --- Create window ---
	window := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
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

	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if appInstance.GetSettings().ConfirmCloseApp {
			wailsApp.Event.Emit("request-window-close", true)
			window.Show()
			window.Focus()
			e.Cancel()
			return
		}
		window.Hide()
		e.Cancel()
	})

	// --- System tray ---
	trayLabels := map[string][2]string{
		"en": {"Show", "Quit"},
		"es": {"Mostrar", "Salir"},
		"pt": {"Mostrar", "Sair"},
		"fr": {"Afficher", "Quitter"},
	}
	lang := appInstance.GetSettings().Language
	labels := trayLabels[lang]
	if labels == [2]string{} {
		labels = trayLabels["en"]
	}
	trayMenu := wailsApp.NewMenu()
	trayMenu.Add(labels[0]).OnClick(func(ctx *application.Context) {
		window.Show()
		window.Focus()
	})
	trayMenu.AddSeparator()
	trayMenu.Add(labels[1]).OnClick(func(ctx *application.Context) {
		wailsApp.Quit()
	})

	systray := wailsApp.SystemTray.New()
	systray.SetIcon(appIcon)
	systray.SetTooltip("Airspace ACARS")
	systray.SetMenu(trayMenu)
	systray.OnClick(func() {
		window.Show()
		window.Focus()
	})

	// --- Start background services ---
	appInstance.StartDiscordLoop()

	go func() {
		time.Sleep(time.Second)

		appInstance.AutoUpdate()
		wailsApp.Event.Emit("update-check-done", true)

		appInstance.AutoConnectLoop()
	}()

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
