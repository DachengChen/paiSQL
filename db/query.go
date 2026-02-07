// query.go implements psql-like meta-commands and arbitrary SQL execution.
//
// All functions accept a context and return structured results that the
// TUI layer can render. Errors are returned, never logged or printed.
package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// TableInfo represents a database object (table, index, view).
type TableInfo struct {
	Schema string
	Name   string
	Type   string // "table", "index", "view"
	Owner  string
}

// QueryResult holds the output of an arbitrary SQL query.
type QueryResult struct {
	Columns  []string
	Rows     [][]string
	RowCount int
	Status   string // e.g. "SELECT 5", "INSERT 0 1"
}

// ExplainResult holds a JSON explain plan.
type ExplainResult struct {
	JSON string
}

// ListTables implements \dt — list tables in the current database.
func (d *DB) ListTables(ctx context.Context, schema string) ([]TableInfo, error) {
	if schema == "" {
		schema = "public"
	}
	return d.listObjects(ctx, schema, "BASE TABLE", "table")
}

// ListIndexes implements \di — list indexes.
func (d *DB) ListIndexes(ctx context.Context, schema string) ([]TableInfo, error) {
	query := `
		SELECT schemaname, indexname, 'index' AS type, ''
		FROM pg_indexes
		WHERE schemaname = $1
		ORDER BY indexname`
	rows, err := d.Pool.Query(ctx, query, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Schema, &t.Name, &t.Type, &t.Owner); err != nil {
			return nil, err
		}
		results = append(results, t)
	}
	return results, rows.Err()
}

// ListViews implements \dv — list views.
func (d *DB) ListViews(ctx context.Context, schema string) ([]TableInfo, error) {
	return d.listObjects(ctx, schema, "VIEW", "view")
}

// listObjects queries information_schema.tables for a given table_type.
func (d *DB) listObjects(ctx context.Context, schema, tableType, label string) ([]TableInfo, error) {
	query := `
		SELECT table_schema, table_name, $1::text AS type, ''
		FROM information_schema.tables
		WHERE table_schema = $2 AND table_type = $3
		ORDER BY table_name`
	rows, err := d.Pool.Query(ctx, query, label, schema, tableType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Schema, &t.Name, &t.Type, &t.Owner); err != nil {
			return nil, err
		}
		results = append(results, t)
	}
	return results, rows.Err()
}

// DescribeTable implements \d <table> — show columns for a table.
func (d *DB) DescribeTable(ctx context.Context, schema, table string) (*QueryResult, error) {
	if schema == "" {
		schema = "public"
	}
	query := `
		SELECT column_name, data_type,
		       COALESCE(character_maximum_length::text, ''),
		       COALESCE(column_default, ''),
		       is_nullable
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`
	return d.executeQuery(ctx, query, schema, table)
}

// Execute runs an arbitrary SQL statement and returns results.
func (d *DB) Execute(ctx context.Context, sql string) (*QueryResult, error) {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return nil, fmt.Errorf("empty query")
	}
	return d.executeQuery(ctx, sql)
}

// Explain runs EXPLAIN (ANALYZE, FORMAT JSON) on a query.
func (d *DB) Explain(ctx context.Context, sql string, analyze bool) (*ExplainResult, error) {
	prefix := "EXPLAIN (FORMAT JSON)"
	if analyze {
		prefix = "EXPLAIN (ANALYZE, FORMAT JSON)"
	}
	explainSQL := prefix + " " + sql

	var jsonPlan string
	err := d.Pool.QueryRow(ctx, explainSQL).Scan(&jsonPlan)
	if err != nil {
		return nil, err
	}
	return &ExplainResult{JSON: jsonPlan}, nil
}

// executeQuery is the internal workhorse for running SQL and collecting results.
func (d *DB) executeQuery(ctx context.Context, sql string, args ...any) (*QueryResult, error) {
	rows, err := d.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &QueryResult{}

	// Extract column names
	for _, fd := range rows.FieldDescriptions() {
		result.Columns = append(result.Columns, fd.Name)
	}

	// Collect rows
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make([]string, len(values))
		for i, v := range values {
			row[i] = fmt.Sprintf("%v", v)
		}
		result.Rows = append(result.Rows, row)
		result.RowCount++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result.Status = fmt.Sprintf("(%d row%s)", result.RowCount, plural(result.RowCount))
	return result, nil
}

// Transaction helpers — thin wrappers so the TUI can manage transactions.

func (d *DB) Begin(ctx context.Context) (pgx.Tx, error) {
	return d.Pool.Begin(ctx)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
