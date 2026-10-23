package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ManiaSacha/dev-mcp/internal/redaction"
)

const (
	// ToolSchema reads tables, columns, indexes, and foreign keys.
	ToolSchema = "db_schema"

	maxTables      = 200
	maxColumns     = 200
	maxIndexes     = 50
	maxForeignKeys = 50
)

// SchemaArgs selects a SQLite database file.
type SchemaArgs struct {
	Path string `json:"path" jsonschema:"path of a SQLite database file"`
}

// Column is one table column.
type Column struct {
	Name       string `json:"name" jsonschema:"column name"`
	Type       string `json:"type" jsonschema:"declared column type"`
	NotNull    bool   `json:"notNull" jsonschema:"true when the column is NOT NULL"`
	PrimaryKey bool   `json:"primaryKey" jsonschema:"true when the column is part of the primary key"`
	Default    string `json:"default,omitempty" jsonschema:"redacted default expression, when one is declared"`
}

// Index is one SQL index.
type Index struct {
	Name    string   `json:"name" jsonschema:"index name"`
	Unique  bool     `json:"unique" jsonschema:"true when the index is unique"`
	Columns []string `json:"columns" jsonschema:"indexed columns, in order"`
}

// ForeignKey is one foreign-key column mapping.
type ForeignKey struct {
	Table string `json:"table" jsonschema:"referenced table"`
	From  string `json:"from" jsonschema:"column in this table"`
	To    string `json:"to" jsonschema:"column in the referenced table"`
}

// Table is one user table.
type Table struct {
	Name        string       `json:"name" jsonschema:"table name"`
	Columns     []Column     `json:"columns" jsonschema:"columns in declared order"`
	Indexes     []Index      `json:"indexes" jsonschema:"indexes on this table"`
	ForeignKeys []ForeignKey `json:"foreignKeys" jsonschema:"foreign keys on this table"`
}

// SchemaResult is the database structure.
type SchemaResult struct {
	Path       string  `json:"path" jsonschema:"database file that was read"`
	Tables     []Table `json:"tables" jsonschema:"user tables, excluding sqlite internal tables"`
	TableCount int     `json:"tableCount" jsonschema:"number of tables returned"`
	Truncated  bool    `json:"truncated" jsonschema:"true when a table, column, index, or foreign-key limit was hit"`
}

// Schema reads the structure of a SQLite database. It does not return table rows.
func (Handler) Schema(ctx context.Context, _ *mcp.CallToolRequest, args SchemaArgs) (*mcp.CallToolResult, SchemaResult, error) {
	var zero SchemaResult
	database, err := openReadOnly(ctx, args.Path)
	if err != nil {
		return nil, zero, toolErr(ToolSchema, err)
	}
	defer closeQuiet(database)

	result, err := readSchema(ctx, database, args.Path)
	if err != nil {
		return nil, zero, toolErr(ToolSchema, err)
	}
	return nil, result, nil
}

