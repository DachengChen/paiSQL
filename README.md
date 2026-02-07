# paiSQL

PostgreSQL CLI with TUI and AI assistant — featuring multi-view navigation, tail logs, explain plans, index suggestions, and keyboard-driven interface.

## Features

- **pgx-based** — connects directly to PostgreSQL via pgx (no `psql` dependency)
- **SSH tunnel** — optional local port forwarding for remote databases
- **6 TUI views** — SQL, Explain, Index, Stats, Log, AI
- **psql-like commands** — `\dt`, `\di`, `\dv`, `\d <table>`, `\set`
- **Async queries** — database and AI operations never block the UI
- **Variable substitution** — set and use variables in queries
- **Keyboard-driven** — tab switching, command mode, jump mode, help overlay

## Quick Start

```bash
# Build
go build -o bin/paiSQL .

# Connect to local PostgreSQL
./bin/paiSQL -H localhost -p 5432 -U docker -d ports -W docker

# Connect via SSH tunnel
./bin/paiSQL \
  --ssh --ssh-host bastion.example.com --ssh-user deploy \
  --ssh-key ~/.ssh/id_rsa \
  -H db-internal -p 5432 -U myuser -d mydb
```

## Keyboard Shortcuts

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Switch between views |
| `1-6` | Jump to view by number |
| `:` | Command mode |
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
│   └── root.go      # Root command with DB/SSH flags
├── config/          # Shared configuration structs
│   └── config.go
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
    ├── app.go          # Root model (tabs, command mode, help)
    ├── view.go         # View interface
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

## Configuration

| Flag | Short | Default | Description |
|---|---|---|---|
| `--host` | `-H` | `localhost` | PostgreSQL host |
| `--port` | `-p` | `5432` | PostgreSQL port |
| `--user` | `-U` | `postgres` | PostgreSQL user |
| `--password` | `-W` | | PostgreSQL password |
| `--dbname` | `-d` | `postgres` | Database name |
| `--sslmode` | | `prefer` | SSL mode |
| `--ssh` | | `false` | Enable SSH tunnel |
| `--ssh-host` | | | SSH server host |
| `--ssh-port` | | `22` | SSH server port |
| `--ssh-user` | | | SSH user |
| `--ssh-key` | | | Path to SSH private key |
