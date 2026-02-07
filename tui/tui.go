package tui

import (
	"fmt"

	"github.com/DachengChen/paiSQL/config"
	tea "github.com/charmbracelet/bubbletea"
)

// Start initializes the connection store and launches the TUI.
func Start() error {
	store, err := config.NewConnectionStore()
	if err != nil {
		return fmt.Errorf("failed to load connections: %w", err)
	}

	app := NewApp(store)
	p := tea.NewProgram(app, tea.WithAltScreen())

	_, err = p.Run()
	return err
}
