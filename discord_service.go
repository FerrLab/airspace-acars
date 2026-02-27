package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"os"
	"sync"
	"time"
)

const discordClientID = "1471884432234381494"

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
	"en": "Flying to %s",
	"es": "Volando a %s",
	"pt": "Voando para %s",
	"fr": "En vol vers %s",
}

var standbyI18n = map[string]string{
	"en": "Standing by for dispatch",
	"es": "En espera de despacho",
	"pt": "Aguardando despacho",
	"fr": "En attente de dispatch",
}

type DiscordService struct {
	settings *SettingsService
	auth     *AuthService
	flight   *FlightService

	mu        sync.Mutex
	pipe      *os.File
	connected bool
	nudge     chan struct{}

	// cached tenant info
	cachedTenantName    string
	cachedTenantLogoURL string
	cachedTenantBaseURL string

	// cached booking
	bookingCache     map[string]interface{}
	bookingCacheTime time.Time

	// idle phrase rotation
	phraseIdx  int
	phraseTime time.Time

	// rate-limit connect attempts
	lastConnectAttempt time.Time
}

func NewDiscordService(settings *SettingsService, auth *AuthService, flight *FlightService) *DiscordService {
	return &DiscordService{
		settings:   settings,
		auth:       auth,
		flight:     flight,
		nudge:      make(chan struct{}, 1),
		phraseIdx:  rand.Intn(len(idlePhrasesI18n["en"])),
		phraseTime: time.Now(),
	}
}

// Start begins the background presence loop. Called once at app startup.
func (d *DiscordService) Start() {
	go d.runLoop()
}

// SetEnabled is called from the frontend when the user toggles the setting.
// It triggers an immediate presence update.
func (d *DiscordService) SetEnabled(enabled bool) {
	select {
	case d.nudge <- struct{}{}:
	default:
	}
}

func (d *DiscordService) runLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	// small delay to let the app initialize
	time.Sleep(2 * time.Second)
	d.tick()

	for {
		select {
		case <-ticker.C:
			d.tick()
		case <-d.nudge:
			d.tick()
		}
	}
}

func (d *DiscordService) tick() {
	if !d.settings.GetSettings().DiscordPresence {
		d.mu.Lock()
		d.disconnect()
		d.mu.Unlock()
		return
	}

	// ensure connected
	d.mu.Lock()
	if !d.connected {
		if time.Since(d.lastConnectAttempt) < 30*time.Second {
			d.mu.Unlock()
			return
		}
		d.lastConnectAttempt = time.Now()
		if err := d.connect(); err != nil {
			d.mu.Unlock()
			slog.Debug("discord: not available", "error", err)
			return
		}
	}
	d.mu.Unlock()

	// resolve tenant
	tenantName, tenantLogo := d.resolveTenant()
	if tenantName == "" {
		return
	}

	// build activity from current state
	activity := d.buildActivity(tenantName, tenantLogo)

	// send to discord
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.connected {
		return
	}
	if err := d.setActivity(activity); err != nil {
		slog.Debug("discord: activity update failed", "error", err)
		d.disconnect()
	}
}

// --- IPC protocol ---

func (d *DiscordService) connect() error {
	for i := 0; i < 10; i++ {
		path := fmt.Sprintf(`\\.\pipe\discord-ipc-%d`, i)
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			continue
		}
		d.pipe = f

		hs, _ := json.Marshal(map[string]interface{}{
			"v":         1,
			"client_id": discordClientID,
		})
		if err := d.writeFrame(0, hs); err != nil {
			f.Close()
			d.pipe = nil
			continue
		}
		if _, err := d.readFrame(); err != nil {
			f.Close()
			d.pipe = nil
			continue
		}

		d.connected = true
		slog.Info("discord: connected", "pipe", i)
		return nil
	}
	return fmt.Errorf("no discord pipe found")
}

func (d *DiscordService) disconnect() {
	if d.pipe != nil {
		d.pipe.Close()
		d.pipe = nil
	}
	d.connected = false
}

func (d *DiscordService) writeFrame(opcode uint32, payload []byte) error {
	hdr := make([]byte, 8)
	binary.LittleEndian.PutUint32(hdr[0:4], opcode)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(len(payload)))
	if _, err := d.pipe.Write(hdr); err != nil {
		return err
	}
	_, err := d.pipe.Write(payload)
	return err
}

