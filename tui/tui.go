// Package tui implements the Bubble Tea terminal UI for paiSQL.
//
// Architecture:
//   - App is the top-level model that owns all views.
//   - Each view (SQL, Explain, Stats, Log, AI, Index) is a sub-model
//     that implements the View interface.
//   - Tab switching, command mode, and help overlay are handled at
//     the App level.
//   - Database queries run asynchronously via tea.Cmd, never blocking
//     the UI event loop.
package tui

import (
	"context"
	"fmt"

	"github.com/DachengChen/paiSQL/ai"
	"github.com/DachengChen/paiSQL/config"
	"github.com/DachengChen/paiSQL/db"
	tea "github.com/charmbracelet/bubbletea"
)

// Start initializes the database connection and launches the TUI.
func Start(cfg config.Config) error {
	ctx := context.Background()

	// Connect to PostgreSQL
	database, err := db.Connect(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer database.Close()

	// Initialize AI provider (placeholder for now)
	aiProvider := ai.NewPlaceholder()

	// Create and run the TUI
	app := NewApp(database, aiProvider, cfg)
	p := tea.NewProgram(app, tea.WithAltScreen())

	_, err = p.Run()
	return err
}
