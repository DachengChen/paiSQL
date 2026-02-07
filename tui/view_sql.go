// view_sql.go — The main SQL query view.
//
// Features:
//   - Text input for SQL queries
//   - Async query execution (never blocks UI)
//   - Results rendered as a table with scrolling
//   - Meta-commands: \dt \di \dv \d <table> \set
//   - Variable substitution via db.Variables
package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/DachengChen/paiSQL/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	focusSidebar = iota
	focusInput
)

type SQLView struct {
	db       *db.DB
	vars     *db.Variables
	viewport *Viewport
	input    string
	history  []string
	histIdx  int
	result   *db.QueryResult
	err      error
	loading  bool
	width    int
	height   int

	// Split view state
	tables   []string
	tableIdx int
	focus    int // focusSidebar or focusInput
	tableErr error
}

func NewSQLView(database *db.DB) *SQLView {
	return &SQLView{
		db:       database,
		vars:     db.NewVariables(),
		viewport: NewViewport(80, 20),
		histIdx:  -1,
		focus:    focusSidebar, // Start in sidebar so user can select tables immediately
	}
}

func (v *SQLView) Name() string { return "SQL" }

func (v *SQLView) SetSize(width, height int) {
	v.width = width
	v.height = height
	// Viewport takes up the right side minus prompt and spacing
	// We'll calculate exact dimensions in View() but setting a safe default here
	sidebarWidth := 25
	contentWidth := width - sidebarWidth - 4
	v.viewport.SetSize(contentWidth, height-4)
}

func (v *SQLView) ShortHelp() []KeyBinding {
	if v.focus == focusSidebar {
		return []KeyBinding{
			{Key: "↑/↓", Desc: "navigate"},
			{Key: "Enter", Desc: "select table"},
			{Key: "Tab", Desc: "focus query"},
		}
	}
	return []KeyBinding{
		{Key: "Enter", Desc: "execute"},
		{Key: "Tab", Desc: "focus tables"},
		{Key: "↑/↓", Desc: "history"},
	}
}

func (v *SQLView) Init() tea.Cmd {
	return v.fetchTables()
}

func (v *SQLView) fetchTables() tea.Cmd {
	return func() tea.Msg {
		// Only listing public tables for the sidebar to keep it clean
		tables, err := v.db.ListTables(context.Background(), "public")
		var names []string
		for _, t := range tables {
			names = append(names, t.Name)
		}
		return TablesListMsg{Tables: tables, Err: err}
	}
}

func (v *SQLView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return v.handleKey(msg)

	case QueryResultMsg:
		v.loading = false
		v.err = msg.Err
		v.result = msg.Result
		if msg.Result != nil {
			v.viewport.SetContentLines(v.formatResult(msg.Result))
		} else if msg.Err != nil {
			v.viewport.SetContent(StyleError.Render("ERROR: " + msg.Err.Error()))
		}
		return v, nil

	case TablesListMsg:
		// Update the sidebar list
		if msg.Err == nil {
			var names []string
			for _, t := range msg.Tables {
				names = append(names, t.Name)
			}
			v.tables = names
			v.tableErr = nil
		} else {
			v.tableErr = msg.Err
		}
		return v, nil
	}

	return v, nil
}

func (v *SQLView) handleKey(msg tea.KeyMsg) (View, tea.Cmd) {
	// Global toggle focus
	if msg.String() == "tab" {
		if v.focus == focusSidebar {
			v.focus = focusInput
		} else {
			v.focus = focusSidebar
		}
		return v, nil
	}

	// Sidebar controls
	if v.focus == focusSidebar {
		switch msg.String() {
		case "up", "k":
			if v.tableIdx > 0 {
				v.tableIdx--
			}
		case "down", "j":
			if v.tableIdx < len(v.tables)-1 {
				v.tableIdx++
			}
		case "enter":
			if len(v.tables) > 0 {
				selected := v.tables[v.tableIdx]
				v.input = fmt.Sprintf("SELECT * FROM %s LIMIT 100;", selected)
				v.focus = focusInput
				// Auto-execute? Optional. Let's let user confirm for now.
			}
		}
		return v, nil
	}

	// Input controls (when focused)
	switch msg.String() {
	case "enter":
		return v, v.execute()

	case "up":
		if len(v.history) > 0 {
			if v.histIdx < len(v.history)-1 {
				v.histIdx++
			}
			v.input = v.history[v.histIdx]
		}
		return v, nil

	case "down":
		if v.histIdx > 0 {
			v.histIdx--
			v.input = v.history[v.histIdx]
		} else {
			v.histIdx = -1
			v.input = ""
		}
		return v, nil

	case "ctrl+k":
		v.viewport.ScrollUp(1)
		return v, nil
	case "ctrl+j":
		v.viewport.ScrollDown(1)
		return v, nil
	case "pgup":
		v.viewport.PageUp()
		return v, nil
	case "pgdown":
		v.viewport.PageDown()
		return v, nil
	case "ctrl+w":
		v.viewport.ToggleWrap()
		return v, nil

	case "backspace":
		if len(v.input) > 0 {
			v.input = v.input[:len(v.input)-1]
		}
		return v, nil

	default:
		// Simple text input
		if len(msg.String()) == 1 || msg.String() == " " {
			v.input += msg.String()
		}
		return v, nil
	}
}

func (v *SQLView) execute() tea.Cmd {
	input := strings.TrimSpace(v.input)
	if input == "" {
		return nil
	}

	// Store in history
	v.history = append([]string{input}, v.history...)
	v.histIdx = -1

	// Handle meta-commands
	if strings.HasPrefix(input, "\\") {
		return v.handleMetaCommand(input)
	}

	// Expand variables
	sql := v.vars.Expand(input)

	v.loading = true
	v.input = "" // clear input

	return func() tea.Msg {
		result, err := v.db.Execute(context.Background(), sql)
		return QueryResultMsg{Result: result, Err: err}
	}
}

