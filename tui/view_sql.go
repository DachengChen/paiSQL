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
	focusResults
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
	focus    int
	tableErr error
}

func NewSQLView(database *db.DB) *SQLView {
	return &SQLView{
		db:       database,
		vars:     db.NewVariables(),
		viewport: NewViewport(80, 20),
		histIdx:  -1,
		focus:    focusSidebar, // Start in sidebar
	}
}

func (v *SQLView) Name() string { return "SQL" }

func (v *SQLView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

func (v *SQLView) ShortHelp() []KeyBinding {
	if v.focus == focusSidebar {
		return []KeyBinding{
			{Key: "↑/↓", Desc: "navigate"},
			{Key: "Enter", Desc: "select table"},
			{Key: "Tab", Desc: "focus results"},
		}
	} else if v.focus == focusResults {
		return []KeyBinding{
			{Key: "↑/↓/PgUp/PgDn", Desc: "scroll"},
			{Key: "Tab", Desc: "focus input"},
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
		tables, err := v.db.ListTables(context.Background(), "public")
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
		// Auto-focus results on success? Optional. Keeping current focus.
		return v, nil

	case TablesListMsg:
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
	// Navigate between panes
	if msg.String() == "tab" {
		v.focus = (v.focus + 1) % 3
		return v, nil
	}
	// Shift+Tab to go back? Bubble Tea might capture as "shift+tab"
	if msg.String() == "shift+tab" {
		v.focus--
		if v.focus < 0 {
			v.focus = 2
		}
		return v, nil
	}

	switch v.focus {
	case focusSidebar:
		return v.handleSidebarKey(msg)
	case focusResults:
		return v.handleResultsKey(msg)
	case focusInput:
		return v.handleInputKey(msg)
	}
	return v, nil
}

func (v *SQLView) handleSidebarKey(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if v.tableIdx > 0 {
			v.tableIdx--
		}
	case "down", "j":
		if v.tableIdx < len(v.tables)-1 {
			v.tableIdx++
		}
	case "pgup":
		pageSize := v.height - 4
		if pageSize < 1 {
			pageSize = 1
		}
		v.tableIdx -= pageSize
		if v.tableIdx < 0 {
			v.tableIdx = 0
		}
	case "pgdown":
		pageSize := v.height - 4
		if pageSize < 1 {
			pageSize = 1
		}
		v.tableIdx += pageSize
		if v.tableIdx >= len(v.tables) {
			v.tableIdx = len(v.tables) - 1
		}
	case "home":
		v.tableIdx = 0
	case "end":
		if len(v.tables) > 0 {
			v.tableIdx = len(v.tables) - 1
		}
	case "enter":
		if len(v.tables) > 0 {
			selected := v.tables[v.tableIdx]
			v.input = fmt.Sprintf("SELECT * FROM %s LIMIT 20;", selected)
			// Execute immediately
			cmd := v.execute()
			// v.focus = focusResults // Keep focus in sidebar
			return v, cmd
		}
	}
	return v, nil
}

func (v *SQLView) handleResultsKey(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		v.viewport.ScrollUp(1)
	case "down", "j":
		v.viewport.ScrollDown(1)
	case "pgup":
		v.viewport.PageUp()
	case "pgdown":
		v.viewport.PageDown()
	case "home":
		v.viewport.Home()
	case "end":
		v.viewport.End()
	case "ctrl+h":
		v.viewport.ScrollLeft(4)
	case "ctrl+l":
		v.viewport.ScrollRight(4)
	case "w": // wrap toggle
		v.viewport.ToggleWrap()
	}
	return v, nil
}

func (v *SQLView) handleInputKey(msg tea.KeyMsg) (View, tea.Cmd) {
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
	case "down":
		if v.histIdx > 0 {
			v.histIdx--
			v.input = v.history[v.histIdx]
		} else {
			v.histIdx = -1
			v.input = ""
		}
	case "backspace":
		if len(v.input) > 0 {
			v.input = v.input[:len(v.input)-1]
		}
	default:
		if len(msg.String()) == 1 || msg.String() == " " {
			v.input += msg.String()
		}
	}
	return v, nil
}

func (v *SQLView) execute() tea.Cmd {
	input := strings.TrimSpace(v.input)
	if input == "" {
		return nil
	}
	v.history = append([]string{input}, v.history...)
	v.histIdx = -1
	if strings.HasPrefix(input, "\\") {
		return v.handleMetaCommand(input)
	}
	sql := v.vars.Expand(input)
	v.loading = true
	v.input = ""
	return func() tea.Msg {
		result, err := v.db.Execute(context.Background(), sql)
		return QueryResultMsg{Result: result, Err: err}
	}
}

