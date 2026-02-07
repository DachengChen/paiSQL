// paiSQL – PostgreSQL CLI with TUI and AI assistant.
//
// Entry point: initializes Cobra root command and launches
// the Bubble Tea TUI by default (no subcommand required).
package main

import (
	"os"

	"github.com/DachengChen/paiSQL/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
