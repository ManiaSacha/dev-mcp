package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaReadsStructure(t *testing.T) {
	t.Parallel()

	path := writeSQLite(t, []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			email TEXT NOT NULL,
			note TEXT DEFAULT 'password=examplevalue'
		)`,
		`CREATE TABLE posts (
			id INTEGER PRIMARY KEY,
			user_id INTEGER NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		`CREATE INDEX posts_user ON posts(user_id)`,
		`INSERT INTO users (email) VALUES ('ada@example.test')`,
	})

	_, result, err := (Handler{}).Schema(t.Context(), nil, SchemaArgs{Path: path})
	if err != nil {
		t.Fatalf("Schema() error = %v", err)
	}
	if result.TableCount != 2 || result.Truncated || result.Path != path {
		t.Fatalf("Schema() = %+v", result)
	}
	if result.Tables[0].Name != "posts" || result.Tables[1].Name != "users" {
		t.Fatalf("tables = %+v", result.Tables)
	}

	posts := result.Tables[0]
	if len(posts.ForeignKeys) != 1 || posts.ForeignKeys[0].Table != "users" || posts.ForeignKeys[0].From != "user_id" || posts.ForeignKeys[0].To != "id" {
		t.Fatalf("foreign keys = %+v", posts.ForeignKeys)
	}
	if !hasIndex(posts.Indexes, "posts_user", "user_id") {
		t.Fatalf("indexes = %+v", posts.Indexes)
	}

	users := result.Tables[1]
	email := columnNamed(t, users.Columns, "email")
	if email.Type != "TEXT" || !email.NotNull || email.PrimaryKey {
		t.Fatalf("email = %+v", email)
	}
	id := columnNamed(t, users.Columns, "id")
	if !id.PrimaryKey {
		t.Fatalf("id = %+v", id)
	}
	note := columnNamed(t, users.Columns, "note")
	if !strings.Contains(note.Default, "[REDACTED]") || strings.Contains(note.Default, "examplevalue") {
		t.Fatalf("note default = %q", note.Default)
	}
	for _, table := range result.Tables {
		if strings.HasPrefix(table.Name, "sqlite_") {
			t.Fatalf("internal table returned: %s", table.Name)
		}
	}
}

func TestMigrationsPending(t *testing.T) {
	t.Parallel()

	dir := writeMigrations(t)
	h := Handler{}

	_, none, err := h.Migrations(t.Context(), nil, MigrationsArgs{Directory: dir})
	if err != nil {
		t.Fatalf("Migrations() error = %v", err)
	}
	if none.Tracker != trackerNone || none.FileCount != 3 || strings.Join(none.Pending, ",") != "1,2" {
		t.Fatalf("no tracker = %+v", none)
	}
	if none.Files[0].Direction != "up" || none.Files[1].Direction != "down" || none.Files[2].File != "2_add_note.up.sql" {
		t.Fatalf("files = %+v", none.Files)
	}

	clean := writeSQLite(t, []string{
		`CREATE TABLE schema_migrations (version INTEGER, dirty INTEGER)`,
		`INSERT INTO schema_migrations (version, dirty) VALUES (1, 0)`,
	})
	_, applied, err := h.Migrations(t.Context(), nil, MigrationsArgs{Directory: dir, Database: clean})
	if err != nil {
		t.Fatalf("clean Migrations() error = %v", err)
	}
	if applied.Tracker != trackerMigrate || applied.Dirty || strings.Join(applied.Pending, ",") != "2" {
		t.Fatalf("clean = %+v", applied)
	}

	dirty := writeSQLite(t, []string{
		`CREATE TABLE schema_migrations (version INTEGER, dirty INTEGER)`,
		`INSERT INTO schema_migrations (version, dirty) VALUES (1, 1)`,
	})
	_, failed, err := h.Migrations(t.Context(), nil, MigrationsArgs{Directory: dir, Database: dirty})
	if err != nil {
		t.Fatalf("dirty Migrations() error = %v", err)
	}
	if !failed.Dirty || strings.Join(failed.Pending, ",") != "1,2" {
		t.Fatalf("dirty = %+v", failed)
	}

	gooseDir := t.TempDir()
	writeFile(t, filepath.Join(gooseDir, "01_init.up.sql"), "-- init\n")
	writeFile(t, filepath.Join(gooseDir, "2_add_note.up.sql"), "-- note\n")
	gooseDB := writeSQLite(t, []string{
		`CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY, version_id INTEGER NOT NULL, is_applied INTEGER NOT NULL)`,
		`INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1), (1, 1)`,
	})
	_, goose, err := h.Migrations(t.Context(), nil, MigrationsArgs{Directory: gooseDir, Database: gooseDB})
	if err != nil {
		t.Fatalf("goose Migrations() error = %v", err)
	}
	if goose.Tracker != trackerGoose || strings.Join(goose.Pending, ",") != "2" {
		t.Fatalf("goose = %+v", goose)
	}

	both := writeSQLite(t, []string{
		`CREATE TABLE schema_migrations (version INTEGER, dirty INTEGER)`,
		`INSERT INTO schema_migrations (version, dirty) VALUES (2, 0)`,
		`CREATE TABLE goose_db_version (id INTEGER PRIMARY KEY, version_id INTEGER NOT NULL, is_applied INTEGER NOT NULL)`,
		`INSERT INTO goose_db_version (version_id, is_applied) VALUES (0, 1)`,
	})
	_, preferred, err := h.Migrations(t.Context(), nil, MigrationsArgs{Directory: dir, Database: both})
	if err != nil {
		t.Fatalf("both trackers error = %v", err)
	}
	if preferred.Tracker != trackerMigrate || preferred.PendingCount != 0 {
		t.Fatalf("both trackers = %+v", preferred)
	}
}

func TestMigrationsRejectsDuplicatesAndBadPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "1_init.up.sql"), "-- init\n")
	writeFile(t, filepath.Join(dir, "1_other.sql"), "-- other\n")
	_, _, err := (Handler{}).Migrations(t.Context(), nil, MigrationsArgs{Directory: dir})
	if err == nil || !strings.Contains(err.Error(), "version 1") {
		t.Fatalf("duplicate error = %v", err)
	}

	file := writeSQLite(t, []string{`CREATE TABLE users (id INTEGER PRIMARY KEY)`})
	_, _, err = (Handler{}).Schema(t.Context(), nil, SchemaArgs{Path: dir})
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("directory schema error = %v", err)
	}
	_, _, err = (Handler{}).Migrations(t.Context(), nil, MigrationsArgs{Directory: file})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("file directory error = %v", err)
	}
	_, _, err = (Handler{}).Schema(t.Context(), nil, SchemaArgs{Path: "a\nb"})
	if err == nil || !strings.Contains(err.Error(), "newline") {
		t.Fatalf("newline error = %v", err)
	}
}

func TestCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, _, err := (Handler{}).Schema(ctx, nil, SchemaArgs{Path: "missing.db"})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("Schema() error = %v", err)
	}
	_, _, err = (Handler{}).Migrations(ctx, nil, MigrationsArgs{Directory: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("Migrations() error = %v", err)
	}
}

func TestQueryOnlyRejectsWrites(t *testing.T) {
	t.Parallel()

	path := writeSQLite(t, []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`,
	})
	database, err := openReadOnly(t.Context(), path)
	if err != nil {
		t.Fatalf("openReadOnly() error = %v", err)
	}
	t.Cleanup(func() { closeQuiet(database) })

	_, err = database.ExecContext(t.Context(), `INSERT INTO users (email) VALUES ('ada@example.test')`)
	if err == nil {
		t.Fatal("INSERT succeeded on a read-only database")
	}
	_, err = database.ExecContext(t.Context(), `DELETE FROM users`)
	if err == nil {
		t.Fatal("DELETE succeeded on a read-only database")
	}
}

func writeSQLite(t *testing.T, statements []string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "app.db")
	database, err := sql.Open(sqliteDriver, "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer closeQuiet(database)
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
	return path
}

func writeMigrations(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "1_init.up.sql"), "-- init\n")
	writeFile(t, filepath.Join(dir, "1_init.down.sql"), "-- down\n")
	writeFile(t, filepath.Join(dir, "2_add_note.up.sql"), "-- note\n")
	writeFile(t, filepath.Join(dir, "README.md"), "ignore\n")
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFile(t, filepath.Join(dir, "nested", "3_hidden.up.sql"), "-- hidden\n")
	return dir
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func columnNamed(t *testing.T, columns []Column, name string) Column {
	t.Helper()
	for _, column := range columns {
		if column.Name == name {
			return column
		}
	}
	t.Fatalf("column %s not found in %+v", name, columns)
	return Column{}
}

func hasIndex(indexes []Index, name, column string) bool {
	for _, index := range indexes {
		if index.Name == name && len(index.Columns) == 1 && index.Columns[0] == column {
			return true
		}
	}
	return false
}
