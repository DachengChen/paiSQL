// Package tui implements the Bubble Tea terminal UI for paiSQL.
//
// Architecture:
//   - App is the top-level model that owns all views.
//   - On startup, the App shows the ConnectView for connection setup.
//   - After successful connection, it switches to the main multi-tab view.
//   - Database queries run asynchronously via tea.Cmd, never blocking
//     the UI event loop.
package tui

import (
	"fmt"

	"github.com/DachengChen/paiSQL/config"
	tea "github.com/charmbracelet/bubbletea"
)

// Start initializes the connection store and launches the TUI.
// No database connection is needed upfront — the user configures
// it in the connection screen.
func Start() error {
	// Load saved connections
	store, err := config.NewConnectionStore()
	if err != nil {
		return fmt.Errorf("failed to load connections: %w", err)
	}

	// Create and run the TUI (starts with connection screen)
	app := NewApp(store)
	p := tea.NewProgram(app, tea.WithAltScreen())

	_, err = p.Run()
	return err
}