func readSchema(ctx context.Context, database *sql.DB, path string) (SchemaResult, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name`)
	if err != nil {
		return SchemaResult{}, err
	}
	defer closeQuiet(rows)

	names := []string{}
	truncated := false
	for rows.Next() {
		if len(names) >= maxTables {
			truncated = true
			break
		}
		var name string
		if err := rows.Scan(&name); err != nil {
			return SchemaResult{}, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return SchemaResult{}, err
	}
	if err := rows.Close(); err != nil {
		return SchemaResult{}, err
	}

	result := SchemaResult{Path: path, Tables: []Table{}, Truncated: truncated}
	for _, name := range names {
		table, tableTruncated, err := readTable(ctx, database, name)
		if err != nil {
			return SchemaResult{}, err
		}
		if tableTruncated {
			result.Truncated = true
		}
		result.Tables = append(result.Tables, table)
	}
	result.TableCount = len(result.Tables)
	return result, nil
}

func readTable(ctx context.Context, database *sql.DB, name string) (Table, bool, error) {
	table := Table{
		Name:        name,
		Columns:     []Column{},
		Indexes:     []Index{},
		ForeignKeys: []ForeignKey{},
	}
	truncated := false

	columns, columnTruncated, err := readColumns(ctx, database, name)
	if err != nil {
		return Table{}, false, err
	}
	table.Columns = columns
	truncated = columnTruncated

	indexes, indexTruncated, err := readIndexes(ctx, database, name)
	if err != nil {
		return Table{}, false, err
	}
	table.Indexes = indexes
	truncated = truncated || indexTruncated

	keys, keyTruncated, err := readForeignKeys(ctx, database, name)
	if err != nil {
		return Table{}, false, err
	}
	table.ForeignKeys = keys
	return table, truncated || keyTruncated, nil
}

func readColumns(ctx context.Context, database *sql.DB, table string) ([]Column, bool, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT name, type, "notnull", dflt_value, pk
		FROM pragma_table_info(?)
		ORDER BY cid`, table)
	if err != nil {
		return nil, false, fmt.Errorf("columns for %s: %w", table, err)
	}
	defer closeQuiet(rows)

	columns := []Column{}
	truncated := false
	for rows.Next() {
		if len(columns) >= maxColumns {
			truncated = true
			break
		}
		var column Column
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&column.Name, &column.Type, &notNull, &dflt, &pk); err != nil {
			return nil, false, err
		}
		column.NotNull = notNull != 0
		column.PrimaryKey = pk != 0
		if dflt.Valid {
			masked, _ := redaction.Redact(dflt.String)
			column.Default = masked
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return columns, truncated, nil
}

func readIndexes(ctx context.Context, database *sql.DB, table string) ([]Index, bool, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT name, "unique" FROM pragma_index_list(?)
		ORDER BY seq`, table)
	if err != nil {
		return nil, false, fmt.Errorf("indexes for %s: %w", table, err)
	}
	defer closeQuiet(rows)

	type listed struct {
		name   string
		unique bool
	}
	var listedIndexes []listed
	truncated := false
	for rows.Next() {
		if len(listedIndexes) >= maxIndexes {
			truncated = true
			break
		}
		var name string
		var unique int
		if err := rows.Scan(&name, &unique); err != nil {
			return nil, false, err
		}
		listedIndexes = append(listedIndexes, listed{name: name, unique: unique != 0})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if err := rows.Close(); err != nil {
		return nil, false, err
	}

	indexes := []Index{}
	for _, item := range listedIndexes {
		columns, err := readIndexColumns(ctx, database, item.name)
		if err != nil {
			return nil, false, err
		}
		indexes = append(indexes, Index{Name: item.name, Unique: item.unique, Columns: columns})
	}
	return indexes, truncated, nil
}

func readIndexColumns(ctx context.Context, database *sql.DB, index string) ([]string, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT name FROM pragma_index_info(?)
		ORDER BY seqno`, index)
	if err != nil {
		return nil, err
	}
	defer closeQuiet(rows)

	columns := []string{}
	for rows.Next() {
		var name sql.NullString
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if name.Valid {
			columns = append(columns, name.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

func readForeignKeys(ctx context.Context, database *sql.DB, table string) ([]ForeignKey, bool, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT "table", "from", "to" FROM pragma_foreign_key_list(?)
		ORDER BY id, seq`, table)
	if err != nil {
		return nil, false, fmt.Errorf("foreign keys for %s: %w", table, err)
	}
	defer closeQuiet(rows)

	keys := []ForeignKey{}
	truncated := false
	for rows.Next() {
		if len(keys) >= maxForeignKeys {
			truncated = true
			break
		}
		var key ForeignKey
		if err := rows.Scan(&key.Table, &key.From, &key.To); err != nil {
			return nil, false, err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return keys, truncated, nil
}
