// query_plan.go defines the structured query plan types and SQL generation logic.
//
// The AI returns a JSON query plan (not raw SQL). This file:
//   - Defines the QueryPlan struct matching the AI output format
//   - Parses AI responses into QueryPlan objects
//   - Converts QueryPlan objects into executable PostgreSQL SQL
//
// This separation (NL → JSON → SQL) ensures safety and auditability.
package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// QueryPlan represents a structured query plan returned by the AI.
// It describes what data to fetch without containing raw SQL.
type QueryPlan struct {
	// Tables lists all tables involved in the query.
	Tables []string `json:"tables"`

	// Joins lists join conditions based on foreign keys (e.g. "company.country_id = country.id").
	Joins []string `json:"joins"`

	// Filters lists WHERE conditions (e.g. "country.name = 'China'").
	Filters []string `json:"filters"`

	// Select lists the columns to return (e.g. "company.id", "company.name").
	Select []string `json:"select"`

	// Limit is the number of rows per page.
	Limit int `json:"limit"`

	// Page is the current page number (1-based).
	Page int `json:"page"`

	// Sort specifies the ordering.
	Sort *QueryPlanSort `json:"sort,omitempty"`

	// NeedOtherTables is true when the request cannot be satisfied
	// with the current table and its FK-related tables.
	NeedOtherTables bool `json:"need_other_tables,omitempty"`

	// Action indicates whether this is a SELECT or a modification query.
	// "select" (default), "update", "delete", "insert", "ddl"
	Action string `json:"action,omitempty"`

	// Update fields (only when action is "update")
	UpdateSet map[string]string `json:"update_set,omitempty"` // column → value

	// Insert fields (only when action is "insert")
	InsertColumns []string   `json:"insert_columns,omitempty"`
	InsertValues  [][]string `json:"insert_values,omitempty"`

	// Description is a human-readable description of what the query does.
	Description string `json:"description,omitempty"`
}

// QueryPlanSort defines the sort order.
type QueryPlanSort struct {
	Column string `json:"column"`
	Order  string `json:"order"` // "asc" or "desc"
}

// ParseQueryPlan extracts a QueryPlan from the AI's response text.
// The response may contain markdown fencing or surrounding text,
// so we search for the JSON object within it.
func ParseQueryPlan(response string) (*QueryPlan, error) {
	// Try to extract JSON from the response
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in AI response")
	}

	var plan QueryPlan
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse query plan JSON: %w\nRaw: %s", err, jsonStr)
	}

	// Default action to "select"
	if plan.Action == "" {
		plan.Action = "select"
	}

	// Default limit
	if plan.Limit <= 0 {
		plan.Limit = 20
	}

	// Default page
	if plan.Page <= 0 {
		plan.Page = 1
	}

	return &plan, nil
}

// extractJSON finds the first {...} JSON object in the text,
// handling markdown code fences and surrounding narrative.
func extractJSON(text string) string {
	// Try to extract from markdown code fence
	if idx := strings.Index(text, "```json"); idx >= 0 {
		start := idx + len("```json")
		end := strings.Index(text[start:], "```")
		if end >= 0 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	if idx := strings.Index(text, "```"); idx >= 0 {
		start := idx + len("```")
		end := strings.Index(text[start:], "```")
		if end >= 0 {
			candidate := strings.TrimSpace(text[start : start+end])
			if strings.HasPrefix(candidate, "{") {
				return candidate
			}
		}
	}

	// Try to find raw JSON object by matching braces
	depth := 0
	start := -1
	for i, ch := range text {
		if ch == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && start >= 0 {
				return text[start : i+1]
			}
		}
	}

	return ""
}

// ToSQL converts a QueryPlan into an executable PostgreSQL SQL string.
// Only SELECT plans are auto-executed; modification plans are shown for review.
func (p *QueryPlan) ToSQL() (string, error) {
	switch p.Action {
	case "select", "":
		return p.toSelectSQL()
	case "update":
		return p.toUpdateSQL()
	case "delete":
		return p.toDeleteSQL()
	case "insert":
		return p.toInsertSQL()
	default:
		return "", fmt.Errorf("unsupported action: %s", p.Action)
	}
}

