package version

import "testing"

func TestFlag(t *testing.T) {
	t.Parallel()

	if !Flag([]string{"--version"}) || !Flag([]string{"-version"}) {
		t.Fatal("version flags were not recognized")
	}
	if Flag(nil) || Flag([]string{"--help"}) || Flag([]string{"version"}) {
		t.Fatal("non-version args were treated as a version request")
	}
	if Version == "" || Name != "dev-mcp" {
		t.Fatalf("identity = %s %s", Name, Version)
	}
}
