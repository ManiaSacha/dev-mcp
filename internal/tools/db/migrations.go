package db

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// ToolMigrations lists migration files and which versions are already applied.
	ToolMigrations = "db_migrations"

	trackerNone    = "none"
	trackerMigrate = "golang-migrate"
	trackerGoose   = "goose"

	maxMigrationFiles = 1000
)

var (
	upDownFile = regexp.MustCompile(`^(\d+)_([A-Za-z0-9][A-Za-z0-9_-]*)\.(up|down)\.sql$`)
	plainFile  = regexp.MustCompile(`^(\d+)_([A-Za-z0-9][A-Za-z0-9_-]*)\.sql$`)
)

// MigrationsArgs selects a migration directory and, optionally, a SQLite file.
type MigrationsArgs struct {
	Directory string `json:"directory" jsonschema:"directory of migration files. Files are read by name only; SQL is not executed."`
	Database  string `json:"database,omitempty" jsonschema:"optional SQLite database file used to compare applied versions"`
}

// MigrationFile is one recognized migration filename.
type MigrationFile struct {
	Version   string `json:"version" jsonschema:"numeric version prefix"`
	Name      string `json:"name" jsonschema:"migration name from the filename"`
	Direction string `json:"direction" jsonschema:"up, down, or sql"`
	File      string `json:"file" jsonschema:"filename"`
}

// MigrationsResult compares migration files with an optional database.
type MigrationsResult struct {
	Directory    string          `json:"directory" jsonschema:"directory that was read"`
	Database     string          `json:"database,omitempty" jsonschema:"SQLite file that was compared, when one was provided"`
	Tracker      string          `json:"tracker" jsonschema:"none, golang-migrate, or goose"`
	Dirty        bool            `json:"dirty" jsonschema:"true when golang-migrate recorded a failed migration"`
	Files        []MigrationFile `json:"files" jsonschema:"recognized migration files"`
	Pending      []string        `json:"pending" jsonschema:"forward versions that are not applied"`
	FileCount    int             `json:"fileCount" jsonschema:"number of recognized files returned"`
	PendingCount int             `json:"pendingCount" jsonschema:"number of pending versions"`
	Truncated    bool            `json:"truncated" jsonschema:"true when the file limit was hit"`
}

// Migrations lists migration files. When a database is set, it reports which
// forward versions are not applied yet. It does not execute SQL.
func (Handler) Migrations(ctx context.Context, _ *mcp.CallToolRequest, args MigrationsArgs) (*mcp.CallToolResult, MigrationsResult, error) {
	var zero MigrationsResult
	if err := ctx.Err(); err != nil {
		return nil, zero, toolErr(ToolMigrations, err)
	}
	if err := validateDirPath(args.Directory); err != nil {
		return nil, zero, toolErr(ToolMigrations, err)
	}
	files, truncated, err := listMigrationFiles(args.Directory)
	if err != nil {
		return nil, zero, toolErr(ToolMigrations, err)
	}

	state := appliedState{tracker: trackerNone}
	if args.Database != "" {
		database, err := openReadOnly(ctx, args.Database)
		if err != nil {
			return nil, zero, toolErr(ToolMigrations, err)
		}
		state, err = readApplied(ctx, database)
		_ = database.Close()
		if err != nil {
			return nil, zero, toolErr(ToolMigrations, err)
		}
	}

	pending := pendingVersions(files, state)
	return nil, MigrationsResult{
		Directory:    args.Directory,
		Database:     args.Database,
		Tracker:      state.tracker,
		Dirty:        state.dirty,
		Files:        files,
		Pending:      pending,
		FileCount:    len(files),
		PendingCount: len(pending),
		Truncated:    truncated,
	}, nil
}

type appliedState struct {
	tracker      string
	dirty        bool
	ceiling      string
	ceilingKnown bool
	applied      map[string]struct{}
}

func listMigrationFiles(dir string) ([]MigrationFile, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false, err
	}
	files := []MigrationFile{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		file, ok := parseMigrationName(entry.Name())
		if !ok {
			continue
		}
		files = append(files, file)
	}
	slices.SortFunc(files, func(a, b MigrationFile) int {
		if cmp := versionCmp(a.Version, b.Version); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(directionOrder(a.Direction), directionOrder(b.Direction)); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.File, b.File)
	})
	if err := duplicateForwardVersions(files); err != nil {
		return nil, false, err
	}
	if len(files) > maxMigrationFiles {
		return files[:maxMigrationFiles], true, nil
	}
	return files, false, nil
}

