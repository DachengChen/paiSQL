// schema.go provides functions to fetch table schema information
// for building AI query plan context.
//
// These functions gather:
//   - Column definitions (name, type, nullable, default, PK)
//   - Foreign key relationships (outgoing)
//   - Referenced table schemas (for FK-related tables)
//
// The output is formatted as a text block suitable for injection
// into an AI system prompt.
package db

import (
	"context"
	"fmt"
	"strings"
)

// ColumnInfo describes a single column in a table.
type ColumnInfo struct {
	Name       string
	DataType   string
	IsNullable bool
	Default    string
	IsPK       bool
}

// ForeignKeyInfo describes a foreign key constraint.
type ForeignKeyInfo struct {
	ConstraintName string
	Column         string
	ForeignTable   string
	ForeignColumn  string
}

// TableSchema holds complete schema information for a table.
type TableSchema struct {
	Name        string
	Columns     []ColumnInfo
	ForeignKeys []ForeignKeyInfo
}

// FetchTableSchema retrieves columns and foreign keys for a table.
func (d *DB) FetchTableSchema(ctx context.Context, schema, table string) (*TableSchema, error) {
	if schema == "" {
		schema = "public"
	}

	ts := &TableSchema{Name: table}

	// Fetch columns
	colResult, err := d.DescribeTable(ctx, schema, table)
	if err != nil {
		return nil, fmt.Errorf("describe %s: %w", table, err)
	}

	for _, row := range colResult.Rows {
		if len(row) < 5 {
			continue
		}
		col := ColumnInfo{
			Name:       row[0],
			DataType:   row[1],
			IsNullable: row[2] == "YES",
			Default:    row[3],
			IsPK:       row[4] == "PK",
		}
		ts.Columns = append(ts.Columns, col)
	}

	// Fetch formal foreign keys
	fkResult, err := d.TableForeignKeys(ctx, schema, table)
	if err != nil {
		return nil, fmt.Errorf("foreign keys %s: %w", table, err)
	}

	for _, row := range fkResult.Rows {
		if len(row) < 4 {
			continue
		}
		fk := ForeignKeyInfo{
			ConstraintName: row[0],
			Column:         row[1],
			ForeignTable:   row[2],
			ForeignColumn:  row[3],
		}
		ts.ForeignKeys = append(ts.ForeignKeys, fk)
	}

	// If no formal FKs found, detect implicit FKs from *_id column naming convention.
	// Many databases use "country_id" → country.id without formal constraints.
	if len(ts.ForeignKeys) == 0 {
		implicitFKs := d.detectImplicitFKs(ctx, schema, ts)
		ts.ForeignKeys = append(ts.ForeignKeys, implicitFKs...)
	}

	return ts, nil
}

// detectImplicitFKs scans columns ending in "_id" and checks if a matching
// table exists. This handles the common convention where FK relationships
// are implied by naming but not enforced by constraints.
func (d *DB) detectImplicitFKs(ctx context.Context, schema string, ts *TableSchema) []ForeignKeyInfo {
	var fks []ForeignKeyInfo

	for _, col := range ts.Columns {
		if col.IsPK {
			continue // skip the table's own PK
		}
		if !strings.HasSuffix(col.Name, "_id") {
			continue
		}

		// "country_id" → "country"
		refTable := strings.TrimSuffix(col.Name, "_id")

		// Check if that table actually exists
		var exists bool
		checkSQL := `SELECT EXISTS(
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = $1 AND table_name = $2
		)`
		err := d.Pool.QueryRow(ctx, checkSQL, schema, refTable).Scan(&exists)
		if err != nil || !exists {
			continue
		}

		fks = append(fks, ForeignKeyInfo{
			ConstraintName: "(implicit)",
			Column:         col.Name,
			ForeignTable:   refTable,
			ForeignColumn:  "id",
		})
	}

	return fks
}

// FetchRelatedSchemas retrieves schemas for all tables referenced by
// the given table's foreign keys.
func (d *DB) FetchRelatedSchemas(ctx context.Context, schema string, mainSchema *TableSchema) (map[string]*TableSchema, error) {
	if schema == "" {
		schema = "public"
	}

	related := make(map[string]*TableSchema)

	// Collect unique referenced tables
	seen := make(map[string]bool)
	for _, fk := range mainSchema.ForeignKeys {
		if seen[fk.ForeignTable] {
			continue
		}
		seen[fk.ForeignTable] = true

		ts, err := d.FetchTableSchema(ctx, schema, fk.ForeignTable)
		if err != nil {
			// Skip tables we can't describe (e.g. missing permissions)
			continue
		}
		related[fk.ForeignTable] = ts
	}

	return related, nil
}

// FormatSchemaContext builds a text description of the current table
// and its related tables, suitable for the AI system prompt.
func FormatSchemaContext(current *TableSchema, related map[string]*TableSchema) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Current Table: %s\n\n", current.Name))

	// Columns
	sb.WriteString("### Columns\n")
	for _, col := range current.Columns {
		nullable := "NULL"
		if !col.IsNullable {
			nullable = "NOT NULL"
		}
		pk := ""
		if col.IsPK {
			pk = " [PK]"
		}
		def := ""
		if col.Default != "" {
			def = " DEFAULT " + col.Default
		}
		sb.WriteString(fmt.Sprintf("- %s %s %s%s%s\n", col.Name, col.DataType, nullable, pk, def))
	}

	// Foreign Keys
	if len(current.ForeignKeys) > 0 {
		sb.WriteString("\n### Foreign Keys\n")
		for _, fk := range current.ForeignKeys {
			sb.WriteString(fmt.Sprintf("- %s.%s → %s.%s (constraint: %s)\n",
				current.Name, fk.Column, fk.ForeignTable, fk.ForeignColumn, fk.ConstraintName))
		}
	}

	// Related tables
	if len(related) > 0 {
		sb.WriteString("\n## Related Tables (via Foreign Keys)\n")
		for name, ts := range related {
			sb.WriteString(fmt.Sprintf("\n### %s\n", name))
			sb.WriteString("Columns:\n")
			for _, col := range ts.Columns {
				nullable := "NULL"
				if !col.IsNullable {
					nullable = "NOT NULL"
				}
				pk := ""
				if col.IsPK {
					pk = " [PK]"
				}
				sb.WriteString(fmt.Sprintf("- %s %s %s%s\n", col.Name, col.DataType, nullable, pk))
			}
		}
	}

	return sb.String()
}
