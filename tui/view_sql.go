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
}

func NewSQLView(database *db.DB) *SQLView {
	return &SQLView{
		db:       database,
		vars:     db.NewVariables(),
		viewport: NewViewport(80, 20),
		histIdx:  -1,
	}
}

func (v *SQLView) Name() string { return "SQL" }

func (v *SQLView) SetSize(width, height int) {
	v.width = width
	v.height = height
	// Reserve space for input line + header
	v.viewport.SetSize(width-2, height-4)
}

func (v *SQLView) ShortHelp() []KeyBinding {
	return []KeyBinding{
		{Key: "Enter", Desc: "execute"},
		{Key: "↑/↓", Desc: "history"},
		{Key: "w", Desc: "wrap"},
	}
}

func (v *SQLView) Init() tea.Cmd { return nil }

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
		v.loading = false
		v.err = msg.Err
		if msg.Err == nil {
			v.viewport.SetContentLines(v.formatTables(msg.Tables))
		} else {
			v.viewport.SetContent(StyleError.Render("ERROR: " + msg.Err.Error()))
		}
		return v, nil
	}

	return v, nil
}

func (v *SQLView) handleKey(msg tea.KeyMsg) (View, tea.Cmd) {
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
	case "ctrl+h":
		v.viewport.ScrollLeft(4)
		return v, nil
	case "ctrl+l":
		v.viewport.ScrollRight(4)
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
	v.input = ""

	return func() tea.Msg {
		result, err := v.db.Execute(context.Background(), sql)
		return QueryResultMsg{Result: result, Err: err}
	}
}

func (v *SQLView) handleMetaCommand(cmd string) tea.Cmd {
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "\\dt":
		v.loading = true
		v.input = ""
		return func() tea.Msg {
			tables, err := v.db.ListTables(context.Background(), "public")
			return TablesListMsg{Tables: tables, Err: err}
		}
	case "\\di":
		v.loading = true
		v.input = ""
		return func() tea.Msg {
			indexes, err := v.db.ListIndexes(context.Background(), "public")
			return TablesListMsg{Tables: indexes, Err: err}
		}
	case "\\dv":
		v.loading = true
		v.input = ""
		return func() tea.Msg {
			views, err := v.db.ListViews(context.Background(), "public")
			return TablesListMsg{Tables: views, Err: err}
		}
	case "\\d":
		if len(parts) < 2 {
			v.viewport.SetContent(StyleError.Render("Usage: \\d <table_name>"))
			return nil
		}
		v.loading = true
		v.input = ""
		table := parts[1]
		return func() tea.Msg {
			result, err := v.db.DescribeTable(context.Background(), "public", table)
			return QueryResultMsg{Result: result, Err: err}
		}
	case "\\set":
		if len(parts) < 3 {
			// List variables
			lines := v.vars.List()
			if len(lines) == 0 {
				v.viewport.SetContent(StyleDimmed.Render("No variables set."))
			} else {
				v.viewport.SetContentLines(lines)
			}
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

	// Cap column widths
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

func (v *SQLView) formatTables(tables []db.TableInfo) []string {
	if len(tables) == 0 {
		return []string{StyleDimmed.Render("No objects found.")}
	}

	var lines []string
	lines = append(lines, StyleSuccess.Render(fmt.Sprintf(" %-20s │ %-10s │ %s", "Schema", "Type", "Name")))
	lines = append(lines, StyleDimmed.Render(strings.Repeat("─", 60)))
	for _, t := range tables {
		lines = append(lines, fmt.Sprintf(" %-20s │ %-10s │ %s", t.Schema, t.Type, t.Name))
	}
	lines = append(lines, "")
	lines = append(lines, StyleDimmed.Render(fmt.Sprintf("(%d object%s)", len(tables), plural2(len(tables)))))
	return lines
}

func plural2(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (v *SQLView) View() string {
	// Input prompt
	prompt := StylePrompt.Render("paiSQL> ") + v.input + "█"
	if v.loading {
		prompt = StylePrompt.Render("paiSQL> ") + StyleDimmed.Render("executing...")
	}

	// Content
	content := v.viewport.Render()

	return lipgloss.JoinVertical(lipgloss.Left, prompt, "", content)
}
