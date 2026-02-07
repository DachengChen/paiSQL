// Package cmd contains all Cobra commands for paiSQL.
//
// Design decision: the root command's RunE launches the TUI directly,
// so running `paiSQL` with no arguments starts the interactive UI.
// Subcommands (e.g. `paiSQL query "SELECT 1"`) can be added later
// for non-interactive / scripting use.
package cmd

import (
	"fmt"
	"os"

	"github.com/DachengChen/paiSQL/config"
	"github.com/DachengChen/paiSQL/tui"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     config.Config
)

var rootCmd = &cobra.Command{
	Use:   "paiSQL",
	Short: "PostgreSQL CLI with TUI and AI assistant",
	Long: `paiSQL is a PostgreSQL CLI tool featuring:
  • Multi-view TUI (SQL, Explain, Stats, Logs, AI)
  • pgx-based PostgreSQL connection (no psql dependency)
  • Optional SSH tunnel for remote servers
  • Keyboard-driven navigation`,
	// Running with no subcommand launches the TUI.
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Start(cfg)
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	// Connection flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: $HOME/.paisql.yaml)")
	rootCmd.PersistentFlags().StringVarP(&cfg.Host, "host", "H", "localhost", "PostgreSQL host")
	rootCmd.PersistentFlags().IntVarP(&cfg.Port, "port", "p", 5432, "PostgreSQL port")
	rootCmd.PersistentFlags().StringVarP(&cfg.User, "user", "U", "postgres", "PostgreSQL user")
	rootCmd.PersistentFlags().StringVarP(&cfg.Password, "password", "W", "", "PostgreSQL password")
	rootCmd.PersistentFlags().StringVarP(&cfg.Database, "dbname", "d", "postgres", "PostgreSQL database name")
	rootCmd.PersistentFlags().StringVar(&cfg.SSLMode, "sslmode", "prefer", "SSL mode (disable, require, verify-ca, verify-full)")

	// SSH tunnel flags
	rootCmd.PersistentFlags().BoolVar(&cfg.SSH.Enabled, "ssh", false, "enable SSH tunnel")
	rootCmd.PersistentFlags().StringVar(&cfg.SSH.Host, "ssh-host", "", "SSH server host")
	rootCmd.PersistentFlags().IntVar(&cfg.SSH.Port, "ssh-port", 22, "SSH server port")
	rootCmd.PersistentFlags().StringVar(&cfg.SSH.User, "ssh-user", "", "SSH user")
	rootCmd.PersistentFlags().StringVar(&cfg.SSH.KeyPath, "ssh-key", "", "path to SSH private key")
	rootCmd.PersistentFlags().StringVar(&cfg.SSH.KeyPassphrase, "ssh-key-passphrase", "", "passphrase for SSH key")
}

func initConfig() {
	// Future: load config from file (viper, etc.)
	// For now, CLI flags are the sole configuration source.
	if cfgFile != "" {
		fmt.Fprintf(os.Stderr, "config file loading not yet implemented: %s\n", cfgFile)
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
