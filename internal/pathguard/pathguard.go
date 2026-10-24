// Package pathguard limits which local paths the file-reading tools may touch.
//
// The allowed roots come from the DEV_MCP_ALLOWED_ROOTS environment variable,
// a list separated by the OS path-list separator (":" on Unix, ";" on
// Windows). When the variable is unset or empty, every path is allowed.
// When it is set, a path must resolve, after symlinks, to a location inside
// one of the roots. A path that cannot be resolved is refused.
package pathguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvAllowedRoots names the environment variable that lists allowed roots.
const EnvAllowedRoots = "DEV_MCP_ALLOWED_ROOTS"

// ErrNotAllowed is returned when a path is outside every allowed root.
var ErrNotAllowed = errors.New("path is outside the allowed roots")

// Roots returns the configured roots, resolved to absolute paths without
// symlinks. restricted is false when no roots are configured. Entries that
// do not exist are skipped, so a restricted guard with no usable root
// refuses every path.
func Roots() (roots []string, restricted bool) {
	raw := strings.TrimSpace(os.Getenv(EnvAllowedRoots))
	if raw == "" {
		return nil, false
	}
	for _, entry := range filepath.SplitList(raw) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		abs, err := filepath.Abs(entry)
		if err != nil {
			continue
		}
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			continue
		}
		roots = append(roots, resolved)
	}
	return roots, true
}

// Check returns nil when path may be read. It resolves path against the
// working directory and follows symlinks before comparing it with the roots.
func Check(path string) error {
	roots, restricted := Roots()
	if !restricted {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return ErrNotAllowed
	}
	for _, root := range roots {
		if within(root, resolved) {
			return nil
		}
	}
	return ErrNotAllowed
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
