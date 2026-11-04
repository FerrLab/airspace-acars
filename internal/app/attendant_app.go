package app

import (
	"fmt"

	"airspace-acars/internal/domain"
)

// GetAttendantSnapshot is everything the Flight Attendant tab shows.
func (a *App) GetAttendantSnapshot() domain.AttendantSnapshot {
	return a.attendant.Snapshot()
}

// TakeAttendantClip hands the webview's player the next clip to play, or
// nil when nothing is waiting.
func (a *App) TakeAttendantClip() *domain.AttendantClip {
	return a.attendant.TakeClip()
}

// AttendantClipFinished tells the attendant a clip played to the end.
func (a *App) AttendantClipFinished(id string) { a.attendant.ClipEnded(id, true) }

// AttendantClipFailed tells the attendant a clip could not be played.
func (a *App) AttendantClipFailed(id string) { a.attendant.ClipEnded(id, false) }

// SkipAttendantClip cuts the current announcement short.
func (a *App) SkipAttendantClip() { a.attendant.Skip() }

// ReplayAttendantClip plays the last announcement again.
func (a *App) ReplayAttendantClip() { a.attendant.Replay() }

// StopAttendant silences the cabin until the next phase.
func (a *App) StopAttendant() { a.attendant.StopAll() }

// PlayAttendantPhase plays a phase's lines now, for trying the pack out.
// service picks a meal service, 0 for any other phase.
func (a *App) PlayAttendantPhase(phase string, service int) error {
	return a.attendant.PlayPhaseNow(phase, service)
}

// SetAttendantEnabled turns the flight attendant on or off and remembers it.
func (a *App) SetAttendantEnabled(enabled bool) error {
	a.settingsMu.Lock()
	a.settings.FlightAttendant = enabled
	err := a.saveSettings()
	a.settingsMu.Unlock()

	a.attendant.SetEnabled(enabled)
	if err != nil {
		return fmt.Errorf("save flight attendant setting: %w", err)
	}
	return nil
}
