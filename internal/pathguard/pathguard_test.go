package pathguard

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCheckUnrestrictedByDefault(t *testing.T) {
	t.Setenv(EnvAllowedRoots, "")
	if err := Check(t.TempDir()); err != nil {
		t.Fatalf("Check() = %v", err)
	}
}

func TestCheckRestricted(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	inside := filepath.Join(root, "sub", "app.log")
	if err := os.MkdirAll(filepath.Dir(inside), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(other, "secret")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAllowedRoots, root)

	if err := Check(inside); err != nil {
		t.Errorf("inside: %v", err)
	}
	if err := Check(root); err != nil {
		t.Errorf("root itself: %v", err)
	}
	if err := Check(outside); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("outside: %v", err)
	}
	if err := Check(filepath.Join(root, "..", filepath.Base(other), "secret")); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("dot-dot escape: %v", err)
	}
	if err := Check(filepath.Join(root, "missing")); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("missing path: %v", err)
	}
}

func TestCheckRefusesSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	root := t.TempDir()
	other := t.TempDir()
	target := filepath.Join(other, "secret")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvAllowedRoots, root)
	if err := Check(link); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("symlink escape: %v", err)
	}
}

func TestCheckSiblingPrefixIsNotInside(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "logs")
	sibling := filepath.Join(parent, "logs-private")
	for _, d := range []string{root, sibling} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(EnvAllowedRoots, root)
	if err := Check(sibling); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("sibling: %v", err)
	}
}

func TestRestrictedWithNoUsableRootRefusesEverything(t *testing.T) {
	t.Setenv(EnvAllowedRoots, filepath.Join(t.TempDir(), "does-not-exist"))
	if err := Check(t.TempDir()); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("Check() = %v", err)
	}
}
