// Package toolutil holds small helpers shared by the tool packages.
package toolutil

import "fmt"

// Err prefixes err with the tool name, so a client can tell which tool failed.
// It returns nil when err is nil.
func Err(tool string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", tool, err)
}

// Bounded validates a caller-supplied count. Zero selects fallback. A
// negative value, or one above max, is an error that names the argument.
func Bounded(n, fallback, max int, name string) (int, error) {
	if n == 0 {
		return fallback, nil
	}
	if n < 0 {
		return 0, fmt.Errorf("%s must be positive", name)
	}
	if n > max {
		return 0, fmt.Errorf("%s must be at most %d", name, max)
	}
	return n, nil
}
