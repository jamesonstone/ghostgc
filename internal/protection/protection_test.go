package protection

import (
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
)

func base() Input {
	return Input{
		Process: process.Process{
			PID:       5000,
			PPID:      1,
			UID:       501,
			StartTime: time.Now().Add(-time.Hour),
			ExecPath:  "/opt/vendor/agent-helper",
			Detailed:  true,
		},
		SelfPID:               99,
		SelfUID:               501,
		AttributionConfidence: 0.99,
	}
}

func hasRule(r Result, id string) bool {
	for _, rule := range r.Rules {
		if rule.ID == id {
			return true
		}
	}
	return false
}

func TestBaselineProcessIsUnprotected(t *testing.T) {
	// The baseline exists so that every other case in this file isolates one
	// protection. If the baseline were protected the tests would prove nothing.
	if got := Evaluate(base()); got.Protected {
		t.Fatalf("baseline should be unprotected, triggered: %v", got.Rules)
	}
}

func TestDaemonProtectsItself(t *testing.T) {
	in := base()
	in.Process.PID = in.SelfPID
	got := Evaluate(in)
	if !hasRule(got, "protected-self-v1") {
		t.Fatalf("the daemon must never be a candidate for termination: %v", got.Rules)
	}
}

func TestInitIsProtected(t *testing.T) {
	in := base()
	in.Process.PID = 1
	if !hasRule(Evaluate(in), "protected-init-v1") {
		t.Fatal("pid 1 must be protected")
	}
}

func TestOtherUsersProcessesAreProtected(t *testing.T) {
	in := base()
	in.Process.UID = 502
	if !hasRule(Evaluate(in), "protected-other-user-v1") {
		t.Fatal("a process owned by another user must be protected")
	}
}

func TestControllingTerminalIsProtected(t *testing.T) {
	in := base()
	in.Process.TTY = "/dev/ttys003"
	if !hasRule(Evaluate(in), "protected-controlling-terminal-v1") {
		t.Fatal("a process with a controlling terminal must be protected")
	}
}

func TestSessionLeaderIsProtected(t *testing.T) {
	in := base()
	in.Process.SID = in.Process.PID
	if !hasRule(Evaluate(in), "protected-session-leader-v1") {
		t.Fatal("a terminal session leader must be protected")
	}
}

func TestAgentRootIsProtected(t *testing.T) {
	in := base()
	in.IsAgentRoot = true
	if !hasRule(Evaluate(in), "protected-agent-root-v1") {
		t.Fatal("an agent session root must be protected")
	}
}

func TestActiveSessionProtectsItsProcesses(t *testing.T) {
	in := base()
	in.SessionActive = true
	if !hasRule(Evaluate(in), "protected-active-session-v1") {
		t.Fatal("a process belonging to an active session must be protected")
	}
}

func TestLiveDescendantsProtect(t *testing.T) {
	in := base()
	in.DescendantCount = 2
	if !hasRule(Evaluate(in), "protected-has-descendants-v1") {
		t.Fatal("a process with live descendants must be protected")
	}
}

// Unknown means protected. This is the single most important rule in the file.
func TestUncertainAttributionIsProtected(t *testing.T) {
	for _, confidence := range []float64{0, 0.5, 0.74, 0.94, 0.9499} {
		in := base()
		in.AttributionConfidence = confidence
		got := Evaluate(in)
		if !hasRule(got, "protected-uncertain-attribution-v1") {
			t.Fatalf("confidence %.4f must be protected as uncertain, triggered: %v", confidence, got.Rules)
		}
		if !got.Protected {
			t.Fatalf("confidence %.4f must be protected", confidence)
		}
	}
}

func TestUninspectedProcessIsProtected(t *testing.T) {
	in := base()
	in.Process.Detailed = false
	if !hasRule(Evaluate(in), "protected-not-inspected-v1") {
		t.Fatal("a process that was never inspected must be protected")
	}
}

// Section 14 of the specification lists executable names that are too broad to
// justify any automated action.
func TestBroadRuntimeNamesAreProtected(t *testing.T) {
	for _, name := range []string{"node", "python3", "go", "java", "git", "gh", "bash", "zsh", "ruby"} {
		in := base()
		in.Process.ExecPath = "/usr/bin/" + name
		got := Evaluate(in)
		if !hasRule(got, "protected-broad-runtime-v1") {
			t.Fatalf("executable %q must be protected as too broad, triggered: %v", name, got.Rules)
		}
	}
}

func TestInfrastructureClassesAreProtected(t *testing.T) {
	cases := map[string]string{
		"/usr/local/bin/gopls":        "protected-language-server-v1",
		"/usr/local/bin/dockerd":      "protected-container-runtime-v1",
		"/usr/local/bin/postgres":     "protected-database-v1",
		"/usr/local/bin/pytest":       "protected-build-or-test-v1",
		"/usr/local/bin/vite":         "protected-development-server-v1",
		"/Applications/Cursor/cursor": "protected-editor-v1",
	}
	for exec, rule := range cases {
		in := base()
		in.Process.ExecPath = exec
		if got := Evaluate(in); !hasRule(got, rule) {
			t.Fatalf("%s should trigger %s, triggered: %v", exec, rule, got.Rules)
		}
	}
}

func TestAdapterRulesApply(t *testing.T) {
	in := base()
	in.Process.ExecPath = "/opt/homebrew/bin/codex"
	in.AdapterRules = []adapters.ProtectionRule{{
		ID:        "codex-session-root-v1",
		Reason:    "an active Codex session root is the user's session",
		ExecNames: []string{"codex"},
	}}
	if !hasRule(Evaluate(in), "codex-session-root-v1") {
		t.Fatal("an adapter-contributed protection must be honoured")
	}
}

func TestAdapterPathSegmentRuleApplies(t *testing.T) {
	in := base()
	in.Process.ExecPath = "/Users/dev/lib/node_modules/@openai/codex/bin/native"
	in.AdapterRules = []adapters.ProtectionRule{{
		ID:          "codex-npm-launcher-v1",
		Reason:      "the launcher owns the session lifetime",
		PathSegment: "@openai/codex",
	}}
	if !hasRule(Evaluate(in), "codex-npm-launcher-v1") {
		t.Fatal("a path-segment protection must match on whole segments")
	}
}

func TestAdapterPathSegmentRuleDoesNotSubstringMatch(t *testing.T) {
	in := base()
	in.Process.ExecPath = "/Users/dev/notopenai/codexish/bin/tool"
	in.AdapterRules = []adapters.ProtectionRule{{
		ID:          "codex-npm-launcher-v1",
		PathSegment: "@openai/codex",
	}}
	if hasRule(Evaluate(in), "codex-npm-launcher-v1") {
		t.Fatal("path-segment matching must not degrade into substring matching")
	}
}

func TestEveryProtectionExplainsItself(t *testing.T) {
	in := base()
	in.Process.PID = 1
	in.Process.TTY = "/dev/ttys001"
	in.AttributionConfidence = 0.1
	got := Evaluate(in)
	if len(got.Rules) < 3 {
		t.Fatalf("expected several protections, got %v", got.Rules)
	}
	for _, rule := range got.Rules {
		if strings.TrimSpace(rule.Reason) == "" {
			t.Fatalf("protection %q has no reason; a conclusion without evidence is a bug", rule.ID)
		}
		if strings.TrimSpace(rule.ID) == "" {
			t.Fatal("a protection must be identifiable")
		}
	}
}
