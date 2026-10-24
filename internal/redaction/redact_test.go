package redaction

import "testing"

func TestRedact(t *testing.T) {
	t.Parallel()

	// Fixtures are synthetic and low-entropy. They match the redaction
	// patterns without being usable credentials.
	jwt := "eyJaaaaaaaaaa.bbbbbbbbbbbb.cccccccccccc"
	pem := "-----BEGIN PRIVATE KEY-----\nabc123\n-----END PRIVATE KEY-----"

	tests := []struct {
		name       string
		in         string
		want       string
		redactions int
	}{
		{
			name:       "plain log",
			in:         "level=info msg=listening addr=:8080",
			want:       "level=info msg=listening addr=:8080",
			redactions: 0,
		},
		{
			name:       "password assignment",
			in:         "login failed password=hunter2 user=ada",
			want:       "login failed password=" + mask + " user=ada",
			redactions: 1,
		},
		{
			name:       "json secret",
			in:         `{"api_key": "example-value"}`,
			want:       `{"api_key": ` + mask + `}`,
			redactions: 1,
		},
		{
			name:       "url userinfo",
			in:         "postgres://app:s3cret@db.internal:5432/app",
			want:       "postgres://app:" + mask + "@db.internal:5432/app",
			redactions: 1,
		},
		{
			name:       "bearer and aws",
			in:         "Authorization: Bearer exampleexample AKIAEXAMPLEEXAMPLEAA",
			want:       "Authorization: Bearer " + mask + " " + mask,
			redactions: 2,
		},
		{
			name:       "github token",
			in:         "ghp_exampleexampleexample",
			want:       mask,
			redactions: 1,
		},
		{
			name:       "slack token",
			in:         "xoxb-exampleexample",
			want:       mask,
			redactions: 1,
		},
		{
			name:       "jwt",
			in:         "session=" + jwt,
			want:       "session=" + mask,
			redactions: 1,
		},
		{
			name:       "private key block",
			in:         "key=\n" + pem + "\n",
			want:       "key=\n" + mask + "\n",
			redactions: 1,
		},
		{
			name:       "empty",
			in:         "",
			want:       "",
			redactions: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, n := Redact(tt.in)
			if got != tt.want || n != tt.redactions {
				t.Fatalf("Redact() = %q, %d; want %q, %d", got, n, tt.want, tt.redactions)
			}
		})
	}
}

func TestRedactLeavesUnrelatedWords(t *testing.T) {
	t.Parallel()
	in := "secretary=ada tokenized=false passwordless login"
	got, n := Redact(in)
	if got != in || n != 0 {
		t.Fatalf("Redact() = %q, %d", got, n)
	}
}

func TestRedactLinesMasksMultiLinePrivateKey(t *testing.T) {
	in := []string{
		"before",
		"key: -----BEGIN RSA PRIVATE KEY-----",
		"MIIEpAIBAAKCAQEA1234567890abcdef",
		"-----END RSA PRIVATE KEY-----",
		"after password=hunter2",
	}
	got, n := RedactLines(in)
	if len(got) != len(in) {
		t.Fatalf("line count = %d, want %d", len(got), len(in))
	}
	want := []string{"before", "key: [REDACTED]", "[REDACTED]", "[REDACTED]", "after password=[REDACTED]"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
	if n != 2 {
		t.Errorf("redactions = %d, want 2", n)
	}
}

func TestRedactLinesSingleLineKeyAndUnterminated(t *testing.T) {
	got, _ := RedactLines([]string{"-----BEGIN PRIVATE KEY-----abc-----END PRIVATE KEY-----", "next"})
	if got[0] != "[REDACTED]" || got[1] != "next" {
		t.Errorf("got %q", got)
	}
	got, _ = RedactLines([]string{"-----BEGIN PRIVATE KEY-----", "body", "more"})
	if got[1] != "[REDACTED]" || got[2] != "[REDACTED]" {
		t.Errorf("unterminated key leaked: %q", got)
	}
}
