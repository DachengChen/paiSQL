package tui

import (
	"fmt"
	"log"

	"github.com/DachengChen/paiSQL/ai"
	"github.com/DachengChen/paiSQL/config"
	tea "github.com/charmbracelet/bubbletea"
)

// Start initializes the connection store and launches the TUI.
func Start() error {
	store, err := config.NewConnectionStore()
	if err != nil {
		return fmt.Errorf("failed to load connections: %w", err)
	}

	appCfg, err := config.LoadAppConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	provider, err := ai.NewProvider(appCfg.AI)
	if err != nil {
		log.Printf("AI provider warning: %v (using placeholder)", err)
		provider = ai.NewPlaceholder()
	}

	app := NewApp(store, provider, appCfg)
	p := tea.NewProgram(app, tea.WithAltScreen())

	_, err = p.Run()
	return err
}
