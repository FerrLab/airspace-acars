package ports

import (
	"airspace-acars/internal/app"
	"airspace-acars/internal/domain"
)

// UserActionPort is the entry point for all user-initiated actions.
// It wraps the App layer and is registered as a Wails service, exposing
// methods to the frontend. It can also serve HTTP or gRPC in the future.
type UserActionPort struct {
	App *app.App
}

// NewUserActionPort creates a new user action port.
func NewUserActionPort(a *app.App) *UserActionPort {
	return &UserActionPort{App: a}
}

// --- Sim connection commands ---

func (p *UserActionPort) ConnectSim(simType string) (string, error) {
	return p.App.ConnectSim(simType)
}

func (p *UserActionPort) DisconnectSim() {
	p.App.DisconnectSim()
}

func (p *UserActionPort) IsConnected() bool {
	return p.App.IsConnected()
}

func (p *UserActionPort) ConnectedAdapter() string {
	return p.App.ConnectedAdapter()
}

func (p *UserActionPort) GetFlightDataNow() (*domain.FlightData, error) {
	return p.App.GetFlightDataNow()
}

// --- Recording commands ---

func (p *UserActionPort) StartRecording() error {
	return p.App.StartRecording()
}

func (p *UserActionPort) StopRecording() {
	p.App.StopRecording()
}

func (p *UserActionPort) IsRecording() bool {
	return p.App.IsRecording()
}

func (p *UserActionPort) GetRecordingInfo() map[string]interface{} {
	return p.App.GetRecordingInfo()
}

func (p *UserActionPort) ExportCSV(filePath string) error {
	return p.App.ExportCSV(filePath)
}

// --- Flight commands ---

func (p *UserActionPort) GetFlightState() string {
	return p.App.GetFlightState()
}

func (p *UserActionPort) GetActiveFlightInfo() map[string]string {
	return p.App.GetActiveFlightInfo()
}

func (p *UserActionPort) GetBooking() (map[string]interface{}, error) {
	return p.App.GetBooking()
}

func (p *UserActionPort) GetPilot() (map[string]interface{}, error) {
	return p.App.GetPilot()
}

func (p *UserActionPort) StartFlight(callsign, departure, arrival, bookingID string) error {
	return p.App.StartFlight(callsign, departure, arrival, bookingID)
}

func (p *UserActionPort) StopFlight() error {
	return p.App.StopFlight()
}

func (p *UserActionPort) FinishFlight() error {
	return p.App.FinishFlight()
}

func (p *UserActionPort) CancelFinish() error {
	return p.App.CancelFinish()
}

// --- Auth commands ---

func (p *UserActionPort) FetchTenants() ([]domain.Tenant, error) {
	return p.App.FetchTenants()
}

func (p *UserActionPort) SelectTenant(d string) {
	p.App.SelectTenant(d)
}

func (p *UserActionPort) RequestDeviceCode() (*domain.DeviceCodeResponse, error) {
	return p.App.RequestDeviceCode()
}

func (p *UserActionPort) PollForToken(authorizationToken string) (*domain.TokenResponse, error) {
	return p.App.PollForToken(authorizationToken)
}

func (p *UserActionPort) OpenAuthorizationURL(userCode string) error {
	return p.App.OpenAuthorizationURL(userCode)
}

func (p *UserActionPort) SetToken(token string) {
	p.App.SetToken(token)
}

// --- Chat commands ---

func (p *UserActionPort) GetMessages(page int) (*domain.MessagesResponse, error) {
	return p.App.GetMessages(page)
}

func (p *UserActionPort) SendMessage(message string) (*domain.ChatMessage, error) {
	return p.App.SendMessage(message)
}

func (p *UserActionPort) ConfirmMessage(messageID int) error {
	return p.App.ConfirmMessage(messageID)
}

// --- Audio queries ---

func (p *UserActionPort) FetchSoundInstructions() ([]domain.SoundInstruction, error) {
	return p.App.FetchSoundInstructions()
}

func (p *UserActionPort) GetAudioData(filename string) (*domain.AudioData, error) {
	return p.App.GetAudioData(filename)
}

func (p *UserActionPort) ClearCache() {
	p.App.ClearCache()
}

// --- Settings commands ---

func (p *UserActionPort) GetSettings() domain.Settings {
	return p.App.GetSettings()
}

func (p *UserActionPort) UpdateSettings(settings domain.Settings) error {
	return p.App.UpdateSettings(settings)
}

// --- Update commands ---

func (p *UserActionPort) GetCurrentVersion() string {
	return p.App.GetCurrentVersion()
}

func (p *UserActionPort) CheckForUpdate() (*domain.UpdateInfo, error) {
	return p.App.CheckForUpdate()
}

func (p *UserActionPort) ApplyUpdate() error {
	return p.App.ApplyUpdate()
}

// --- Logs ---

func (p *UserActionPort) TailLogs(n int) ([]string, error) {
	return p.App.TailLogs(n)
}

// --- Discord ---

func (p *UserActionPort) SetEnabled(enabled bool) {
	p.App.SetDiscordEnabled(enabled)
}
