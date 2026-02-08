// messages.go defines Bubble Tea messages used for async communication.
//
// All database operations and AI requests send results back to the
// TUI via these message types, ensuring the UI never blocks.
package tui

import (
	"github.com/DachengChen/paiSQL/ai"
	"github.com/DachengChen/paiSQL/db"
)

// QueryResultMsg is sent when a SQL query completes.
type QueryResultMsg struct {
	Result   *db.QueryResult
	Err      error
	PagTotal int64  // total rows for pagination (0 = not paginated)
	PagInfo  string // table info header (name, size, etc.)
}

// ExplainResultMsg is sent when an EXPLAIN query completes.
type ExplainResultMsg struct {
	Result *db.ExplainResult
	Err    error
}

// TablesListMsg is sent when \dt, \di, \dv completes.
type TablesListMsg struct {
	Tables []db.TableInfo
	Err    error
}

// DescribeResultMsg is sent when a table describe completes.
type DescribeResultMsg struct {
	Result       *db.QueryResult // columns
	Indexes      *db.QueryResult
	ForeignKeys  *db.QueryResult
	ReferencedBy *db.QueryResult
	Header       string
	Err          error
}

// AIResponseMsg is sent when an AI request completes.
type AIResponseMsg struct {
	Response string
	Err      error
}

// IndexSuggestionMsg is sent when AI index analysis completes.
type IndexSuggestionMsg struct {
	Suggestion string
	Err        error
}

// StatsMsg carries database statistics.
type StatsMsg struct {
	Lines []string
	Err   error
}

// LogMsg carries a new log line from tail.
type LogMsg struct {
	Line string
	Err  error
}

// StatusMsg is a transient status message for the status bar.
type StatusMsg string

// QueryPlanMsg is sent when an AI query plan generation completes.
type QueryPlanMsg struct {
	Plan        *ai.QueryPlan // parsed query plan
	SQL         string        // generated SQL from the plan
	RawResponse string        // raw AI response (for debugging)
	Err         error
}

// AntigravityLoginMsg is sent when Google Antigravity OAuth login completes.
type AntigravityLoginMsg struct {
	Err error
}
