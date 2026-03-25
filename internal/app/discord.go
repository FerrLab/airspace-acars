package app

import (
	"fmt"
	"log/slog"
	"math/rand"
	"time"
)

var idlePhrasesI18n = map[string][]string{
	"en": {
		"Going through security at %s",
		"Checking weather in %s",
		"Grabbing coffee in %s",
		"Pre-flighting in %s",
		"Reviewing charts at %s",
		"In the crew lounge at %s",
		"Loading cargo in %s",
		"Briefing crew in %s",
	},
	"es": {
		"Pasando seguridad en %s",
		"Revisando el clima en %s",
		"Tomando un café en %s",
		"Pre-vuelo en %s",
		"Revisando cartas en %s",
		"En la sala de tripulación en %s",
		"Cargando en %s",
		"Briefing de tripulación en %s",
	},
	"pt": {
		"Passando pela segurança em %s",
		"Verificando o clima em %s",
		"Tomando um café em %s",
		"Pré-voo em %s",
		"Revisando cartas em %s",
		"Na sala da tripulação em %s",
		"Carregando em %s",
		"Briefing da tripulação em %s",
	},
	"fr": {
		"Passage de la sécurité à %s",
		"Vérification de la météo à %s",
		"Pause café à %s",
		"Pré-vol à %s",
		"Révision des cartes à %s",
		"Dans le salon équipage à %s",
		"Chargement à %s",
		"Briefing équipage à %s",
	},
}

var flyingToI18n = map[string]string{
	"en": "Flying %s",
	"es": "Volando %s",
	"pt": "Voando %s",
	"fr": "En vol %s",
}

var standbyI18n = map[string]string{
	"en": "Standing by for dispatch",
	"es": "En espera de despacho",
	"pt": "Aguardando despacho",
	"fr": "En attente de dispatch",
}

// Discord loop state (kept in App since the loop runs here)
var (
	discordPhraseIdx  = rand.Intn(len(idlePhrasesI18n["en"]))
	discordPhraseTime = time.Now()

	cachedTenantName    string
	cachedTenantLogoURL string
	cachedTenantBaseURL string

	bookingCache     map[string]interface{}
	bookingCacheTime time.Time

	pilotCache     map[string]interface{}
	pilotCacheTime time.Time

	lastDiscordConnectAttempt time.Time
)

// SetDiscordEnabled triggers an immediate Discord presence update.
func (a *App) SetDiscordEnabled(enabled bool) {
	select {
	case a.discordNudge <- struct{}{}:
	default:
	}
}

// StartDiscordLoop begins the background Discord presence loop.
func (a *App) StartDiscordLoop() {
	go a.discordRunLoop()
}

func (a *App) discordRunLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	time.Sleep(2 * time.Second)
	a.discordTick()

	for {
		select {
		case <-ticker.C:
			a.discordTick()
		case <-a.discordNudge:
			a.discordTick()
		}
	}
}

func (a *App) discordTick() {
	if !a.GetSettings().DiscordPresence {
		if a.Discord != nil && a.Discord.IsConnected() {
			a.Discord.Disconnect()
		}
		return
	}

	if a.Discord == nil {
		return
	}

	if !a.Discord.IsConnected() {
		if time.Since(lastDiscordConnectAttempt) < 30*time.Second {
			return
		}
		lastDiscordConnectAttempt = time.Now()
		if err := a.Discord.Connect(); err != nil {
			slog.Debug("discord: not available", "error", err)
			return
		}
	}

	tenantName, tenantLogo := a.resolveTenant()
	if tenantName == "" {
		return
	}

	activity := a.buildDiscordActivity(tenantName, tenantLogo)

	if err := a.Discord.SetActivity(activity); err != nil {
		slog.Debug("discord: activity update failed", "error", err)
		a.Discord.Disconnect()
	}
}

func (a *App) resolveTenant() (string, string) {
	baseURL := a.Airspace.BaseURL()
	if baseURL == "" {
		return "", ""
	}
	if baseURL == cachedTenantBaseURL && cachedTenantName != "" {
		return cachedTenantName, cachedTenantLogoURL
	}

	tenants, err := a.FetchTenants()
	if err != nil {
		return cachedTenantName, cachedTenantLogoURL
	}
	for _, t := range tenants {
		for _, dom := range t.Domains {
			if "https://"+dom == baseURL {
				cachedTenantBaseURL = baseURL
				cachedTenantName = t.Name
				if t.LogoURL != nil {
					cachedTenantLogoURL = *t.LogoURL
				}
				return cachedTenantName, cachedTenantLogoURL
			}
		}
	}
	return "", ""
}