func parseMigrationName(name string) (MigrationFile, bool) {
	if parts := upDownFile.FindStringSubmatch(name); parts != nil {
		return MigrationFile{Version: parts[1], Name: parts[2], Direction: parts[3], File: name}, true
	}
	if parts := plainFile.FindStringSubmatch(name); parts != nil {
		return MigrationFile{Version: parts[1], Name: parts[2], Direction: "sql", File: name}, true
	}
	return MigrationFile{}, false
}

func directionOrder(direction string) string {
	switch direction {
	case "up":
		return "1"
	case "sql":
		return "2"
	default:
		return "3"
	}
}

func duplicateForwardVersions(files []MigrationFile) error {
	seen := map[string]string{}
	for _, file := range files {
		if file.Direction == "down" {
			continue
		}
		if previous, ok := seen[file.Version]; ok {
			return fmt.Errorf("version %s is declared by both %s and %s", file.Version, previous, file.File)
		}
		seen[file.Version] = file.File
	}
	return nil
}

func readApplied(ctx context.Context, database *sql.DB) (appliedState, error) {
	rows, err := database.QueryContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name IN ('schema_migrations', 'goose_db_version')
		ORDER BY name`)
	if err != nil {
		return appliedState{}, err
	}
	defer closeQuiet(rows)

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return appliedState{}, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return appliedState{}, err
	}
	hasMigrate := slices.Contains(names, "schema_migrations")
	hasGoose := slices.Contains(names, "goose_db_version")
	if hasMigrate {
		return readGolangMigrate(ctx, database)
	}
	if hasGoose {
		return readGoose(ctx, database)
	}
	return appliedState{tracker: trackerNone, applied: map[string]struct{}{}}, nil
}

func readGolangMigrate(ctx context.Context, database *sql.DB) (appliedState, error) {
	rows, err := database.QueryContext(ctx, `SELECT version, dirty FROM schema_migrations`)
	if err != nil {
		return appliedState{}, err
	}
	defer closeQuiet(rows)

	state := appliedState{tracker: trackerMigrate}
	for rows.Next() {
		var version int64
		var dirty int
		if err := rows.Scan(&version, &dirty); err != nil {
			return appliedState{}, err
		}
		text := fmt.Sprintf("%d", version)
		if !state.ceilingKnown || versionCmp(text, state.ceiling) > 0 {
			state.ceiling = text
			state.ceilingKnown = true
			state.dirty = dirty != 0
		}
	}
	if err := rows.Err(); err != nil {
		return appliedState{}, err
	}
	return state, nil
}

func readGoose(ctx context.Context, database *sql.DB) (appliedState, error) {
	rows, err := database.QueryContext(ctx, `SELECT version_id, is_applied FROM goose_db_version`)
	if err != nil {
		return appliedState{}, err
	}
	defer closeQuiet(rows)

	state := appliedState{tracker: trackerGoose, applied: map[string]struct{}{}}
	for rows.Next() {
		var version int64
		var applied int
		if err := rows.Scan(&version, &applied); err != nil {
			return appliedState{}, err
		}
		if applied == 0 || version == 0 {
			continue
		}
		state.applied[fmt.Sprintf("%d", version)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return appliedState{}, err
	}
	return state, nil
}

func pendingVersions(files []MigrationFile, state appliedState) []string {
	pending := []string{}
	seen := map[string]struct{}{}
	for _, file := range files {
		if file.Direction == "down" {
			continue
		}
		if _, ok := seen[file.Version]; ok {
			continue
		}
		seen[file.Version] = struct{}{}
		if !isApplied(file.Version, state) {
			pending = append(pending, file.Version)
		}
	}
	return pending
}

func isApplied(version string, state appliedState) bool {
	switch state.tracker {
	case trackerMigrate:
		if !state.ceilingKnown {
			return false
		}
		cmp := versionCmp(version, state.ceiling)
		if cmp < 0 {
			return true
		}
		return cmp == 0 && !state.dirty
	case trackerGoose:
		_, ok := state.applied[normalizeVersion(version)]
		return ok
	default:
		return false
	}
}

func normalizeVersion(version string) string {
	parsed, ok := new(big.Int).SetString(version, 10)
	if !ok {
		return version
	}
	return parsed.String()
}

func versionCmp(a, b string) int {
	ia, oka := new(big.Int).SetString(a, 10)
	ib, okb := new(big.Int).SetString(b, 10)
	if oka && okb {
		return ia.Cmp(ib)
	}
	return strings.Compare(a, b)
}
