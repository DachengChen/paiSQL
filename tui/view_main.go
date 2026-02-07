// view_main.go — The main view.
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
	"unicode/utf8"

	"github.com/DachengChen/paiSQL/ai"
	"github.com/DachengChen/paiSQL/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	focusSidebar = iota
	focusResults
	focusInput
)

// inputMode determines whether the input block is SQL or Chat.
const (
	inputModeChat = iota
	inputModeSQL
)

type MainView struct {
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
	tables    []string
	tableRows []int64 // estimated row counts per table
	tableIdx  int
	focus     int
	tableErr  error

	// Pagination state
	pagTable    string // current paginated table name
	pagPage     int    // current page (0-based)
	pagPageSize int    // rows per page
	pagTotal    int64  // total rows in table

	// Chat mode state
	inputMode    int // inputModeChat or inputModeSQL
	aiProvider   ai.Provider
	chatInput    string
	chatMessages []ai.Message
	chatLoading  bool
}

func NewMainView(database *db.DB, provider ai.Provider) *MainView {
	return &MainView{
		db:         database,
		vars:       db.NewVariables(),
		viewport:   NewViewport(80, 20),
		histIdx:    -1,
		focus:      focusSidebar,
		aiProvider: provider,
	}
}

func (v *MainView) Name() string { return "Main" }

func (v *MainView) SetSize(width, height int) {
	v.width = width
	v.height = height
}

func (v *MainView) ShortHelp() []KeyBinding {
	modeLabel := "chat"
	if v.inputMode == inputModeChat {
		modeLabel = "sql"
	}
	toggle := KeyBinding{Key: "F2", Desc: modeLabel}

	if v.focus == focusSidebar {
		return []KeyBinding{
			toggle,
			{Key: "↑/↓", Desc: "navigate"},
			{Key: "Enter", Desc: "select table"},
			{Key: "Tab", Desc: "focus results"},
		}
	} else if v.focus == focusResults {
		return []KeyBinding{
			toggle,
			{Key: "↑/↓", Desc: "scroll"},
			{Key: "←/→", Desc: "pan"},
			{Key: "Tab", Desc: "focus input"},
		}
	}

	if v.inputMode == inputModeChat {
		return []KeyBinding{
			toggle,
			{Key: "Enter", Desc: "send"},
			{Key: "Ctrl+L", Desc: "clear chat"},
		}
	}
	return []KeyBinding{
		toggle,
		{Key: "Enter", Desc: "execute"},
		{Key: "Tab", Desc: "focus tables"},
		{Key: "↑/↓", Desc: "history"},
	}
}

func (v *MainView) Init() tea.Cmd {
	return v.fetchTables()
}

func (v *MainView) fetchTables() tea.Cmd {
	return func() tea.Msg {
		tables, err := v.db.ListTables(context.Background(), "public")
		return TablesListMsg{Tables: tables, Err: err}
	}
}

func (v *MainView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return v.handleKey(msg)

	case QueryResultMsg:
		v.loading = false
		v.err = msg.Err
		v.result = msg.Result
		if msg.PagTotal > 0 {
			v.pagTotal = msg.PagTotal
		}
		if msg.Result != nil {
			lines := v.formatResult(msg.Result)
			if msg.PagInfo != "" {
				lines = append([]string{msg.PagInfo, ""}, lines...)
			}
			v.viewport.SetContentLines(lines)
		} else if msg.Err != nil {
			v.viewport.SetContent("ERROR: " + msg.Err.Error())
		}
		return v, nil

	case TablesListMsg:
		if msg.Err == nil {
			var names []string
			var rowCounts []int64
			for _, t := range msg.Tables {
				names = append(names, t.Name)
				rowCounts = append(rowCounts, t.RowCount)
			}
			v.tables = names
			v.tableRows = rowCounts
			v.tableErr = nil
		} else {
			v.tableErr = msg.Err
		}
		return v, nil

	case AIResponseMsg:
		v.chatLoading = false
		if msg.Err != nil {
			v.chatMessages = append(v.chatMessages, ai.Message{
				Role:    "assistant",
				Content: "Error: " + msg.Err.Error(),
			})
		} else {
			v.chatMessages = append(v.chatMessages, ai.Message{
				Role:    "assistant",
				Content: msg.Response,
			})
		}
		v.viewport.SetContentLines(v.renderChatHistory())
		v.viewport.End()
		return v, nil
	}

	return v, nil
}

