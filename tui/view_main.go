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

// rightMode determines what the right results pane shows.
const (
	rightModeData = iota
	rightModeDescribe
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

	// Right pane mode
	rightMode    int  // rightModeData or rightModeDescribe
	expandedMode bool // vertical display like \x in psql

	// Chat mode state
	inputMode    int // inputModeChat or inputModeSQL
	aiProvider   ai.Provider
	chatInput    string
	chatMessages []ai.Message
	chatLoading  bool

	// Query plan state — tracks the last AI-generated plan for pagination
	lastQueryPlan *ai.QueryPlan
	planSortCol   string // saved sort column from last plan
	planSortOrder string // saved sort order from last plan

	// Fullscreen toggle (F5) — hides sidebar for clean text selection
	fullscreen bool
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

func (v *MainView) WantsTextInput() bool {
	return v.inputMode == inputModeChat || v.focus == focusInput
}

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
	fs := KeyBinding{Key: "F5", Desc: "fullscreen"}

	if v.focus == focusSidebar {
		return []KeyBinding{
			toggle,
			fs,
			{Key: "↑/↓", Desc: "navigate"},
			{Key: "Enter", Desc: "data"},
			{Key: "d", Desc: "describe"},
			{Key: "Tab", Desc: "focus results"},
		}
	} else if v.focus == focusResults {
		return []KeyBinding{
			toggle,
			fs,
			{Key: "↑/↓", Desc: "scroll"},
			{Key: "←/→", Desc: "pan"},
			{Key: "[/]", Desc: "record"},
			{Key: "x", Desc: "expand"},
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
			v.rightMode = rightModeData
		} else if msg.Err != nil {
			v.viewport.SetContent("ERROR: " + msg.Err.Error())
		}
		return v, nil

	case DescribeResultMsg:
		v.loading = false
		if msg.Err != nil {
			v.viewport.SetContent("ERROR: " + msg.Err.Error())
		} else if msg.Result != nil {
			lines := []string{msg.Header, ""}
			// Columns
			lines = append(lines, "── Columns ──")
			lines = append(lines, v.formatResult(msg.Result)...)
			// Indexes
			if msg.Indexes != nil && msg.Indexes.RowCount > 0 {
				lines = append(lines, "", "── Indexes ──")
				lines = append(lines, v.formatResult(msg.Indexes)...)
			}
			// Foreign Keys
			if msg.ForeignKeys != nil && msg.ForeignKeys.RowCount > 0 {
				lines = append(lines, "", "── Foreign Keys ──")
				lines = append(lines, v.formatResult(msg.ForeignKeys)...)
			}
			// Referenced By
			if msg.ReferencedBy != nil && msg.ReferencedBy.RowCount > 0 {
				lines = append(lines, "", "── Referenced By ──")
				lines = append(lines, v.formatResult(msg.ReferencedBy)...)
			}
			v.viewport.SetContentLines(lines)
			v.rightMode = rightModeDescribe
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

	case QueryPlanMsg:
		v.chatLoading = false
		if msg.Err != nil {
			v.chatMessages = append(v.chatMessages, ai.Message{
				Role:    "assistant",
				Content: "❌ Query plan error: " + msg.Err.Error(),
			})
			v.viewport.SetContentLines(v.renderChatHistory())
			v.viewport.End()
			return v, nil
		}

		plan := msg.Plan

		// Handle "need_other_tables" response
		if plan.NeedOtherTables {
			v.chatMessages = append(v.chatMessages, ai.Message{
				Role:    "assistant",
				Content: "❌ Cannot satisfy this query with the current table and its related tables. Please select a different table or rephrase your question.",
			})
			v.viewport.SetContentLines(v.renderChatHistory())
			v.viewport.End()
			return v, nil
		}

		// Save the plan for pagination
		v.lastQueryPlan = plan
		if plan.Sort != nil {
			v.planSortCol = plan.Sort.Column
			v.planSortOrder = plan.Sort.Order
		}

		// Sync pagination state so PgUp/PgDn works
		if len(plan.Tables) > 0 {
			v.pagTable = plan.Tables[0]
		}
		v.pagPage = plan.Page - 1 // pagPage is 0-based
		v.pagPageSize = plan.Limit

		if plan.IsReadOnly() {
			// SELECT: auto-execute with rich info (same as table browse)
			v.chatMessages = append(v.chatMessages, ai.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("📋 %s\n\n```sql\n%s\n```\n\n🔄 Executing...", plan.Summary(), msg.SQL),
			})
			v.viewport.SetContentLines(v.renderChatHistory())
			v.viewport.End()

			v.loading = true
			return v, v.fetchQueryPlanPage(plan, msg.SQL)
		}

		// Modification: show SQL for review, don't auto-execute
		var reviewLines []string
		reviewLines = append(reviewLines, "⚠️  Modification Query — Review before executing")
		reviewLines = append(reviewLines, "")
		reviewLines = append(reviewLines, "📝 "+plan.Summary())
		reviewLines = append(reviewLines, "")
		reviewLines = append(reviewLines, "Generated SQL:")
		reviewLines = append(reviewLines, "```")
		reviewLines = append(reviewLines, msg.SQL)
		reviewLines = append(reviewLines, "```")
		reviewLines = append(reviewLines, "")
		reviewLines = append(reviewLines, "Copy the SQL above and execute manually with SQL> prompt (F2 to switch).")

		v.chatMessages = append(v.chatMessages, ai.Message{
			Role:    "assistant",
			Content: strings.Join(reviewLines, "\n"),
		})
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

	// F5 toggles fullscreen for the currently focused panel
	if msg.String() == "f5" {
		v.fullscreen = !v.fullscreen
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
			if v.tableIdx < len(v.tableRows) {
				v.pagTotal = v.tableRows[v.tableIdx]
			} else {
				v.pagTotal = 0
			}
			return v, v.fetchPage()
		}
	case "d":
		if len(v.tables) > 0 {
			selected := v.tables[v.tableIdx]
			v.pagTable = ""
			return v, v.fetchDescribe(selected)
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
	case "[":
		// In expanded mode: jump to previous record
		if v.expandedMode && v.result != nil {
			recordHeight := len(v.result.Columns) + 1 // columns + header
			v.viewport.ScrollUp(recordHeight)
		} else {
			v.viewport.ScrollUp(5)
		}
	case "]":
		// In expanded mode: jump to next record
		if v.expandedMode && v.result != nil {
			recordHeight := len(v.result.Columns) + 1
			v.viewport.ScrollDown(recordHeight)
		} else {
			v.viewport.ScrollDown(5)
		}
	case "pgup":
		if v.pagTable != "" {
			if v.pagPage > 0 {
				v.pagPage--
				return v, v.fetchPage()
			}
		} else {
			v.viewport.PageUp()
		}
	case "pgdown":
		if v.pagTable != "" {
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
	case "x": // expanded/vertical display toggle
		v.expandedMode = !v.expandedMode
		if v.result != nil {
			var lines []string
			if v.expandedMode {
				lines = v.formatResultExpanded(v.result)
			} else {
				lines = v.formatResult(v.result)
			}
			if v.pagTable != "" {
				lines = append([]string{v.result.Status, ""}, lines...)
			}
			v.viewport.SetContentLines(lines)
		}
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

// fetchDescribe queries the table schema and returns a DescribeResultMsg.
func (v *MainView) fetchDescribe(table string) tea.Cmd {
	v.loading = true
	return func() tea.Msg {
		ctx := context.Background()

		// Get table size info
		var totalSize, tableSize, indexSize string
		var rowCount int64
		sizeSQL := `SELECT pg_size_pretty(pg_total_relation_size($1)),
		                   pg_size_pretty(pg_relation_size($1)),
		                   pg_size_pretty(pg_indexes_size($1))`
		_ = v.db.Pool.QueryRow(ctx, sizeSQL, table).Scan(&totalSize, &tableSize, &indexSize)
		_ = v.db.Pool.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&rowCount)

		header := fmt.Sprintf("📋 %s  |  Total: %s  |  Table: %s  |  Indexes: %s  |  %d rows",
			table, totalSize, tableSize, indexSize, rowCount)

		result, err := v.db.DescribeTable(ctx, "public", table)
		if err != nil {
			return DescribeResultMsg{Err: err, Header: header}
		}
		indexes, _ := v.db.TableIndexes(ctx, "public", table)
		fks, _ := v.db.TableForeignKeys(ctx, "public", table)
		refs, _ := v.db.TableReferencedBy(ctx, "public", table)

		return DescribeResultMsg{
			Result: result, Indexes: indexes,
			ForeignKeys: fks, ReferencedBy: refs,
			Header: header,
		}
	}
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

	// Check for pagination commands using the saved query plan
	lowerText := strings.ToLower(text)
	if v.lastQueryPlan != nil {
		if strings.Contains(lowerText, "next page") {
			v.lastQueryPlan.Page++
			return v.executeQueryPlan(v.lastQueryPlan)
		}
		if strings.Contains(lowerText, "previous page") || strings.Contains(lowerText, "prev page") {
			if v.lastQueryPlan.Page > 1 {
				v.lastQueryPlan.Page--
			}
			return v.executeQueryPlan(v.lastQueryPlan)
		}
	}

	// Check if a table is selected — if yes, use query plan generation
	if v.tableIdx >= 0 && v.tableIdx < len(v.tables) && v.db != nil {
		return v.generateQueryPlan(text)
	}

	// Fallback to regular chat if no table is selected
	msgs := make([]ai.Message, len(v.chatMessages))
	copy(msgs, v.chatMessages)

	// Inject database context as system message
	ctxMsg := v.buildDBContext()
	if ctxMsg != "" {
		msgs = append([]ai.Message{{Role: "system", Content: ctxMsg}}, msgs...)
	}

	provider := v.aiProvider
	providerName := provider.Name()
	return func() tea.Msg {
		var inputSummary string
		for _, m := range msgs {
			inputSummary += m.Role + ": " + m.Content + "\n"
		}
		ai.LogAIRequest("Chat", providerName, map[string]string{
			"Messages": inputSummary,
		})

		resp, err := provider.Chat(context.Background(), msgs)
		ai.LogAIResponse("Chat", resp, err)
		return AIResponseMsg{Response: resp, Err: err}
	}
}

// generateQueryPlan sends the user's question to the AI with full schema context
// and parses the response into a structured query plan.
func (v *MainView) generateQueryPlan(question string) tea.Cmd {
	provider := v.aiProvider
	database := v.db
	table := v.tables[v.tableIdx]

	// Build data view state string
	var dataViewState string
	if v.lastQueryPlan != nil {
		dataViewState = fmt.Sprintf("Current table: %s, Page: %d, Limit: %d",
			v.lastQueryPlan.Tables[0], v.lastQueryPlan.Page, v.lastQueryPlan.Limit)
		if v.lastQueryPlan.Sort != nil {
			dataViewState += fmt.Sprintf(", Sort: %s %s", v.lastQueryPlan.Sort.Column, v.lastQueryPlan.Sort.Order)
		}
	} else {
		dataViewState = fmt.Sprintf("Current table: %s, Page: 1, Limit: 20", table)
	}

	return func() tea.Msg {
		ctx := context.Background()

		// Fetch schema for the current table
		mainSchema, err := database.FetchTableSchema(ctx, "public", table)
		if err != nil {
			return QueryPlanMsg{Err: fmt.Errorf("failed to fetch schema for %s: %w", table, err)}
		}

		// Fetch schemas for FK-related tables
		relatedSchemas, err := database.FetchRelatedSchemas(ctx, "public", mainSchema)
		if err != nil {
			// Non-fatal — we can still generate a plan without related schemas
			relatedSchemas = make(map[string]*db.TableSchema)
		}

		// Build the schema context text
		schemaContext := db.FormatSchemaContext(mainSchema, relatedSchemas)

		// Log the request
		providerName := fmt.Sprintf("%T", provider)
		ai.LogQueryPlanRequest(providerName, schemaContext, question, dataViewState)

		// Call AI to generate query plan
		rawResponse, err := provider.GenerateQueryPlan(ctx, schemaContext, question, dataViewState)
		if err != nil {
			ai.LogQueryPlanResponse(rawResponse, nil, "", err)
			return QueryPlanMsg{Err: fmt.Errorf("AI error: %w", err), RawResponse: rawResponse}
		}

		// Parse the JSON response into a QueryPlan
		plan, err := ai.ParseQueryPlan(rawResponse)
		if err != nil {
			ai.LogQueryPlanResponse(rawResponse, nil, "", err)
			return QueryPlanMsg{Err: err, RawResponse: rawResponse}
		}

		// Convert the plan to SQL
		sql, err := plan.ToSQL()
		if err != nil {
			ai.LogQueryPlanResponse(rawResponse, plan, "", err)
			return QueryPlanMsg{Plan: plan, Err: fmt.Errorf("SQL generation error: %w", err), RawResponse: rawResponse}
		}

		// Log the successful response
		ai.LogQueryPlanResponse(rawResponse, plan, sql, nil)

		return QueryPlanMsg{Plan: plan, SQL: sql, RawResponse: rawResponse}
	}
}

// executeQueryPlan is used for pagination — it takes an existing plan and
// re-generates the SQL with updated page/sort.
func (v *MainView) executeQueryPlan(plan *ai.QueryPlan) tea.Cmd {
	sql, err := plan.ToSQL()
	if err != nil {
		v.chatMessages = append(v.chatMessages, ai.Message{
			Role:    "assistant",
			Content: "❌ SQL generation error: " + err.Error(),
		})
		v.viewport.SetContentLines(v.renderChatHistory())
		return nil
	}

	// Sync pagination state
	v.pagPage = plan.Page - 1
	v.pagPageSize = plan.Limit

	v.chatMessages = append(v.chatMessages, ai.Message{
		Role:    "assistant",
		Content: fmt.Sprintf("📄 Page %d — Executing...", plan.Page),
	})
	v.viewport.SetContentLines(v.renderChatHistory())
	v.viewport.End()

	v.loading = true
	v.chatLoading = false
	return v.fetchQueryPlanPage(plan, sql)
}

// fetchQueryPlanPage executes a query plan's SQL and builds the same rich info
// header as fetchPage() — table name, sizes, pagination, and sort info.
func (v *MainView) fetchQueryPlanPage(plan *ai.QueryPlan, sql string) tea.Cmd {
	mainTable := ""
	if len(plan.Tables) > 0 {
		mainTable = plan.Tables[0]
	}
	page := plan.Page
	pageSize := plan.Limit
	var sortInfo string
	if plan.Sort != nil && plan.Sort.Column != "" {
		sortInfo = fmt.Sprintf("  |  Sort: %s %s", plan.Sort.Column, strings.ToUpper(plan.Sort.Order))
	}

	// Build the count SQL that matches the same JOINs and filters
	countSQL := plan.ToCountSQL()

	database := v.db
	return func() tea.Msg {
		ctx := context.Background()

		// Get filtered row count (using same JOINs + WHERE as the main query)
		var total int64
		if countSQL != "" {
			_ = database.Pool.QueryRow(ctx, countSQL).Scan(&total)
		}

		// Get table size info for the main table
		var totalSize, tableSize, indexSize string
		if mainTable != "" {
			sizeSQL := `SELECT pg_size_pretty(pg_total_relation_size($1)),
			                   pg_size_pretty(pg_relation_size($1)),
			                   pg_size_pretty(pg_indexes_size($1))`
			_ = database.Pool.QueryRow(ctx, sizeSQL, mainTable).Scan(&totalSize, &tableSize, &indexSize)
		}

		// Build info header (same format as fetchPage)
		info := fmt.Sprintf("📊 %s  |  Total: %s  |  Table: %s  |  Indexes: %s  |  %d rows%s",
			mainTable, totalSize, tableSize, indexSize, total, sortInfo)

		// Add filter info if present
		if len(plan.Filters) > 0 {
			info += "  |  Filters: " + strings.Join(plan.Filters, ", ")
		}

		// Execute the query
		result, err := database.Execute(ctx, sql)
		if result != nil {
			offset := (page - 1) * pageSize
			lastRow := offset + result.RowCount
			totalPages := maxPageCalc(total, int64(pageSize)) + 1
			result.Status = fmt.Sprintf("Page %d/%d  |  Rows %d–%d of %d",
				page, totalPages,
				offset+1, lastRow,
				total)
		}

		return QueryResultMsg{Result: result, Err: err, PagTotal: total, PagInfo: info}
	}
}

// buildDBContext returns a system message with the current database context.
// Used as fallback for regular chat when no table is selected.
func (v *MainView) buildDBContext() string {
	if v.db == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Current Database Context\n")

	// Include selected table schema if available
	if v.tableIdx >= 0 && v.tableIdx < len(v.tables) {
		table := v.tables[v.tableIdx]
		sb.WriteString(fmt.Sprintf("\nSelected table: %s\n", table))

		// Row count
		if v.tableIdx < len(v.tableRows) {
			sb.WriteString(fmt.Sprintf("Estimated rows: %d\n", v.tableRows[v.tableIdx]))
		}

		// Fetch full schema (columns + FKs)
		ctx := context.Background()
		mainSchema, err := v.db.FetchTableSchema(ctx, "public", table)
		if err == nil && mainSchema != nil {
			relatedSchemas, _ := v.db.FetchRelatedSchemas(ctx, "public", mainSchema)
			if relatedSchemas == nil {
				relatedSchemas = make(map[string]*db.TableSchema)
			}
			sb.WriteString("\n")
			sb.WriteString(db.FormatSchemaContext(mainSchema, relatedSchemas))
		}
	}

	sb.WriteString("\nWhen the user asks about 'this table' or gives a natural language query, generate SQL for the selected table above.")
	sb.WriteString("\nAlways output executable PostgreSQL queries the user can copy-paste.")
	return sb.String()
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

// formatResultExpanded renders rows vertically like \x in psql.
func (v *MainView) formatResultExpanded(r *db.QueryResult) []string {
	if r == nil || len(r.Columns) == 0 {
		return []string{r.Status}
	}

	runeLen := utf8.RuneCountInString

	// Find max column name width for alignment
	maxCol := 0
	for _, col := range r.Columns {
		if l := runeLen(col); l > maxCol {
			maxCol = l
		}
	}

	var lines []string
	for rowIdx, row := range r.Rows {
		// Record separator
		sep := fmt.Sprintf("─[ RECORD %d ]─", rowIdx+1)
		lines = append(lines, sep)
		for i, cell := range row {
			if i < len(r.Columns) {
				lines = append(lines, fmt.Sprintf(" %-*s │ %s", maxCol, r.Columns[i], cell))
			}
		}
	}
	lines = append(lines, "", r.Status)
	return lines
}

// renderTableList renders the scrollable table list items.
// maxWidth controls truncation, visibleRows controls the scroll window.
func (v *MainView) renderTableList(maxWidth, visibleRows int) []string {
	if v.tableErr != nil {
		return []string{StyleError.Render("Error: " + v.tableErr.Error())}
	}
	if len(v.tables) == 0 {
		return []string{StyleDimmed.Render(" (no tables)")}
	}

	start := 0
	if v.tableIdx > visibleRows/2 {
		start = v.tableIdx - visibleRows/2
	}
	end := start + visibleRows
	if end > len(v.tables) {
		end = len(v.tables)
	}

	var lines []string
	for i := start; i < end; i++ {
		name := v.tables[i]
		suffix := ""
		if i < len(v.tableRows) {
			suffix = " (" + db.FormatRowCount(v.tableRows[i]) + ")"
		}
		display := name + suffix
		if maxWidth > 4 && len(display) > maxWidth {
			maxName := maxWidth - len(suffix) - 1
			if maxName > 0 && maxName < len(name) {
				display = name[:maxName] + "…" + suffix
			} else if maxWidth > 1 {
				display = display[:maxWidth-1] + "…"
			}
		}
		if i == v.tableIdx {
			if v.focus == focusSidebar {
				lines = append(lines, StyleListItemActive.Render("▸ "+display))
			} else {
				lines = append(lines, StyleDimmed.Render("▸ "+display))
			}
		} else {
			lines = append(lines, StyleDimmed.Render("  "+display))
		}
	}
	return lines
}

func (v *MainView) View() string {
	// ── Fullscreen mode: show only the focused panel ──
	if v.fullscreen {
		hint := StyleDimmed.Render("  [F5 exit fullscreen]")
		switch v.focus {
		case focusSidebar:
			lines := v.renderTableList(v.width, v.height-2)
			header := StyleBold.Render("Tables") + hint
			result := []string{header, ""}
			result = append(result, lines...)
			for len(result) < v.height {
				result = append(result, "")
			}
			return strings.Join(result, "\n")

		case focusResults:
			v.viewport.SetSize(v.width, v.height-1)
			return hint + "\n" + v.viewport.Render()

		case focusInput:
			var label, txt string
			if v.inputMode == inputModeChat {
				label = "Ask> "
				txt = v.chatInput
			} else {
				label = "SQL> "
				txt = v.input
			}
			content := StylePrompt.Render(label) + txt + "█"
			lines := []string{hint, "", content}
			for len(lines) < v.height {
				lines = append(lines, "")
			}
			return strings.Join(lines, "\n")
		}
	}

	// ── Normal layout ──
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
	tableList = append(tableList, v.renderTableList(sidebarWidth-4, v.height)...)

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
