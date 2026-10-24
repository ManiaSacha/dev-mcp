// Package db inspects a local SQLite database and migration files.
//
// The tools are read-only. They do not apply migrations, do not accept SQL
// from the caller, and do not invoke a shell. The database is opened with
// mode=ro and PRAGMA query_only.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // register the CGO-free SQLite driver

	"github.com/ManiaSacha/dev-mcp/internal/pathguard"
)

const sqliteDriver = "sqlite"

func openReadOnly(ctx context.Context, path string) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateFilePath(path); err != nil {
		return nil, err
	}
	if err := pathguard.Check(path); err != nil {
		return nil, fmt.Errorf("path: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("path: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path %s is not a regular file", path)
	}

	dsnPath := filepath.ToSlash(abs)
	if !strings.HasPrefix(dsnPath, "/") {
		dsnPath = "/" + dsnPath
	}
	dsn := (&url.URL{Scheme: "file", Path: dsnPath, RawQuery: "mode=ro"}).String()
	database, err := sql.Open(sqliteDriver, dsn)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("sqlite: %w", err)
	}
	if _, err := database.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("sqlite: %w", err)
	}
	return database, nil
}

func closeQuiet(c io.Closer) {
	_ = c.Close()
}

func validateFilePath(path string) error {
	if path == "" || strings.ContainsRune(path, 0) || strings.ContainsAny(path, "\r\n") {
		return errors.New("path is empty or contains a newline")
	}
	return nil
}

func validateDirPath(path string) error {
	if err := validateFilePath(path); err != nil {
		return err
	}
	if err := pathguard.Check(path); err != nil {
		return fmt.Errorf("path: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path %s is not a directory", path)
	}
	return nil
}
