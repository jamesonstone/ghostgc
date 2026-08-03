package codex

import (
	"context"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
)

var start = time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)

func mk(pid, ppid int, exec string, args []string, env map[string]string) process.Process {
	return process.Process{
		PID:       pid,
		PPID:      ppid,
		StartTime: start.Add(time.Duration(pid) * time.Millisecond),
		ExecPath:  exec,
		Args:      args,
		Env:       env,
		Detailed:  true,
		CWD:       "/tmp",
	}
}

func graphOf(procs ...process.Process) adapters.Graph {
	snap := process.NewSnapshot(start.Add(time.Hour), procs, len(procs))
	return adapters.Graph{Snapshot: snap, Tree: process.BuildTree(snap), Roots: map[int]adapters.AgentRoot{}}
}

func TestEnvironmentSessionIdentifierIsAuthoritative(t *testing.T) {
	p := mk(900, 1, "/opt/homebrew/bin/codex", []string{"codex"},
		map[string]string{"CODEX_SESSION_ID": "8f2a-real-session"})
	s := InspectProcess(p)

	if s.NativeSessionID != "8f2a-real-session" {
		t.Fatalf("NativeSessionID = %q, want the environment value", s.NativeSessionID)
	}
	if got := s.Confidence(); got < adapters.ConfidencePolicyEligible {
		t.Fatalf("confidence %.2f, want at least %.2f when the agent names its own session",
			got, adapters.ConfidencePolicyEligible)
	}
}

func TestConfidenceNeverReachesCertainty(t *testing.T) {
	p := mk(901, 1,
		"/Users/dev/.nvm/versions/node/v22.19.0/lib/node_modules/@openai/codex/vendor/aarch64-apple-darwin/bin/codex",
		[]string{"codex"},
		map[string]string{
			"CODEX_SESSION_ID":           "s",
			"CODEX_HOME":                 "/Users/dev/.codex",
			"CODEX_MANAGED_BY_NPM":       "1",
			"CODEX_MANAGED_PACKAGE_ROOT": "/x",
		})
	if got := InspectProcess(p).Confidence(); got >= 1.0 {
		t.Fatalf("confidence = %.4f; heuristic agreement must never be reported as certainty", got)
	}
}

func TestDetectRootProcessesKeepsOnlyTheTopmostCandidate(t *testing.T) {
	launcher := mk(4100, 1, "/Users/dev/.nvm/versions/node/v22.19.0/bin/node",
		[]string{"node", "/Users/dev/.nvm/versions/node/v22.19.0/lib/node_modules/@openai/codex/bin/codex.js"}, nil)
	native := mk(4101, 4100,
		"/Users/dev/.nvm/versions/node/v22.19.0/lib/node_modules/@openai/codex-darwin-arm64/vendor/aarch64-apple-darwin/bin/codex",
		[]string{"codex"}, map[string]string{"CODEX_MANAGED_BY_NPM": "1"})
	init := mk(1, 0, "/sbin/launchd", []string{"launchd"}, nil)

	a := New(nil)
	roots, err := a.DetectRootProcesses(context.Background(), graphOf(init, launcher, native))
	if err != nil {
		t.Fatalf("DetectRootProcesses: %v", err)
	}
	if len(roots) != 1 {
		t.Fatalf("got %d roots, want the launcher only: %+v", len(roots), roots)
	}
	if roots[0].Process.PID != 4100 {
		t.Fatalf("root pid = %d, want the launcher 4100", roots[0].Process.PID)
	}
}

func TestUninspectedProcessIsNeverDetected(t *testing.T) {
	p := mk(4200, 1, "/opt/homebrew/bin/codex", []string{"codex"}, nil)
	p.Detailed = false
	p.ExecPath = ""
	p.Args = nil

	a := New(nil)
	roots, err := a.DetectRootProcesses(context.Background(), graphOf(p))
	if err != nil {
		t.Fatalf("DetectRootProcesses: %v", err)
	}
	if len(roots) != 0 {
		t.Fatalf("got %d roots from an uninspected process, want none", len(roots))
	}
}

func TestAttributeProcessFollowsAncestryOnly(t *testing.T) {
	root := mk(4100, 1, "/opt/homebrew/bin/codex", []string{"codex"}, nil)
	child := mk(4102, 4100, "/bin/sh", []string{"sh", "-c", "npm test"}, nil)
	stranger := mk(4103, 1, "/bin/sh", []string{"sh", "-c", "sleep 1000"}, nil)

	g := graphOf(mk(1, 0, "/sbin/launchd", nil, nil), root, child, stranger)
	a := New(nil)
	roots, _ := a.DetectRootProcesses(context.Background(), g)
	if len(roots) != 1 {
		t.Fatalf("expected one root, got %d", len(roots))
	}
	g.Roots[roots[0].Process.PID] = roots[0]

	rootAttr := a.AttributeProcess(context.Background(), root, g)
	if rootAttr.Relation != adapters.RelationRoot {
		t.Fatalf("root relation = %q, want root", rootAttr.Relation)
	}

	childAttr := a.AttributeProcess(context.Background(), child, g)
	if childAttr.Relation != adapters.RelationDescendant {
		t.Fatalf("child relation = %q, want descendant", childAttr.Relation)
	}
	if childAttr.SessionID != rootAttr.SessionID {
		t.Fatalf("child session %q != root session %q", childAttr.SessionID, rootAttr.SessionID)
	}
	if childAttr.Confidence > rootAttr.Confidence {
		t.Fatalf("a descendant (%.2f) must never be more confidently attributed than its root (%.2f)",
			childAttr.Confidence, rootAttr.Confidence)
	}

	if got := a.AttributeProcess(context.Background(), stranger, g); got.SessionID != "" {
		t.Fatalf("an unrelated shell was attributed to session %q", got.SessionID)
	}
}

func TestDeriveSessionIDIsStableAndStartTimeSensitive(t *testing.T) {
	p := mk(4100, 1, "/opt/homebrew/bin/codex", []string{"codex"}, nil)
	rootA := adapters.AgentRoot{Process: p, Key: p.Key()}

	first, second := DeriveSessionID(rootA), DeriveSessionID(rootA)
	if first != second {
		t.Fatalf("session id must be stable for the same root process: %q then %q", first, second)
	}

	// Same PID, later start time: a different process, so a different session.
	q := p
	q.StartTime = p.StartTime.Add(time.Hour)
	rootB := adapters.AgentRoot{Process: q, Key: q.Key()}
	if DeriveSessionID(rootA) == DeriveSessionID(rootB) {
		t.Fatal("a recycled pid must not inherit the previous session's identifier")
	}

	native := adapters.AgentRoot{Process: p, Key: p.Key(),
		Metadata: adapters.SessionMetadata{SessionID: "abc/../def 123"}}
	if got := DeriveSessionID(native); got != "abc----def-123" {
		t.Fatalf("native session id sanitisation = %q", got)
	}
}

func TestEnvKeysAreNotSharedWithCallers(t *testing.T) {
	a := New(nil)
	keys := a.EnvKeys()
	if len(keys) == 0 {
		t.Fatal("the adapter must declare the environment variables it needs")
	}
	keys[0] = "MUTATED"
	if a.EnvKeys()[0] == "MUTATED" {
		t.Fatal("EnvKeys returned an aliased slice")
	}
}
