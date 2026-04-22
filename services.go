package main

// This file defines thin Wails service wrappers that delegate to the App layer.
// They exist in package main because Wails v3 binding generation requires
// service types to be defined in the scanned package.
//
// These are the "User Action Port" from the hexagonal architecture,
// split into domain-specific services to preserve frontend binding compatibility.

import (
	"airspace-acars/internal/app"
	"airspace-acars/internal/domain"
)

// --- FlightDataService: sim connection, data streaming, recording ---

type FlightDataService struct{ app *app.App }

func (s *FlightDataService) ConnectSim(simType string) (string, error) {
	return s.app.ConnectSim(simType)
}
func (s *FlightDataService) DisconnectSim()              { s.app.DisconnectSim() }
func (s *FlightDataService) IsConnected() bool            { return s.app.IsConnected() }
func (s *FlightDataService) ConnectedAdapter() string     { return s.app.ConnectedAdapter() }
func (s *FlightDataService) GetFlightDataNow() (*domain.FlightData, error) {
	return s.app.GetFlightDataNow()
}
func (s *FlightDataService) StartRecording() error        { return s.app.StartRecording() }
func (s *FlightDataService) StopRecording()               { s.app.StopRecording() }
func (s *FlightDataService) IsRecording() bool            { return s.app.IsRecording() }
func (s *FlightDataService) GetRecordingInfo() map[string]interface{} {
	return s.app.GetRecordingInfo()
}
func (s *FlightDataService) ExportCSV(filePath string) error { return s.app.ExportCSV(filePath) }

// --- FlightService: flight state, start/stop/finish, booking, pilot ---

type FlightService struct{ app *app.App }

func (s *FlightService) GetFlightState() string            { return s.app.GetFlightState() }
func (s *FlightService) GetActiveFlightInfo() map[string]string {
	return s.app.GetActiveFlightInfo()
}
func (s *FlightService) GetBooking() (map[string]interface{}, error) { return s.app.GetBooking() }
func (s *FlightService) GetPilot() (map[string]interface{}, error)   { return s.app.GetPilot() }
func (s *FlightService) StartFlight(callsign, departure, arrival, bookingID string) error {
	return s.app.StartFlight(callsign, departure, arrival, bookingID)
}
func (s *FlightService) StopFlight() error   { return s.app.StopFlight() }
func (s *FlightService) FinishFlight() error { return s.app.FinishFlight() }
func (s *FlightService) CancelFinish() error { return s.app.CancelFinish() }

// --- AuthService: tenant selection, device-code OAuth ---

type AuthService struct{ app *app.App }

func (s *AuthService) FetchTenants() ([]domain.Tenant, error)    { return s.app.FetchTenants() }
func (s *AuthService) SelectTenant(d string)                      { s.app.SelectTenant(d) }
func (s *AuthService) RequestDeviceCode() (*domain.DeviceCodeResponse, error) {
	return s.app.RequestDeviceCode()
}
func (s *AuthService) PollForToken(authToken string) (*domain.TokenResponse, error) {
	return s.app.PollForToken(authToken)
}
func (s *AuthService) OpenAuthorizationURL(userCode string) error {
	return s.app.OpenAuthorizationURL(userCode)
}
func (s *AuthService) SetToken(token string) { s.app.SetToken(token) }

// --- ChatService: messages ---

type ChatService struct{ app *app.App }

func (s *ChatService) GetMessages(page int) (*domain.MessagesResponse, error) {
	return s.app.GetMessages(page)
}
func (s *ChatService) SendMessage(message string) (*domain.ChatMessage, error) {
	return s.app.SendMessage(message)
}
func (s *ChatService) ConfirmMessage(messageID int) error { return s.app.ConfirmMessage(messageID) }

// --- AudioService: sound instructions, cached audio ---

type AudioService struct{ app *app.App }

func (s *AudioService) FetchSoundInstructions() ([]domain.SoundInstruction, error) {
	return s.app.FetchSoundInstructions()
}
func (s *AudioService) GetAudioData(filename string) (*domain.AudioData, error) {
	return s.app.GetAudioData(filename)
}
func (s *AudioService) ClearCache() { s.app.ClearCache() }

// --- SettingsService: user preferences ---

type SettingsService struct{ app *app.App }

func (s *SettingsService) GetSettings() domain.Settings          { return s.app.GetSettings() }
func (s *SettingsService) UpdateSettings(settings domain.Settings) error {
	return s.app.UpdateSettings(settings)
}

// --- UpdateService: self-update ---

type UpdateService struct{ app *app.App }

func (s *UpdateService) GetCurrentVersion() string                    { return s.app.GetCurrentVersion() }
func (s *UpdateService) CheckForUpdate() (*domain.UpdateInfo, error)  { return s.app.CheckForUpdate() }
func (s *UpdateService) ApplyUpdate() error                           { return s.app.ApplyUpdate() }
func (s *UpdateService) TailLogs(n int) ([]string, error)             { return s.app.TailLogs(n) }

// --- DiscordService: presence toggle ---

type DiscordService struct{ app *app.App }

func (s *DiscordService) SetEnabled(enabled bool) { s.app.SetDiscordEnabled(enabled) }