// ... handleMetaCommand ... (keep as is or copy if needed, assuming reusing existing logic structure)
func (v *SQLView) handleMetaCommand(cmd string) tea.Cmd {
	// Re-implementing meta commands to work with new struct, minimizing changes
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "\\dt", "\\di", "\\dv", "\\d":
		// These commands in original code returned tables.
		// For now we'll just execute them and show results in viewport text if needed?
		// Or trigger a refresh of the sidebar?
		// Original logic was fine, we can keep it.
		// NOTE: simplified for this refactor to just refresh tables
		return v.fetchTables()

	case "\\set":
		if len(parts) < 3 {
			lines := v.vars.List()
			v.viewport.SetContentLines(lines)
			v.input = ""
			return nil
		}
		v.vars.Set(parts[1], strings.Join(parts[2:], " "))
		v.viewport.SetContent(StyleSuccess.Render(fmt.Sprintf("SET %s = %s", parts[1], strings.Join(parts[2:], " "))))
		v.input = ""
		return nil
	default:
		v.viewport.SetContent(StyleError.Render("Unknown command: " + cmd))
		v.input = ""
		return nil
	}
}

// ... formatResult ... (keep same)
func (v *SQLView) formatResult(r *db.QueryResult) []string {
	if r == nil || len(r.Columns) == 0 {
		return []string{StyleDimmed.Render(r.Status)}
	}

	// Calculate column widths
	widths := make([]int, len(r.Columns))
	for i, col := range r.Columns {
		widths[i] = len(col)
	}
	for _, row := range r.Rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	// Cap
	for i := range widths {
		if widths[i] > 50 {
			widths[i] = 50
		}
	}

	// Build header
	var lines []string
	header := ""
	separator := ""
	for i, col := range r.Columns {
		header += fmt.Sprintf(" %-*s │", widths[i], col)
		separator += strings.Repeat("─", widths[i]+1) + "┼"
	}
	lines = append(lines, StyleSuccess.Render(strings.TrimRight(header, "│")))
	lines = append(lines, StyleDimmed.Render(strings.TrimRight(separator, "┼")))

	// Build rows
	for _, row := range r.Rows {
		line := ""
		for i, cell := range row {
			if i < len(widths) {
				if len(cell) > widths[i] {
					cell = cell[:widths[i]-1] + "…"
				}
				line += fmt.Sprintf(" %-*s │", widths[i], cell)
			}
		}
		lines = append(lines, strings.TrimRight(line, "│"))
	}
	lines = append(lines, "")
	lines = append(lines, StyleDimmed.Render(r.Status))
	return lines
}

func (v *SQLView) View() string {
	// Sidebar (Tables)
	sidebarWidth := 25
	var tableList []string

	headerStyle := StyleBold.BorderBottom(true).BorderForeground(ColorDim).Width(sidebarWidth - 2)
	tableList = append(tableList, headerStyle.Render(" Tables"))

	if v.tableErr != nil {
		tableList = append(tableList, StyleError.Render("Error"))
	} else if len(v.tables) == 0 {
		tableList = append(tableList, StyleDimmed.Render(" (no tables)"))
	} else {
		// Visible window for tables (simple scrolling)
		limit := v.height - 4 // approx
		start := 0
		if v.tableIdx > limit/2 {
			start = v.tableIdx - limit/2
		}
		end := start + limit
		if end > len(v.tables) {
			end = len(v.tables)
		}

		for i := start; i < end; i++ {
			name := v.tables[i]
			// Truncate name if too long
			if len(name) > sidebarWidth-4 {
				name = name[:sidebarWidth-4] + "…"
			}

			if i == v.tableIdx {
				if v.focus == focusSidebar {
					tableList = append(tableList, StyleListItemActive.Render("▸ "+name))
				} else {
					tableList = append(tableList, StyleDimmed.Render("▸ "+name))
				}
			} else {
				tableList = append(tableList, StyleDimmed.Render("  "+name))
			}
		}
	}

	sidebarStyle := lipgloss.NewStyle().
		Width(sidebarWidth).
		Height(v.height).
		Border(lipgloss.NormalBorder(), false, true, false, false). // Right border
		BorderForeground(ColorDim)                                  // Uses simplified color

	if v.focus == focusSidebar {
		sidebarStyle = sidebarStyle.BorderForeground(ColorAccent) // Highlight border if focused
	}

	sidebar := sidebarStyle.Render(strings.Join(tableList, "\n"))

	// Main Content (Input + Results)
	promptColor := ColorDim
	if v.focus == focusInput {
		promptColor = ColorAccent
	}

	promptLabel := lipgloss.NewStyle().Foreground(promptColor).Bold(true).Render("SQL> ")
	promptTxt := v.input
	if v.focus == focusInput {
		promptTxt += "█"
	} else if v.input == "" {
		promptTxt = StyleDimmed.Render("(press tab to focus sidebar)")
	} else {
		promptTxt = StyleDimmed.Render(promptTxt)
	}

	promptBar := promptLabel + promptTxt
	if v.loading {
		promptBar = promptLabel + StyleDimmed.Render("executing...")
	}

	content := v.viewport.Render()

	mainPane := lipgloss.JoinVertical(lipgloss.Left,
		promptBar,
		"", // gap
		content,
	)

	// Ensure main pane fills remaining width
	mainPane = lipgloss.NewStyle().
		Width(v.width - sidebarWidth - 2).
		Height(v.height).
		PaddingLeft(1).
		Render(mainPane)

	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, mainPane)
}