func (v *MainView) handleKey(msg tea.KeyMsg) (View, tea.Cmd) {
	// F2 toggles between SQL and Chat input mode
	if msg.String() == "f2" {
		if v.inputMode == inputModeSQL {
			v.inputMode = inputModeChat
		} else {
			v.inputMode = inputModeSQL
		}
		v.focus = focusInput
		return v, nil
	}

	// Navigate between panes
	if msg.String() == "tab" {
		v.focus = (v.focus + 1) % 3
		return v, nil
	}
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
		if v.inputMode == inputModeChat {
			return v.handleChatInputKey(msg)
		}
		return v.handleInputKey(msg)
	}
	return v, nil
}

func (v *MainView) handleSidebarKey(msg tea.KeyMsg) (View, tea.Cmd) {
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
			v.pagTable = selected
			v.pagPage = 0
			v.pagPageSize = 20
			// Use estimated row count from sidebar
			if v.tableIdx < len(v.tableRows) {
				v.pagTotal = v.tableRows[v.tableIdx]
			} else {
				v.pagTotal = 0
			}
			return v, v.fetchPage()
		}
	}
	return v, nil
}

func (v *MainView) handleResultsKey(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		v.viewport.ScrollUp(1)
	case "down", "j":
		v.viewport.ScrollDown(1)
	case "left", "h":
		v.viewport.ScrollLeft(4)
	case "right", "l":
		v.viewport.ScrollRight(4)
	case "pgup":
		if v.pagTable != "" {
			// Database-level pagination: previous page
			if v.pagPage > 0 {
				v.pagPage--
				return v, v.fetchPage()
			}
		} else {
			v.viewport.PageUp()
		}
	case "pgdown":
		if v.pagTable != "" {
			// Database-level pagination: next page
			maxPage := v.maxPage()
			if v.pagPage < maxPage {
				v.pagPage++
				return v, v.fetchPage()
			}
		} else {
			v.viewport.PageDown()
		}
	case "home":
		v.viewport.Home()
	case "end":
		v.viewport.End()
	case "ctrl+h":
		v.viewport.ScrollLeft(20)
	case "ctrl+l":
		v.viewport.ScrollRight(20)
	case "w": // wrap toggle
		v.viewport.ToggleWrap()
	}
	return v, nil
}

