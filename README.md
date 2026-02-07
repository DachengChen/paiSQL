# paiSQL

[![CI](https://github.com/DachengChen/paiSQL/actions/workflows/ci.yml/badge.svg)](https://github.com/DachengChen/paiSQL/actions/workflows/ci.yml)
[![Release](https://github.com/DachengChen/paiSQL/actions/workflows/release.yml/badge.svg)](https://github.com/DachengChen/paiSQL/actions/workflows/release.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/DachengChen/paiSQL)](https://go.dev/)
[![License](https://img.shields.io/github/license/DachengChen/paiSQL)](LICENSE)

PostgreSQL CLI with TUI and AI assistant — featuring multi-view navigation, tail logs, explain plans, index suggestions, and keyboard-driven interface.

## Features

- **pgx-based** — connects directly to PostgreSQL via pgx (no `psql` dependency)
- **TUI connection manager** — configure, save, and select database connections in the TUI
- **SSH tunnel** — optional local port forwarding for remote databases
- **Multi-LLM AI assistant** — OpenAI, Anthropic, Google Gemini, and Ollama (local) support
- **6 TUI views** — SQL, Explain, Index, Stats, Log, AI
- **psql-like commands** — `\dt`, `\di`, `\dv`, `\d <table>`, `\set`
- **Async queries** — database and AI operations never block the UI
- **Keyboard-driven** — tab switching, command mode, jump mode, help overlay

## Installation

### Using `go install`

```bash
go install github.com/DachengChen/paiSQL@latest
```

### Download from GitHub Releases

Download the latest binary for your platform from the [Releases page](https://github.com/DachengChen/paiSQL/releases):

| Platform | File |
|---|---|
| macOS (Apple Silicon) | `paisql_darwin_arm64.tar.gz` |
| macOS (Intel) | `paisql_darwin_amd64.tar.gz` |
| Linux (x86_64) | `paisql_linux_amd64.tar.gz` |
| Windows (x86_64) | `paisql_windows_amd64.zip` |

```bash
# Example: macOS Apple Silicon
curl -sSL https://github.com/DachengChen/paiSQL/releases/latest/download/paisql_darwin_arm64.tar.gz | tar xz
sudo mv paisql /usr/local/bin/
```

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
| `F2` | Toggle input between Chat and SQL |
| `Tab` / `Shift+Tab` | Switch between views |
| `1-6` | Jump to view by number |
| `:` | Command mode (`:dt`, `:quit`, `:disconnect`) |
| `/` | Jump to view by name |
| `?` | Toggle help overlay |
| `Enter` | Execute query / send chat |
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

## Linting

Run lint checks locally before pushing to catch CI failures early:

```bash
# Run lint (mirrors CI: golangci-lint v1.64 + Go 1.24)
scripts/lint-local.sh

# Auto-fix lint issues
scripts/lint-local.sh --fix

# Verbose output
scripts/lint-local.sh --verbose
```

The script automatically downloads the exact `golangci-lint` version used in CI and uses `goenv` to ensure the correct Go version.

## AI Providers

paiSQL supports multiple AI backends for the built-in AI assistant. Configure via `~/.paisql/config.json` or environment variables.

### Quick Setup

```bash
# Option 1: Environment variable (recommended)
export OPENAI_API_KEY="sk-..."

# Option 2: Config file (~/.paisql/config.json)
```

```json
{
  "ai": {
    "provider": "openai",
    "openai":    { "api_key": "sk-...",     "model": "gpt-4o" },
    "anthropic": { "api_key": "sk-ant-...", "model": "claude-sonnet-4-20250514" },
    "gemini":    { "api_key": "AI...",      "model": "gemini-2.0-flash" },
    "ollama":    { "host": "http://localhost:11434", "model": "llama3.2" }
  }
}
```

### Supported Providers

| Provider | Env Variable | Models | Notes |
|---|---|---|---|
| **OpenAI** | `OPENAI_API_KEY` | gpt-4o, gpt-4o-mini, etc. | Default cloud option |
| **Anthropic** | `ANTHROPIC_API_KEY` | claude-sonnet-4-20250514, etc. | |
| **Gemini** | `GEMINI_API_KEY` | gemini-2.0-flash, gemini-2.5-pro, etc. | Google AI |
| **Ollama** | `OLLAMA_HOST` | llama3.2, codellama, etc. | Free, runs locally |
| **Antigravity** | — | gemini-2.0-flash, etc. | Free, uses Google OAuth login |

### Antigravity (Google OAuth)

Antigravity lets you use Gemini models for free by logging in with your Google account — no API key needed. Select "antigravity" as the provider in the AI Settings panel and click "Login with Google".

**For developers building from source:** The OAuth Client ID and Secret are injected at build time via `-ldflags` (see `.goreleaser.yaml` and `.github/workflows/ci.yml`). These are the same public credentials used by the [Gemini CLI](https://github.com/google-gemini/gemini-cli). You can find them in the Gemini CLI source code:

- **File:** [`packages/core/src/code_assist/oauth2.ts`](https://github.com/google-gemini/gemini-cli/blob/main/packages/core/src/code_assist/oauth2.ts)
- **Google's note:** *"It's ok to save this in git because this is an installed application"* — [Google OAuth2 docs](https://developers.google.com/identity/protocols/oauth2#installed)

To set up locally:
```bash
# 1. Copy the example and fill in values from the Gemini CLI source
cp .env.example .env

# 2. paiSQL auto-loads .env on startup — just run it
go run .
```

## Saved Connections

Connections are saved to `~/.paisql/connections.json`. You can save, load, and delete connections directly from the TUI connection screen.

---

*Built with assistance from [Antigravity](https://deepmind.google/) 🚀*
