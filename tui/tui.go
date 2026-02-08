package tui

import (
	"fmt"
	"log"

	"github.com/DachengChen/paiSQL/ai"
	"github.com/DachengChen/paiSQL/applog"
	"github.com/DachengChen/paiSQL/config"
	tea "github.com/charmbracelet/bubbletea"
)

// Start initializes the connection store and launches the TUI.
func Start() error {
	applog.Event("APP", "paiSQL starting")

	store, err := config.NewConnectionStore()
	if err != nil {
		applog.Error("Failed to load connections: %v", err)
		return fmt.Errorf("failed to load connections: %w", err)
	}
	applog.Event("CONFIG", "Loaded %d saved connections", len(store.Connections))

	appCfg, err := config.LoadAppConfig()
	if err != nil {
		applog.Error("Failed to load config: %v", err)
		return fmt.Errorf("failed to load config: %w", err)
	}
	applog.Event("CONFIG", "App config loaded, AI provider: %s", appCfg.AI.Provider)

	provider, err := ai.NewProvider(appCfg.AI)
	if err != nil {
		log.Printf("AI provider warning: %v (using placeholder)", err)
		applog.Event("AI", "Provider init failed: %v, using placeholder", err)
		provider = ai.NewPlaceholder()
	} else {
		applog.Event("AI", "Provider initialized: %s", provider.Name())
	}

	app := NewApp(store, provider, appCfg)
	p := tea.NewProgram(app, tea.WithAltScreen())

	_, err = p.Run()
	applog.Event("APP", "paiSQL stopped")
	applog.Close()
	return err
}
