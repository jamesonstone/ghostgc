package process

import (
	"slices"
	"strings"
	"testing"
)

func TestRedactArgsRemovesCredentials(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "inline flag value",
			in:   []string{"tool", "--api-key=sk-live-abcdefghijklmnop", "--verbose"},
			want: []string{"tool", "--api-key=[redacted]", "--verbose"},
		},
		{
			name: "separated flag value",
			in:   []string{"tool", "--token", "abcd1234", "--port", "8080"},
			want: []string{"tool", "--token", "[redacted]", "--port", "8080"},
		},
		{
			name: "sensitive flag with no value does not eat the next flag",
			in:   []string{"tool", "--password", "--dry-run"},
			want: []string{"tool", "--password", "--dry-run"},
		},
		{
			name: "inline environment assignment",
			in:   []string{"env", "OPENAI_API_KEY=sk-proj-secret", "PATH=/usr/bin"},
			want: []string{"env", "OPENAI_API_KEY=[redacted]", "PATH=/usr/bin"},
		},
		{
			name: "bare credential recognised by prefix",
			in:   []string{"curl", "-H", "ghp_0123456789abcdefghij"},
			want: []string{"curl", "-H", "[redacted]"},
		},
		{
			name: "jwt",
			in:   []string{"tool", "eyJhbGciOiJIUzI1NiJ9.payload.signature"},
			want: []string{"tool", "[redacted]"},
		},
		{
			name: "url with embedded password",
			in:   []string{"psql", "postgres://admin:hunter2@db.internal:5432/app"},
			want: []string{"psql", "postgres://admin:%5Bredacted%5D@db.internal:5432/app"},
		},
		{
			name: "presigned url query parameters",
			in:   []string{"curl", "https://example.com/o?X-Amz-Signature=deadbeef&file=notes.txt"},
			want: []string{"curl", "https://example.com/o?X-Amz-Signature=%5Bredacted%5D&file=notes.txt"},
		},
		{
			name: "ordinary arguments are untouched",
			in:   []string{"go", "test", "./...", "-run", "TestRedact", "--design-file", "layout.json"},
			want: []string{"go", "test", "./...", "-run", "TestRedact", "--design-file", "layout.json"},
		},
		{
			name: "codex invocation survives intact",
			in:   []string{"codex", "-c", "features.code_mode_host=true", "app-server", "--analytics-default-enabled"},
			want: []string{"codex", "-c", "features.code_mode_host=true", "app-server", "--analytics-default-enabled"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactArgs(tt.in)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("RedactArgs(%q)\n got %q\nwant %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRedactArgsNeverLeaksKnownSecretShapes(t *testing.T) {
	secrets := []string{
		"sk-proj-0123456789",
		"ghp_abcdefghijklmnopqrst",
		"github_pat_11ABCDEFG",
		"xoxb-1234-5678-abcdef",
		"AKIAIOSFODNN7EXAMPLE",
		"AIzaSyD-EXAMPLE-KEY",
		"glpat-abcdefghij",
		"eyJhbGciOi.body.sig",
	}
	for _, secret := range secrets {
		out := strings.Join(RedactArgs([]string{"tool", secret}), " ")
		if strings.Contains(out, secret) {
			t.Fatalf("secret %q survived redaction: %q", secret, out)
		}
	}
}

func TestRedactArgsNilStaysNil(t *testing.T) {
	if got := RedactArgs(nil); got != nil {
		t.Fatalf("RedactArgs(nil) = %v, want nil", got)
	}
}

func TestSensitiveNameDoesNotOverMatchSessionIdentifiers(t *testing.T) {
	// CODEX_SESSION_ID is attribution evidence, not a credential. Redacting it
	// would destroy the strongest ownership signal the adapter has.
	if SensitiveName("CODEX_SESSION_ID") {
		t.Fatal("CODEX_SESSION_ID must not be treated as sensitive")
	}
	if !SensitiveName("AWS_SESSION_TOKEN") {
		t.Fatal("AWS_SESSION_TOKEN must be treated as sensitive")
	}
}

func TestRedactEnvKeepsOnlyAllowlistedKeys(t *testing.T) {
	env := map[string]string{
		"CODEX_HOME":       "/Users/dev/.codex",
		"CODEX_SESSION_ID": "abc-123",
		"OPENAI_API_KEY":   "sk-live-secret",
		"AWS_SECRET_KEY":   "wJalrXUtn",
		"PATH":             "/usr/bin",
	}
	policy := EnvPolicy{
		Allow:        []string{"CODEX_HOME", "CODEX_SESSION_ID", "AWS_SECRET_KEY"},
		RedactValues: true,
	}
	got := RedactEnv(env, policy)

	if _, ok := got["OPENAI_API_KEY"]; ok {
		t.Fatal("a variable outside the allowlist must be dropped entirely")
	}
	if _, ok := got["PATH"]; ok {
		t.Fatal("a variable outside the allowlist must be dropped entirely")
	}
	if got["CODEX_HOME"] != "/Users/dev/.codex" {
		t.Fatalf("CODEX_HOME = %q, want the value retained", got["CODEX_HOME"])
	}
	if got["CODEX_SESSION_ID"] != "abc-123" {
		t.Fatalf("CODEX_SESSION_ID = %q, want the value retained", got["CODEX_SESSION_ID"])
	}
	if got["AWS_SECRET_KEY"] != Redacted {
		t.Fatalf("AWS_SECRET_KEY = %q, want it redacted even though it is allowlisted", got["AWS_SECRET_KEY"])
	}
}

func TestRedactEnvWithoutAllowlistReturnsNothing(t *testing.T) {
	env := map[string]string{"CODEX_HOME": "/x"}
	if got := RedactEnv(env, EnvPolicy{}); got != nil {
		t.Fatalf("RedactEnv with an empty allowlist = %v, want nil", got)
	}
}
