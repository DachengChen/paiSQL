// view_stats.go — Database statistics view.
//
// Shows live stats: database size, active connections, table sizes,
// cache hit ratio, etc. Data is fetched asynchronously.
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/DachengChen/paiSQL/db"
	tea "github.com/charmbracelet/bubbletea"
)

type StatsView struct {
	db       *db.DB
	viewport *Viewport
	loading  bool
	err      error
	width    int
	height   int
}

func NewStatsView(database *db.DB) *StatsView {
	return &StatsView{
		db:       database,
		viewport: NewViewport(80, 20),
	}
}

func (v *StatsView) Name() string { return "Stats" }

func (v *StatsView) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.viewport.SetSize(width-2, height-2)
}

func (v *StatsView) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "r", Desc: "refresh"},
		{Key: "↑/↓", Desc: "scroll"},
	}
}

func (v *StatsView) Init() tea.Cmd {
	return v.fetchStats()
}

func (v *StatsView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return v.handleKey(msg)
	case StatsMsg:
		v.loading = false
		v.err = msg.Err
		if msg.Err != nil {
			v.viewport.SetContent(StyleError.Render("ERROR: " + msg.Err.Error()))
		} else {
			v.viewport.SetContentLines(msg.Lines)
		}
		return v, nil
	}
	return v, nil
}

func (v *StatsView) handleKey(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "r":
		return v, v.fetchStats()
	case "ctrl+k":
		v.viewport.ScrollUp(1)
	case "ctrl+j":
		v.viewport.ScrollDown(1)
	case "pgup":
		v.viewport.PageUp()
	case "pgdown":
		v.viewport.PageDown()
	}
	return v, nil
}

func (v *StatsView) fetchStats() tea.Cmd {
	v.loading = true
	return func() tea.Msg {
		ctx := context.Background()
		var lines []string

		lines = append(lines, StyleTitle.Render("📊 Database Statistics"))
		lines = append(lines, "")

		// Database size
		var dbSize string
		err := v.db.Pool.QueryRow(ctx,
			"SELECT pg_size_pretty(pg_database_size(current_database()))").Scan(&dbSize)
		if err != nil {
			return StatsMsg{Err: err}
		}
		lines = append(lines, fmt.Sprintf("  Database size:        %s", dbSize))

		// Current database
		var dbName string
		if err := v.db.Pool.QueryRow(ctx, "SELECT current_database()").Scan(&dbName); err != nil {
			return StatsMsg{Err: err}
		}
		lines = append(lines, fmt.Sprintf("  Database:             %s", dbName))

		// Active connections
		var connCount int
		if err := v.db.Pool.QueryRow(ctx,
			"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()").Scan(&connCount); err != nil {
			return StatsMsg{Err: err}
		}
		lines = append(lines, fmt.Sprintf("  Active connections:   %d", connCount))

		// PostgreSQL version
		var version string
		if err := v.db.Pool.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
			return StatsMsg{Err: err}
		}
		// Truncate long version strings
		if idx := strings.Index(version, ","); idx > 0 {
			version = version[:idx]
		}
		lines = append(lines, fmt.Sprintf("  Server version:       %s", version))

		lines = append(lines, "")

		// Cache hit ratio
		var hitRatio *float64
		v.db.Pool.QueryRow(ctx, `
			SELECT ROUND(
				sum(heap_blks_hit) / NULLIF(sum(heap_blks_hit) + sum(heap_blks_read), 0) * 100, 2
			) FROM pg_statio_user_tables`).Scan(&hitRatio)
		if hitRatio != nil {
			lines = append(lines, fmt.Sprintf("  Cache hit ratio:      %.2f%%", *hitRatio))
		} else {
			lines = append(lines, "  Cache hit ratio:      N/A")
		}

		lines = append(lines, "")
		lines = append(lines, StyleTitle.Render("📋 Table Sizes (Top 20)"))
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("  %-40s │ %-12s │ %s", "Table", "Size", "Rows (est.)"))
		lines = append(lines, "  "+strings.Repeat("─", 70))

		// Table sizes
		rows, err := v.db.Pool.Query(ctx, `
			SELECT schemaname || '.' || relname,
			       pg_size_pretty(pg_total_relation_size(relid)),
			       n_live_tup
			FROM pg_stat_user_tables
			ORDER BY pg_total_relation_size(relid) DESC
			LIMIT 20`)
		if err != nil {
			return StatsMsg{Err: err}
		}
		defer rows.Close()

		for rows.Next() {
			var name, size string
			var rowCount int64
			if err := rows.Scan(&name, &size, &rowCount); err != nil {
				return StatsMsg{Err: err}
			}
			lines = append(lines, fmt.Sprintf("  %-40s │ %-12s │ %d", name, size, rowCount))
		}

		lines = append(lines, "")
		lines = append(lines, StyleDimmed.Render("  Press 'r' to refresh"))

		return StatsMsg{Lines: lines}
	}
}

func (v *StatsView) View() string {
	if v.loading {
		return StyleDimmed.Render("  Loading statistics...")
	}
	return v.viewport.Render()
}