func (v *MainView) handleInputKey(msg tea.KeyMsg) (View, tea.Cmd) {
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

func (v *MainView) execute() tea.Cmd {
	input := strings.TrimSpace(v.input)
	if input == "" {
		return nil
	}
	v.history = append([]string{input}, v.history...)
	v.histIdx = -1
	v.pagTable = "" // clear pagination for manual queries
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

// fetchPage runs a paginated SELECT for the current table.
func (v *MainView) fetchPage() tea.Cmd {
	table := v.pagTable
	page := v.pagPage
	pageSize := v.pagPageSize
	v.loading = true
	return func() tea.Msg {
		ctx := context.Background()

		// Get real row count
		var total int64
		countSQL := fmt.Sprintf("SELECT count(*) FROM %s", table)
		_ = v.db.Pool.QueryRow(ctx, countSQL).Scan(&total)

		// Get table size info
		var totalSize, tableSize, indexSize string
		sizeSQL := `SELECT pg_size_pretty(pg_total_relation_size($1)),
		                   pg_size_pretty(pg_relation_size($1)),
		                   pg_size_pretty(pg_indexes_size($1))`
		_ = v.db.Pool.QueryRow(ctx, sizeSQL, table).Scan(&totalSize, &tableSize, &indexSize)

		info := fmt.Sprintf("📊 %s  |  Total: %s  |  Table: %s  |  Indexes: %s  |  %d rows",
			table, totalSize, tableSize, indexSize, total)

		offset := page * pageSize
		sql := fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", table, pageSize, offset)
		result, err := v.db.Execute(ctx, sql)
		if result != nil {
			lastRow := offset + result.RowCount
			totalPages := maxPageCalc(total, int64(pageSize)) + 1
			result.Status = fmt.Sprintf("Page %d/%d  |  Rows %d–%d of %d",
				page+1, totalPages,
				offset+1, lastRow,
				total)
		}
		return QueryResultMsg{Result: result, Err: err, PagTotal: total, PagInfo: info}
	}
}

func (v *MainView) maxPage() int {
	if v.pagTotal <= 0 || v.pagPageSize <= 0 {
		return 0
	}
	return int((v.pagTotal - 1) / int64(v.pagPageSize))
}

func maxPageCalc(total int64, pageSize int64) int64 {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return (total - 1) / pageSize
}

func (v *MainView) handleMetaCommand(cmd string) tea.Cmd {
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

// ══════════════════════════════════════════
// Chat input mode handlers
// ══════════════════════════════════════════

func (v *MainView) handleChatInputKey(msg tea.KeyMsg) (View, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return v, v.sendChatMessage()
	case "ctrl+l":
		v.chatMessages = nil
		v.chatInput = ""
		v.viewport.SetContentLines(v.renderChatHistory())
		return v, nil
	case "backspace":
		if len(v.chatInput) > 0 {
			v.chatInput = v.chatInput[:len(v.chatInput)-1]
		}
	default:
		if len(msg.String()) == 1 || msg.String() == " " {
			v.chatInput += msg.String()
		}
	}
	return v, nil
}

func (v *MainView) sendChatMessage() tea.Cmd {
	text := strings.TrimSpace(v.chatInput)
	if text == "" {
		return nil
	}

	v.chatMessages = append(v.chatMessages, ai.Message{
		Role:    "user",
		Content: text,
	})
	v.chatInput = ""
	v.chatLoading = true
	v.viewport.SetContentLines(v.renderChatHistory())
	v.viewport.End()

	msgs := make([]ai.Message, len(v.chatMessages))
	copy(msgs, v.chatMessages)

	return func() tea.Msg {
		resp, err := v.aiProvider.Chat(context.Background(), msgs)
		return AIResponseMsg{Response: resp, Err: err}
	}
}

func (v *MainView) renderChatHistory() []string {
	var lines []string

	lines = append(lines, StyleTitle.Render("🤖 AI Chat")+" "+
		StyleDimmed.Render("("+v.aiProvider.Name()+")"))
	lines = append(lines, "")

	if len(v.chatMessages) == 0 {
		lines = append(lines, StyleDimmed.Render("Ask anything about your database..."))
		lines = append(lines, StyleDimmed.Render("Press F2 to switch back to SQL."))
		return lines
	}

	userStyle := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true)
	assistantStyle := lipgloss.NewStyle().
		Foreground(ColorSuccess)

	for _, msg := range v.chatMessages {
		switch msg.Role {
		case "user":
			lines = append(lines, userStyle.Render("You: ")+msg.Content)
			lines = append(lines, "")
		case "assistant":
			lines = append(lines, assistantStyle.Render("AI: "))
			for _, line := range strings.Split(msg.Content, "\n") {
				lines = append(lines, "  "+line)
			}
			lines = append(lines, "")
		}
	}

	if v.chatLoading {
		lines = append(lines, StyleDimmed.Render("  ⏳ Thinking..."))
	}

	return lines
}

func (v *MainView) formatResult(r *db.QueryResult) []string {
	if r == nil || len(r.Columns) == 0 {
		return []string{StyleDimmed.Render(r.Status)}
	}

	runeLen := utf8.RuneCountInString

	widths := make([]int, len(r.Columns))
	for i, col := range r.Columns {
		widths[i] = runeLen(col)
	}
	for _, row := range r.Rows {
		for i, cell := range row {
			if i < len(widths) && runeLen(cell) > widths[i] {
				widths[i] = runeLen(cell)
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
	for i, col := range r.Columns {
		header += fmt.Sprintf(" %-*s │", widths[i], col)
	}
	// Build separator from header: replace every char with ─, except │ → ┼
	var sepBuilder strings.Builder
	for _, ch := range header {
		if ch == '│' {
			sepBuilder.WriteRune('┼')
		} else {
			sepBuilder.WriteRune('─')
		}
	}
	separator := sepBuilder.String()
	lines = append(lines, strings.TrimRight(header, "│"))
	lines = append(lines, strings.TrimRight(separator, "┼"))
	for _, row := range r.Rows {
		line := ""
		for i, cell := range row {
			if i < len(widths) {
				if runeLen(cell) > widths[i] {
					runes := []rune(cell)
					cell = string(runes[:widths[i]-1]) + "…"
				}
				line += fmt.Sprintf(" %-*s │", widths[i], cell)
			}
		}
		lines = append(lines, strings.TrimRight(line, "│"))
	}
	lines = append(lines, "", r.Status)
	return lines
}

func (v *MainView) View() string {
	// Dimensions
	sidebarWidth := v.width / 5 // 20% of full width
	if sidebarWidth < 20 {
		sidebarWidth = 20
	}
	inputHeight := 5

	contentWidth := v.width - sidebarWidth - 1
	resultsHeight := v.height - inputHeight - 1

	// 1. Sidebar (same for both modes)
	var tableList []string
	headerStyle := StyleBold.BorderBottom(true).BorderForeground(ColorDim).Width(sidebarWidth - 2)
	sidebarTitle := "   Tables"
	if v.focus == focusSidebar {
		sidebarTitle = lipgloss.NewStyle().Foreground(ColorAccent).Render(" ●") + " Tables"
	}
	tableList = append(tableList, headerStyle.Render(sidebarTitle))

	if v.tableErr != nil {
		tableList = append(tableList, StyleError.Render("Error: "+v.tableErr.Error()))
	} else if len(v.tables) > 0 {
		limit := v.height
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
			// Append row count suffix
			suffix := ""
			if i < len(v.tableRows) {
				suffix = " (" + db.FormatRowCount(v.tableRows[i]) + ")"
			}
			display := name + suffix
			if len(display) > sidebarWidth-4 {
				// Truncate name but keep the suffix
				maxName := sidebarWidth - 4 - len(suffix) - 1
				if maxName > 0 {
					display = name[:maxName] + "…" + suffix
				} else {
					display = display[:sidebarWidth-4] + "…"
				}
			}
			if i == v.tableIdx {
				if v.focus == focusSidebar {
					tableList = append(tableList, StyleListItemActive.Render("▸ "+display))
				} else {
					tableList = append(tableList, StyleDimmed.Render("▸ "+display))
				}
			} else {
				tableList = append(tableList, StyleDimmed.Render("  "+display))
			}
		}
	} else {
		tableList = append(tableList, StyleDimmed.Render(" (no tables)"))
	}

	// Pad table list to fill sidebar height so the border extends to the bottom
	for len(tableList) < v.height+1 {
		tableList = append(tableList, "")
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

	// 2. Results Block (Top Right) — single viewport for both SQL and Chat
	v.viewport.SetSize(contentWidth-2, resultsHeight-2)
	resultsBorderColor := ColorDim
	resultsFocus := "  "
	if v.focus == focusResults {
		resultsBorderColor = ColorAccent
		resultsFocus = lipgloss.NewStyle().Foreground(ColorAccent).Render(" ●")
	}

	resultBlock := lipgloss.NewStyle().
		Width(contentWidth).
		Height(resultsHeight).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(resultsBorderColor).
		Render(resultsFocus + v.viewport.Render())

	// 3. Input Block (Bottom Right)
	inputFocus := "  "
	if v.focus == focusInput {
		inputFocus = lipgloss.NewStyle().Foreground(ColorAccent).Render("● ")
	}

	var promptLabel, promptTxt string
	if v.inputMode == inputModeChat {
		promptLabel = StylePrompt.Render("Ask> ")
		promptTxt = v.chatInput
		if v.focus == focusInput {
			promptTxt += "█"
		} else if v.chatInput == "" {
			promptTxt = StyleDimmed.Render("(press tab to focus input)")
		} else {
			promptTxt = StyleDimmed.Render(promptTxt)
		}
		if v.chatLoading {
			promptTxt = StyleDimmed.Render("waiting for response...")
		}
	} else {
		promptLabel = StylePrompt.Render("SQL> ")
		promptTxt = v.input
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
	}

	inputBlock := lipgloss.NewStyle().
		Width(contentWidth).
		Height(inputHeight).
		Padding(0, 1).
		Render(inputFocus + promptLabel + promptTxt)

	// Combine Right Side
	rightPane := lipgloss.JoinVertical(lipgloss.Left, resultBlock, inputBlock)

	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rightPane)
}
