// Package cmd contains all Cobra commands for paiSQL.
//
// Design decision: the root command launches the TUI directly.
// Connection configuration happens inside the TUI, not via CLI flags.
// Running `paisql` with no arguments starts the interactive UI
// with a connection setup screen.
package cmd

import (
	"github.com/DachengChen/paiSQL/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "paisql",
	Short: "PostgreSQL CLI with TUI and AI assistant",
	Long: `paiSQL is a PostgreSQL CLI tool featuring:
  • Multi-view TUI (SQL, Explain, Stats, Logs, AI)
  • pgx-based PostgreSQL connection (no psql dependency)
  • Optional SSH tunnel for remote servers
  • Keyboard-driven navigation

Run 'paisql' to start the TUI with a connection setup screen.`,
	// Running with no subcommand launches the TUI.
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Start()
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