func (d *DiscordService) readFrame() (json.RawMessage, error) {
	hdr := make([]byte, 8)
	if _, err := io.ReadFull(d.pipe, hdr); err != nil {
		return nil, err
	}
	length := binary.LittleEndian.Uint32(hdr[4:8])
	buf := make([]byte, length)
	if _, err := io.ReadFull(d.pipe, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func (d *DiscordService) setActivity(activity map[string]interface{}) error {
	payload, _ := json.Marshal(map[string]interface{}{
		"cmd":   "SET_ACTIVITY",
		"nonce": fmt.Sprintf("%d", time.Now().UnixNano()),
		"args": map[string]interface{}{
			"pid":      os.Getpid(),
			"activity": activity,
		},
	})
	if err := d.writeFrame(1, payload); err != nil {
		return err
	}
	_, err := d.readFrame()
	return err
}

// --- State helpers ---

func (d *DiscordService) resolveTenant() (string, string) {
	d.auth.mu.RLock()
	baseURL := d.auth.tenantBaseURL
	d.auth.mu.RUnlock()

	if baseURL == "" {
		return "", ""
	}
	if baseURL == d.cachedTenantBaseURL && d.cachedTenantName != "" {
		return d.cachedTenantName, d.cachedTenantLogoURL
	}

	tenants, err := d.auth.FetchTenants()
	if err != nil {
		return d.cachedTenantName, d.cachedTenantLogoURL
	}
	for _, t := range tenants {
		for _, dom := range t.Domains {
			if "https://"+dom == baseURL {
				d.cachedTenantBaseURL = baseURL
				d.cachedTenantName = t.Name
				if t.LogoURL != nil {
					d.cachedTenantLogoURL = *t.LogoURL
				}
				return d.cachedTenantName, d.cachedTenantLogoURL
			}
		}
	}
	return "", ""
}

func (d *DiscordService) lang() string {
	lang := d.settings.GetSettings().Language
	if lang == "" {
		return "en"
	}
	return lang
}

func (d *DiscordService) buildActivity(tenantName, tenantLogo string) map[string]interface{} {
	d.flight.mu.Lock()
	state := d.flight.state
	callsign := d.flight.callsign
	arrival := d.flight.arrival
	startTime := d.flight.startTime
	d.flight.mu.Unlock()

	lang := d.lang()
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
		arrCity := d.cityFromBooking("arrival", arrival)
		flyFmt := flyingToI18n[lang]
		if flyFmt == "" {
			flyFmt = flyingToI18n["en"]
		}
		activity["details"] = details
		activity["state"] = fmt.Sprintf(flyFmt, arrCity)
		activity["timestamps"] = map[string]interface{}{
			"start": startTime.Unix(),
		}
	} else {
		activity["details"] = tenantName
		booking := d.getCachedBooking()
		if booking != nil {
			dep := d.cityFromBooking("departure", d.bookingField(booking, "departure", "dep"))
			activity["state"] = d.idlePhrase(dep)
		} else {
			standby := standbyI18n[lang]
			if standby == "" {
				standby = standbyI18n["en"]
			}
			activity["state"] = standby
		}
	}

	return activity
}

func (d *DiscordService) getCachedBooking() map[string]interface{} {
	if time.Since(d.bookingCacheTime) < 60*time.Second && d.bookingCache != nil {
		return d.bookingCache
	}
	booking, err := d.flight.GetBooking()
	d.bookingCacheTime = time.Now()
	if err != nil {
		d.bookingCache = nil
		return nil
	}
	d.bookingCache = booking
	return booking
}

func (d *DiscordService) bookingField(booking map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := booking[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func (d *DiscordService) cityFromBooking(which, fallbackCode string) string {
	if d.bookingCache != nil {
		if city, ok := d.bookingCache[which+"_city"].(string); ok && city != "" {
			return city
		}
	}
	if fallbackCode != "" {
		return fallbackCode
	}
	return "unknown"
}

func (d *DiscordService) idlePhrase(city string) string {
	lang := d.lang()
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
	if time.Since(d.phraseTime) > 2*time.Minute {
		d.phraseIdx = (d.phraseIdx + 1) % len(phrases)
		d.phraseTime = time.Now()
	}
	return fmt.Sprintf(phrases[d.phraseIdx%len(phrases)], city)
}
