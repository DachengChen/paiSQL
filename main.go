// paiSQL – PostgreSQL CLI with TUI and AI assistant.
//
// Entry point: initializes Cobra root command and launches
// the Bubble Tea TUI by default (no subcommand required).
package main

import (
	"bufio"
	"os"
	"strings"

	"github.com/DachengChen/paiSQL/cmd"
)

func main() {
	loadEnv(".env")
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// loadEnv reads a .env file and sets variables via os.Setenv.
// Silently skips if the file doesn't exist.
func loadEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip optional "export " prefix
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		// Don't overwrite existing env vars
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}
