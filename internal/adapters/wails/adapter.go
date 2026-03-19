package wails

import "github.com/wailsapp/wails/v3/pkg/application"

// Adapter wraps the Wails application for event emission.
type Adapter struct {
	app *application.App
}

// NewAdapter creates a new Wails event emitter.
func NewAdapter() *Adapter {
	return &Adapter{}
}

// SetApp sets the Wails application reference (called after app creation).
func (w *Adapter) SetApp(app *application.App) {
	w.app = app
}

// EmitEvent sends an event to the frontend via Wails.
func (w *Adapter) EmitEvent(name string, data interface{}) {
	if w.app != nil {
		w.app.Event.Emit(name, data)
	}
}
