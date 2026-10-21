package git

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	maxBranchLen = 250
	maxRevision  = 250
	maxPaths     = 2000
	maxMessage   = 1 << 20
)

func validateBranchName(name string) error {
	if name == "" {
		return errors.New("branch name is empty")
	}
	if len(name) > maxBranchLen {
		return errors.New("branch name is too long")
	}
	if strings.ContainsRune(name, 0) || strings.ContainsAny(name, "\r\n") {
		return errors.New("branch name contains a newline")
	}
	if strings.HasPrefix(name, "-") {
		return errors.New("branch name must not start with '-'")
	}
	return nil
}

func validateStartPoint(ref string) error {
	if ref == "" {
		return nil
	}
	if len(ref) > maxRevision {
		return errors.New("start point is too long")
	}
	if strings.ContainsRune(ref, 0) || strings.ContainsAny(ref, "\r\n") {
		return errors.New("start point contains a newline")
	}
	if strings.HasPrefix(ref, "-") {
		return errors.New("start point must not start with '-'")
	}
	return nil
}

// validateRelPath returns a repository-relative slash path.
// Absolute paths and paths that climb out of the work tree are rejected.
func validateRelPath(p string) (string, error) {
	if p == "" || strings.ContainsRune(p, 0) || strings.ContainsAny(p, "\r\n") {
		return "", errors.New("path is empty or contains a newline")
	}
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("path %q must be relative to the repository", p)
	}
	clean := filepath.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the repository", p)
	}
	return filepath.ToSlash(clean), nil
}

func normalizeMessage(message string) (string, error) {
	if strings.ContainsRune(message, 0) {
		return "", errors.New("commit message contains NUL")
	}
	if len(message) > maxMessage {
		return "", errors.New("commit message is too long")
	}
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return "", errors.New("commit message is empty")
	}
	return trimmed + "\n", nil
}

func stageSelection(args StageAndCommitArgs) (paths []string, all bool, err error) {
	allSet := args.All != nil && *args.All
	if len(args.Paths) > 0 && allSet {
		return nil, false, errors.New("set paths or all, not both")
	}
	if len(args.Paths) == 0 && !allSet {
		return nil, false, errors.New("provide paths or set all to true")
	}
	if allSet {
		return nil, true, nil
	}
	if len(args.Paths) > maxPaths {
		return nil, false, fmt.Errorf("too many paths (%d)", len(args.Paths))
	}

	seen := make(map[string]struct{}, len(args.Paths))
	out := make([]string, 0, len(args.Paths))
	for _, p := range args.Paths {
		clean, err := validateRelPath(p)
		if err != nil {
			return nil, false, err
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out, false, nil
}

func encodePathspecs(paths []string) []byte {
	var buf []byte
	for _, p := range paths {
		buf = append(buf, p...)
		buf = append(buf, 0)
	}
	return buf
}
