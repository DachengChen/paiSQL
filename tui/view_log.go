// view_log.go — Tail log view for continuous streaming.
//
// Streams pg_stat_activity to show live queries. Refreshes
// periodically using tea.Tick. The user can pause/resume streaming.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DachengChen/paiSQL/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const logRefreshInterval = 2 * time.Second

type LogView struct {
	db       *db.DB
	viewport *Viewport
	lines    []string
	paused   bool
	loading  bool
	width    int
	height   int
}

func NewLogView(database *db.DB) *LogView {
	return &LogView{
		db:       database,
		viewport: NewViewport(80, 20),
	}
}

func (v *LogView) Name() string         { return "Log" }
func (v *LogView) WantsTextInput() bool { return false }

func (v *LogView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.viewport.SetSize(width-2, height-2)
}

func (v *LogView) ShortHelp() []KeyBinding {
	pause := "pause"
	if v.paused {
		pause = "resume"
	}
	return []KeyBinding{
		{Key: "p", Desc: pause},
		{Key: "c", Desc: "clear"},
		{Key: "↑/↓", Desc: "scroll"},
	}
}

// tickMsg triggers periodic refresh.
type tickMsg time.Time

func (v *LogView) Init() tea.Cmd {
	return tea.Batch(v.fetchLog(), v.tick())
}

func (v *LogView) tick() tea.Cmd {
	return tea.Tick(logRefreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (v *LogView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return v.handleKey(msg)

	case tickMsg:
		if !v.paused {
			return v, tea.Batch(v.fetchLog(), v.tick())
		}
		return v, v.tick()

	case LogMsg:
		v.loading = false
		if msg.Err != nil {
			v.lines = append(v.lines, StyleError.Render("ERROR: "+msg.Err.Error()))
		} else if msg.Line != "" {
			v.lines = append(v.lines, msg.Line)
		}
		v.viewport.SetContentLines(v.lines)
		// Auto-scroll to bottom when not paused
		if !v.paused {
			v.viewport.End()
		}
		return v, nil
	}

	return v, nil
}

func (v *LogView) handleKey(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "p":
		v.paused = !v.paused
		return v, nil
	case "c":
		v.lines = nil
		v.viewport.SetContentLines(nil)
		return v, nil
	case "ctrl+k":
		v.viewport.ScrollUp(1)
	case "ctrl+j":
		v.viewport.ScrollDown(1)
	case "pgup":
		v.viewport.PageUp()
	case "pgdown":
		v.viewport.PageDown()
	case "home":
		v.viewport.Home()
	case "end":
		v.viewport.End()
	}
	return v, nil
}

func (v *LogView) fetchLog() tea.Cmd {
	v.loading = true
	return func() tea.Msg {
		ctx := context.Background()

		rows, err := v.db.Pool.Query(ctx, `
			SELECT pid, usename, state,
			       COALESCE(LEFT(query, 120), ''),
			       COALESCE(EXTRACT(EPOCH FROM (now() - query_start))::int::text, '?')
			FROM pg_stat_activity
			WHERE datname = current_database()
			  AND pid != pg_backend_pid()
			  AND state IS NOT NULL
			ORDER BY query_start DESC NULLS LAST
			LIMIT 50`)
		if err != nil {
			return LogMsg{Err: err}
		}
		defer rows.Close()

		var logLines []string
		timestamp := time.Now().Format("15:04:05")
		logLines = append(logLines, "")
		logLines = append(logLines, StyleDimmed.Render(fmt.Sprintf("── %s ──", timestamp)))

		for rows.Next() {
			var pid int
			var user, state, query, elapsed string
			if err := rows.Scan(&pid, &user, &state, &query, &elapsed); err != nil {
				return LogMsg{Err: err}
			}

			stateColor := ColorFgDim
			if state == "active" {
				stateColor = ColorSuccess
			} else if state == "idle in transaction" {
				stateColor = ColorWarning
			}

			logLines = append(logLines,
				fmt.Sprintf("  [%d] %s %s %ss",
					pid,
					lipgloss.NewStyle().Foreground(stateColor).Render(state),
					user,
					elapsed))
			if query != "" && state == "active" {
				logLines = append(logLines,
					"       "+StyleDimmed.Render(strings.ReplaceAll(query, "\n", " ")))
			}
		}

		// Return as a single log message with all lines
		return LogMsg{Line: strings.Join(logLines, "\n")}
	}
}

func (v *LogView) View() string {
	status := StyleSuccess.Render("● STREAMING")
	if v.paused {
		status = StyleWarning.Render("● PAUSED")
	}

	header := fmt.Sprintf("  %s  %s",
		StyleTitle.Render("📋 Activity Log"),
		status)

	content := v.viewport.Render()

	return lipgloss.JoinVertical(lipgloss.Left, header, content)
}
