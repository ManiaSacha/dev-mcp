package git

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want StatusResult
	}{
		{
			name: "clean",
			raw:  "## main\x00",
			want: StatusResult{Branch: "main", Clean: true, Entries: []StatusEntry{}},
		},
		{
			name: "no commits",
			raw:  "## No commits yet on main\x00",
			want: StatusResult{Branch: "main", Clean: true, Entries: []StatusEntry{}},
		},
		{
			name: "detached",
			raw:  "## HEAD (no branch)\x00 M file.txt\x00",
			want: StatusResult{
				Branch:   "HEAD",
				Detached: true,
				Clean:    false,
				Entries: []StatusEntry{{
					Path:     "file.txt",
					Index:    "unmodified",
					Worktree: "modified",
				}},
			},
		},
		{
			name: "ahead behind rename untracked",
			raw:  "## main...origin/main [ahead 1, behind 2]\x00 M file.txt\x00R  renamed.txt\x00keep.txt\x00?? untracked.txt\x00",
			want: StatusResult{
				Branch:   "main",
				Upstream: "origin/main",
				Ahead:    1,
				Behind:   2,
				Entries: []StatusEntry{
					{Path: "file.txt", Index: "unmodified", Worktree: "modified"},
					{Path: "renamed.txt", OriginalPath: "keep.txt", Index: "renamed", Worktree: "unmodified"},
					{Path: "untracked.txt", Index: "untracked", Worktree: "untracked"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseStatus([]byte(tt.raw))
			if err != nil {
				t.Fatalf("parseStatus() error = %v", err)
			}
			if got.Branch != tt.want.Branch || got.Detached != tt.want.Detached || got.Upstream != tt.want.Upstream ||
				got.Ahead != tt.want.Ahead || got.Behind != tt.want.Behind || got.Clean != tt.want.Clean {
				t.Fatalf("header = %+v, want %+v", got, tt.want)
			}
			if len(got.Entries) != len(tt.want.Entries) {
				t.Fatalf("entries = %+v, want %+v", got.Entries, tt.want.Entries)
			}
			for i := range tt.want.Entries {
				if got.Entries[i] != tt.want.Entries[i] {
					t.Fatalf("entry %d = %+v, want %+v", i, got.Entries[i], tt.want.Entries[i])
				}
			}
		})
	}
}

func TestParseNumstat(t *testing.T) {
	t.Parallel()

	out := []byte("1\t2\tfile.txt\x00-\t-\tbin.dat\x000\t0\t\x00keep.txt\x00renamed.txt\x00")
	got, err := parseNumstat(out)
	if err != nil {
		t.Fatalf("parseNumstat() error = %v", err)
	}
	if got.FileCount != 3 || got.Additions != 1 || got.Deletions != 2 {
		t.Fatalf("totals = %+v", got)
	}
	want := []DiffFile{
		{Path: "file.txt", Additions: 1, Deletions: 2},
		{Path: "bin.dat", Binary: true},
		{Path: "renamed.txt", OriginalPath: "keep.txt"},
	}
	if len(got.Files) != len(want) {
		t.Fatalf("files = %+v", got.Files)
	}
	for i := range want {
		if got.Files[i] != want[i] {
			t.Fatalf("file %d = %+v, want %+v", i, got.Files[i], want[i])
		}
	}
}

func TestParseNumstatEmpty(t *testing.T) {
	t.Parallel()
	got, err := parseNumstat(nil)
	if err != nil {
		t.Fatalf("parseNumstat() error = %v", err)
	}
	if got.FileCount != 0 || got.Files == nil {
		t.Fatalf("empty summary = %+v", got)
	}
}

func TestValidateInputs(t *testing.T) {
	t.Parallel()

	if _, err := validateRelPath("../outside.txt"); err == nil {
		t.Fatal("parent path was accepted")
	}
	if _, err := validateRelPath("/etc/passwd"); err == nil {
		t.Fatal("absolute path was accepted")
	}
	if _, err := validateRelPath("."); err == nil {
		t.Fatal("dot path was accepted")
	}
	got, err := validateRelPath(`dir/file.txt`)
	if err != nil || got != "dir/file.txt" {
		t.Fatalf("relative path = %q, %v", got, err)
	}

	if err := validateBranchName("-c"); err == nil {
		t.Fatal("option-like branch name was accepted")
	}
	if err := validateBranchName("feature/step-2"); err != nil {
		t.Fatalf("valid branch rejected: %v", err)
	}
	if err := validateStartPoint("--upload-pack=evil"); err == nil {
		t.Fatal("option-like start point was accepted")
	}
	if _, err := normalizeMessage("   "); err == nil {
		t.Fatal("blank message was accepted")
	}
	msg, err := normalizeMessage("subject\n\nbody")
	if err != nil || !strings.HasSuffix(msg, "\n") || strings.Contains(msg, "\x00") {
		t.Fatalf("message = %q, %v", msg, err)
	}

	_, _, err = stageSelection(StageAndCommitArgs{})
	if err == nil {
		t.Fatal("missing paths and all was accepted")
	}
	all := true
	_, _, err = stageSelection(StageAndCommitArgs{Paths: []string{"a.txt"}, All: &all})
	if err == nil {
		t.Fatal("paths combined with all was accepted")
	}
}

func TestSplitNULDropsTrailer(t *testing.T) {
	t.Parallel()
	got := splitNUL([]byte("a\x00b\x00"))
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("splitNUL = %#v", got)
	}
	if !bytes.Equal(encodePathspecs([]string{"a", "b"}), []byte("a\x00b\x00")) {
		t.Fatal("pathspec encoding mismatch")
	}
}