func (p *QueryPlan) toSelectSQL() (string, error) {
	if len(p.Tables) == 0 {
		return "", fmt.Errorf("query plan has no tables")
	}

	// SELECT columns
	selectCols := "*"
	if len(p.Select) > 0 {
		selectCols = strings.Join(p.Select, ", ")
	}

	sql := fmt.Sprintf("SELECT %s\nFROM %s", selectCols, p.buildFromClause())

	// WHERE
	if len(p.Filters) > 0 {
		sql += "\nWHERE " + strings.Join(p.Filters, "\n  AND ")
	}

	// ORDER BY
	if p.Sort != nil && p.Sort.Column != "" {
		order := strings.ToUpper(p.Sort.Order)
		if order != "DESC" {
			order = "ASC"
		}
		sql += fmt.Sprintf("\nORDER BY %s %s", p.Sort.Column, order)
	}

	// LIMIT & OFFSET
	sql += fmt.Sprintf("\nLIMIT %d", p.Limit)
	if p.Page > 1 {
		offset := (p.Page - 1) * p.Limit
		sql += fmt.Sprintf(" OFFSET %d", offset)
	}

	return sql, nil
}

// ToCountSQL generates a COUNT(*) query with the same FROM/JOIN/WHERE
// as the SELECT query, so filtered results get accurate counts.
func (p *QueryPlan) ToCountSQL() string {
	if len(p.Tables) == 0 {
		return ""
	}

	sql := fmt.Sprintf("SELECT count(*) FROM %s", p.buildFromClause())

	if len(p.Filters) > 0 {
		sql += "\nWHERE " + strings.Join(p.Filters, "\n  AND ")
	}

	return sql
}

// buildFromClause builds the FROM + JOIN clause shared by SELECT and COUNT queries.
func (p *QueryPlan) buildFromClause() string {
	fromClause := p.Tables[0]
	for i := 1; i < len(p.Tables); i++ {
		joinCond := ""
		// Find the join condition for this table
		for _, j := range p.Joins {
			if strings.Contains(j, p.Tables[i]+".") {
				joinCond = j
				break
			}
		}
		if joinCond != "" {
			fromClause += fmt.Sprintf("\nJOIN %s ON %s", p.Tables[i], joinCond)
		} else {
			fromClause += fmt.Sprintf("\nCROSS JOIN %s", p.Tables[i])
		}
	}
	return fromClause
}

func (p *QueryPlan) toUpdateSQL() (string, error) {
	if len(p.Tables) == 0 {
		return "", fmt.Errorf("update plan has no tables")
	}
	if len(p.UpdateSet) == 0 {
		return "", fmt.Errorf("update plan has no SET values")
	}

	var setClauses []string
	for col, val := range p.UpdateSet {
		setClauses = append(setClauses, fmt.Sprintf("%s = %s", col, val))
	}

	sql := fmt.Sprintf("UPDATE %s\nSET %s", p.Tables[0], strings.Join(setClauses, ", "))

	if len(p.Filters) > 0 {
		sql += "\nWHERE " + strings.Join(p.Filters, "\n  AND ")
	}

	return sql, nil
}

func (p *QueryPlan) toDeleteSQL() (string, error) {
	if len(p.Tables) == 0 {
		return "", fmt.Errorf("delete plan has no tables")
	}

	sql := fmt.Sprintf("DELETE FROM %s", p.Tables[0])
	if len(p.Filters) > 0 {
		sql += "\nWHERE " + strings.Join(p.Filters, "\n  AND ")
	}

	return sql, nil
}

func (p *QueryPlan) toInsertSQL() (string, error) {
	if len(p.Tables) == 0 {
		return "", fmt.Errorf("insert plan has no tables")
	}
	if len(p.InsertColumns) == 0 || len(p.InsertValues) == 0 {
		return "", fmt.Errorf("insert plan has no columns or values")
	}

	var valueRows []string
	for _, row := range p.InsertValues {
		valueRows = append(valueRows, "("+strings.Join(row, ", ")+")")
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s)\nVALUES %s",
		p.Tables[0],
		strings.Join(p.InsertColumns, ", "),
		strings.Join(valueRows, ",\n       "))

	return sql, nil
}

// IsReadOnly returns true if the query plan is a SELECT (safe to auto-execute).
func (p *QueryPlan) IsReadOnly() bool {
	return p.Action == "select" || p.Action == ""
}

// Summary returns a short human-readable summary of the query plan.
func (p *QueryPlan) Summary() string {
	if p.NeedOtherTables {
		return "❌ Cannot satisfy this query with the current table and its related tables."
	}
	if p.Description != "" {
		return p.Description
	}

	action := strings.ToUpper(p.Action)
	if action == "" {
		action = "SELECT"
	}

	tables := strings.Join(p.Tables, ", ")
	summary := fmt.Sprintf("%s on %s", action, tables)

	if len(p.Filters) > 0 {
		summary += " where " + strings.Join(p.Filters, " and ")
	}
	if p.Sort != nil {
		summary += fmt.Sprintf(" order by %s %s", p.Sort.Column, p.Sort.Order)
	}
	if p.Limit > 0 {
		summary += fmt.Sprintf(" (limit %d, page %d)", p.Limit, p.Page)
	}

	return summary
}
