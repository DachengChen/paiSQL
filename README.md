# paiSQL

PostgreSQL CLI with TUI and AI assistant — featuring multi-view navigation, tail logs, explain plans, index suggestions, and keyboard-driven interface.

## Features

- **pgx-based** — connects directly to PostgreSQL via pgx (no `psql` dependency)
- **TUI connection manager** — configure, save, and select database connections in the TUI
- **SSH tunnel** — optional local port forwarding for remote databases
- **6 TUI views** — SQL, Explain, Index, Stats, Log, AI
- **psql-like commands** — `\dt`, `\di`, `\dv`, `\d <table>`, `\set`
- **Async queries** — database and AI operations never block the UI
- **Keyboard-driven** — tab switching, command mode, jump mode, help overlay

## Quick Start

```bash
# Build
go build -o bin/paisql .

# Start the TUI (opens connection setup screen)
./bin/paisql

# Or use air for auto-reload during development
air
```

## Keyboard Shortcuts

### Connection Screen

| Key | Action |
|---|---|
| `↑/↓` | Navigate fields |
| `Enter` | Edit field / toggle / action |
| `Esc` | Stop editing |
| `←/→` | Switch saved connection / cycle SSL mode |
| `Tab` | Jump to Connect button |
| `Ctrl+C` | Quit |

### Main View

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Switch between views |
| `1-6` | Jump to view by number |
| `:` | Command mode (`:dt`, `:quit`, `:disconnect`) |
| `/` | Jump to view by name |
| `?` | Toggle help overlay |
| `Enter` | Execute query |
| `Ctrl+K/J` | Scroll up/down |
| `Ctrl+H/L` | Scroll left/right |
| `PgUp/PgDn` | Page up/down |
| `Ctrl+W` | Toggle text wrapping |
| `q` / `Ctrl+C` | Quit |

## Project Structure

```
├── main.go          # Entry point
├── cmd/             # Cobra CLI commands
│   └── root.go      # Root command → launches TUI
├── config/          # Configuration & saved connections
│   ├── config.go       # Runtime config structs
│   └── connections.go  # Saved connections (~/.paisql/connections.json)
├── db/              # pgx connection and queries
│   ├── connection.go   # Connection pool + SSH tunnel integration
│   ├── query.go        # psql-like meta-commands + SQL execution
│   └── variables.go    # \set variable substitution
├── ssh/             # SSH tunnel management
│   └── tunnel.go       # Local port forwarding
├── ai/              # AI provider interface
│   ├── provider.go     # Provider interface
│   └── placeholder.go  # Mock provider for development
└── tui/             # Bubble Tea terminal UI
    ├── tui.go          # TUI entry point
    ├── app.go          # Root model (phases, tabs, commands)
    ├── view.go         # View interface
    ├── view_connect.go # Connection setup form
    ├── viewport.go     # Scrollable viewport component
    ├── styles.go       # Color palette and shared styles
    ├── messages.go     # Async message types
    ├── view_sql.go     # SQL query view
    ├── view_explain.go # EXPLAIN plan view
    ├── view_index.go   # Index suggestions view
    ├── view_stats.go   # Database statistics view
    ├── view_log.go     # Activity tail log view
    └── view_ai.go      # AI assistant chat view
```

## Saved Connections

Connections are saved to `~/.paisql/connections.json`. You can save, load, and delete connections directly from the TUI connection screen.
