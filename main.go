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
	"airspace-acars/internal/adapters/winaudio"
	"airspace-acars/internal/adapters/xplane"
	"airspace-acars/internal/adapters/xplane/aptdat"
	"airspace-acars/internal/app"
	"airspace-acars/internal/domain"
	"airspace-acars/internal/profiles"
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
	application.RegisterEvent[*profiles.Plan]("aircraft-profile")
	application.RegisterEvent[bool]("session-expired")
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

	// X-Plane's airports are read from its installation, which is found from
	// the running simulator unless the pilot has named a folder. The source
	// outlives any one connection so its index is built once per session.
	var appInstance *app.App
	xplaneLayouts := aptdat.NewSource(func() string { return appInstance.GetSettings().XPlanePath })

	// --- Create app instance (inject adapters) ---
	appInstance = app.NewApp(
		airspaceAdapter,
		wailsEmitter,
		db,
		discordAdapter,
		func() domain.SimConnector { return simconnectadapter.NewAdapter() },
		func(host string, port int) domain.SimConnector {
			return xplane.NewAdapter(host, port, xplaneLayouts)
		},
	)

	// Initialize settings and audio cache
	appInstance.InitSettings()
	appInstance.InitAudioCache()
	appInstance.InitProfiles()

	// --- Create service wrappers (User Action Port for Wails) ---
	flightDataSvc := &FlightDataService{app: appInstance}
	flightSvc := &FlightService{app: appInstance}
	authSvc := &AuthService{app: appInstance}
	chatSvc := &ChatService{app: appInstance}
	notamSvc := &NOTAMService{app: appInstance}
	documentSvc := &DocumentService{app: appInstance}
	audioSvc := &AudioService{app: appInstance}
	settingsSvc := &SettingsService{app: appInstance}
	updateSvc := &UpdateService{app: appInstance}
	discordSvc := &DiscordService{app: appInstance}
	profileSvc := &ProfileService{app: appInstance}
	flightLogSvc := &FlightLogService{app: appInstance}
	debugSvc := &DebugService{app: appInstance}

	// --- Create Wails application ---
	wailsApp := application.New(application.Options{
		Name:        "Airspace ACARS",
		Description: "Flight Simulator ACARS Desktop Application",
		Services: []application.Service{
			application.NewService(flightDataSvc),
			application.NewService(flightSvc),
			application.NewService(authSvc),
			application.NewService(chatSvc),
			application.NewService(notamSvc),
			application.NewService(documentSvc),
			application.NewService(audioSvc),
			application.NewService(settingsSvc),
			application.NewService(updateSvc),
			application.NewService(discordSvc),
			application.NewService(profileSvc),
			application.NewService(flightLogSvc),
			application.NewService(debugSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
			// /documents/{id}/pdf streams a library PDF fetched with the
			// pilot token, so the Documents tab can render it in place.
			Middleware: appInstance.DocumentPDFMiddleware,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	// Wire the Wails app back to the emitter and set quit function
	windowStateMgr := newWindowStateManager("")
	wailsEmitter.SetApp(wailsApp)
	appInstance.QuitFunc = func() {
		windowStateMgr.saveSync()
		wailsApp.Quit()
	}

	windowOpts := application.WebviewWindowOptions{
		Title:  "Airspace ACARS",
		Width:  domain.DefaultWindowWidth,
		Height: domain.DefaultWindowHeight,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(10, 10, 10),
		URL:              "/",
	}
	var screens []*application.Screen
	if wailsApp.Screen != nil {
		screens = wailsApp.Screen.GetAll()
	}
	windowStateMgr.applyToOptions(&windowOpts, screens)

	// --- Create window ---
	window := wailsApp.Window.NewWithOptions(windowOpts)
	windowStateMgr.bind(window)

	wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		windowStateMgr.onApplicationStarted(window)
	})

	si.SetOnShow(func() {
		window.Show()
		window.Focus()
	})

	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		windowStateMgr.saveSync()
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
		windowStateMgr.saveSync()
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

	// Keep the Windows volume mixer showing the ACARS rather than the
	// webview that actually plays its audio. No-op on other platforms.
	audioLabelCtx, stopAudioLabelling := context.WithCancel(context.Background())
	defer stopAudioLabelling()
	go func() {
		defer observability.Recover()
		winaudio.Start(audioLabelCtx, "Airspace ACARS")
	}()

	go func() {
		defer observability.Recover()
		time.Sleep(time.Second)

		appInstance.AutoUpdate()
		wailsApp.Event.Emit("update-check-done", true)

		appInstance.AutoConnectLoop()
	}()

	if err := wailsApp.Run(); err != nil {
		log.Fatal(err)
	}
}
