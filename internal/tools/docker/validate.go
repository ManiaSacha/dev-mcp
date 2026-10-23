package docker

import (
	"errors"
	"regexp"
	"strings"
)

// containerRef matches a container name or id. It rejects option-like
// values, paths, and shell metacharacters.
var containerRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

var containerStates = map[string]struct{}{
	"created":    {},
	"restarting": {},
	"running":    {},
	"paused":     {},
	"exited":     {},
	"dead":       {},
}

func validateContainer(ref string) error {
	if ref == "" {
		return errors.New("container is empty")
	}
	if strings.ContainsRune(ref, 0) || strings.ContainsAny(ref, "\r\n") {
		return errors.New("container contains a newline")
	}
	if strings.HasPrefix(ref, "-") {
		return errors.New("container must not start with '-'")
	}
	if !containerRef.MatchString(ref) {
		return errors.New("container has invalid characters")
	}
	return nil
}

func validateState(state string) error {
	if state == "" {
		return nil
	}
	if _, ok := containerStates[state]; !ok {
		return errors.New("state must be created, restarting, running, paused, exited, or dead")
	}
	return nil
}

func validateNameFilter(name string) error {
	if name == "" {
		return nil
	}
	if strings.ContainsRune(name, 0) || strings.ContainsAny(name, "\r\n") {
		return errors.New("name contains a newline")
	}
	if len(name) > 128 {
		return errors.New("name is too long")
	}
	return nil
}