func (a *App) discordLang() string {
	lang := a.GetSettings().Language
	if lang == "" {
		return "en"
	}
	return lang
}

func (a *App) buildDiscordActivity(tenantName, tenantLogo string) map[string]interface{} {
	a.flightMu.Lock()
	state := a.state
	callsign := a.callsign
	departure := a.departure
	arrival := a.arrival
	startTime := a.startTime
	a.flightMu.Unlock()

	lang := a.discordLang()
	activity := map[string]interface{}{}

	if tenantLogo != "" {
		activity["assets"] = map[string]interface{}{
			"large_image": tenantLogo,
			"large_text":  tenantName,
		}
	}

	if state == "active" {
		details := tenantName
		if callsign != "" {
			details = fmt.Sprintf("%s — %s", tenantName, callsign)
		}
		depCity := a.cityForFlight("departure_airport", departure)
		arrCity := a.cityForFlight("arrival_airport", arrival)
		flyFmt := flyingToI18n[lang]
		if flyFmt == "" {
			flyFmt = flyingToI18n["en"]
		}
		activity["details"] = details
		activity["state"] = fmt.Sprintf(flyFmt, depCity+" → "+arrCity)
		activity["timestamps"] = map[string]interface{}{
			"start": startTime.Unix(),
		}
	} else {
		activity["details"] = tenantName
		booking := a.getCachedBooking()
		depCode := a.airportField(booking, "departure_airport", "icao")
		if booking != nil && depCode != "" {
			dep := a.airportField(booking, "departure_airport", "city")
			if dep == "" {
				dep = depCode
			}
			activity["state"] = a.idlePhrase(dep)
		} else {
			city := a.pilotCity()
			activity["state"] = a.idlePhrase(city)
		}
	}

	return activity
}

func (a *App) getCachedBooking() map[string]interface{} {
	if time.Since(bookingCacheTime) < 60*time.Second {
		return bookingCache
	}
	booking, err := a.GetBooking()
	bookingCacheTime = time.Now()
	if err != nil || len(booking) == 0 {
		bookingCache = nil
		return nil
	}
	bookingCache = booking
	return booking
}

func (a *App) getCachedPilot() map[string]interface{} {
	if time.Since(pilotCacheTime) < 60*time.Second && pilotCache != nil {
		return pilotCache
	}
	pilot, err := a.GetPilot()
	pilotCacheTime = time.Now()
	if err != nil {
		slog.Debug("discord: failed to fetch pilot", "error", err)
		pilotCache = nil
		return nil
	}
	pilotCache = pilot
	return pilot
}

func (a *App) pilotCity() string {
	pilot := a.getCachedPilot()
	if pilot == nil {
		return ""
	}
	profile, _ := pilot["profile"].(map[string]interface{})
	if profile == nil {
		return ""
	}
	airport, _ := profile["current_airport"].(map[string]interface{})
	if airport == nil {
		return ""
	}
	if city, ok := airport["city"].(string); ok && city != "" {
		return city
	}
	if icao, ok := airport["icao"].(string); ok && icao != "" {
		return icao
	}
	return ""
}

func (a *App) airportField(booking map[string]interface{}, airportKey, field string) string {
	if booking == nil {
		return ""
	}
	airport, ok := booking[airportKey].(map[string]interface{})
	if !ok {
		return ""
	}
	if v, ok := airport[field].(string); ok {
		return v
	}
	return ""
}

func (a *App) cityForFlight(airportKey, fallbackCode string) string {
	if bookingCache != nil {
		if city := a.airportField(bookingCache, airportKey, "city"); city != "" {
			return city
		}
	}
	if fallbackCode != "" {
		return fallbackCode
	}
	return "unknown"
}

func (a *App) idlePhrase(city string) string {
	lang := a.discordLang()
	if city == "" {
		standby := standbyI18n[lang]
		if standby == "" {
			standby = standbyI18n["en"]
		}
		return standby
	}
	phrases := idlePhrasesI18n[lang]
	if len(phrases) == 0 {
		phrases = idlePhrasesI18n["en"]
	}
	if time.Since(discordPhraseTime) > 2*time.Minute {
		discordPhraseIdx = (discordPhraseIdx + 1) % len(phrases)
		discordPhraseTime = time.Now()
	}
	return fmt.Sprintf(phrases[discordPhraseIdx%len(phrases)], city)
}