func (v *SQLView) handleMetaCommand(cmd string) tea.Cmd {
	// Simple meta commands
	parts := strings.Fields(cmd)
	switch parts[0] {
	case "\\dt", "\\di", "\\dv", "\\d":
		return v.fetchTables()
	case "\\set":
		if len(parts) >= 3 {
			v.vars.Set(parts[1], strings.Join(parts[2:], " "))
			v.viewport.SetContent(StyleSuccess.Render(fmt.Sprintf("SET %s = ...", parts[1])))
		} else {
			v.viewport.SetContentLines(v.vars.List())
		}
		v.input = ""
		return nil
	}
	v.viewport.SetContent(StyleError.Render("Unknown command: " + cmd))
	v.input = ""
	return nil
}

func (v *SQLView) formatResult(r *db.QueryResult) []string {
	if r == nil || len(r.Columns) == 0 {
		return []string{StyleDimmed.Render(r.Status)}
	}
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
	for i := range widths {
		if widths[i] > 50 {
			widths[i] = 50
		}
	}
	var lines []string
	header := ""
	separator := ""
	for i, col := range r.Columns {
		header += fmt.Sprintf(" %-*s │", widths[i], col)
		separator += strings.Repeat("─", widths[i]+1) + "┼"
	}
	lines = append(lines, StyleSuccess.Render(strings.TrimRight(header, "│")))
	lines = append(lines, StyleDimmed.Render(strings.TrimRight(separator, "┼")))
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
	lines = append(lines, "", StyleDimmed.Render(r.Status))
	return lines
}

func (v *SQLView) View() string {
	// Dimensions
	sidebarWidth := 25
	inputHeight := 5 // fixed height for input block

	// Calculate available width for the right pane (results + input)
	// v.width - sidebarWidth - 2 (for sidebar's right border + 1 padding)
	contentWidth := v.width - sidebarWidth - 1

	// Calculate height for results block
	// v.height - inputHeight - 1 (for input block's top border)
	resultsHeight := v.height - inputHeight - 1

	// 1. Sidebar
	var tableList []string
	headerStyle := StyleBold.BorderBottom(true).BorderForeground(ColorDim).Width(sidebarWidth - 2)
	tableList = append(tableList, headerStyle.Render(" Tables"))

	if v.tableErr != nil {
		tableList = append(tableList, StyleError.Render("Error: "+v.tableErr.Error()))
	} else if len(v.tables) > 0 {
		limit := v.height - 4
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
	} else {
		tableList = append(tableList, StyleDimmed.Render(" (no tables)"))
	}

	sidebarBorderColor := ColorDim
	if v.focus == focusSidebar {
		sidebarBorderColor = ColorAccent
	}
	sidebar := lipgloss.NewStyle().
		Width(sidebarWidth).
		Height(v.height).
		Border(lipgloss.NormalBorder(), false, true, false, false).
		BorderForeground(sidebarBorderColor).
		Render(strings.Join(tableList, "\n"))

	// 2. Results Block (Top Right)
	// Viewport content width is contentWidth - 2 for left/right padding/border
	// Viewport content height is resultsHeight - 2 for top/bottom padding/border
	v.viewport.SetSize(contentWidth-2, resultsHeight-2)
	resultsBorderColor := ColorDim
	if v.focus == focusResults {
		resultsBorderColor = ColorAccent
	}

	resultBlock := lipgloss.NewStyle().
		Width(contentWidth).
		Height(resultsHeight).
		Border(lipgloss.NormalBorder(), false, false, true, false). // Bottom border
		BorderForeground(resultsBorderColor).
		Render(v.viewport.Render())

	// 3. Input Block (Bottom Right)
	inputBorderColor := ColorDim
	if v.focus == focusInput {
		inputBorderColor = ColorAccent
	}
	promptTxt := v.input
	if v.focus == focusInput {
		promptTxt += "█"
	} else if v.input == "" {
		promptTxt = StyleDimmed.Render("(press tab to focus input)")
	} else {
		promptTxt = StyleDimmed.Render(promptTxt)
	}

	if v.loading {
		promptTxt = StyleDimmed.Render("Executing...")
	}

	inputBlock := lipgloss.NewStyle().
		Width(contentWidth).
		Height(inputHeight).
		Padding(0, 1). // Padding inside the input block
		BorderForeground(inputBorderColor).
		Render(StylePrompt.Render("SQL> ") + promptTxt)

	// Combine Right Side
	rightPane := lipgloss.JoinVertical(lipgloss.Left, resultBlock, inputBlock)

	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPane)
}
