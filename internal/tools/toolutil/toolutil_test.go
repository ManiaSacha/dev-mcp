package toolutil

import (
	"errors"
	"testing"
)

func TestErr(t *testing.T) {
	if Err("tool", nil) != nil {
		t.Error("nil error was wrapped")
	}
	base := errors.New("boom")
	got := Err("git_status", base)
	if got.Error() != "git_status: boom" || !errors.Is(got, base) {
		t.Errorf("got %v", got)
	}
}

func TestBounded(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		want    int
		wantErr string
	}{
		{"zero uses fallback", 0, 100, ""},
		{"in range", 50, 50, ""},
		{"at max", 1000, 1000, ""},
		{"negative", -1, 0, "lines must be positive"},
		{"too large", 1001, 0, "lines must be at most 1000"},
	}
	for _, tt := range tests {
		got, err := Bounded(tt.n, 100, 1000, "lines")
		if tt.wantErr == "" {
			if err != nil || got != tt.want {
				t.Errorf("%s: got %d, %v", tt.name, got, err)
			}
			continue
		}
		if err == nil || err.Error() != tt.wantErr {
			t.Errorf("%s: err = %v, want %q", tt.name, err, tt.wantErr)
		}
	}
}
