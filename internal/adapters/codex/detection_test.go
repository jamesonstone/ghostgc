package codex

import (
	"context"
	"testing"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
)

// Command lines recorded from a real machine. The negative cases are the point:
// each one contains "codex" somewhere and none of them is a Codex CLI session.
func TestInspectProcessAgainstRealWorldCommandLines(t *testing.T) {
	tests := []struct {
		name       string
		proc       process.Process
		wantDetect bool
	}{
		{
			name: "vscode extension native binary",
			proc: mk(37859, 37850,
				"/Users/dev/.vscode-insiders/extensions/openai.chatgpt-26.727.40816-darwin-arm64/bin/macos-aarch64/codex",
				[]string{"codex", "-c", "features.code_mode_host=true", "app-server", "--analytics-default-enabled"}, nil),
			wantDetect: true,
		},
		{
			name: "npm launcher under node",
			proc: mk(4100, 4000, "/Users/dev/.nvm/versions/node/v22.19.0/bin/node",
				[]string{"node", "/Users/dev/.nvm/versions/node/v22.19.0/lib/node_modules/@openai/codex/bin/codex.js", "exec"}, nil),
			wantDetect: true,
		},
		{
			name: "native binary launched by the npm wrapper",
			proc: mk(4101, 4100,
				"/Users/dev/.nvm/versions/node/v22.19.0/lib/node_modules/@openai/codex-darwin-arm64/vendor/aarch64-apple-darwin/bin/codex",
				[]string{"codex", "exec"},
				map[string]string{
					"CODEX_MANAGED_BY_NPM":       "1",
					"CODEX_MANAGED_PACKAGE_ROOT": "/Users/dev/.nvm/versions/node/v22.19.0/lib/node_modules/@openai/codex",
				}),
			wantDetect: true,
		},
		{
			name: "chatgpt desktop electron helper named Codex",
			proc: mk(29739, 79764,
				"/Applications/ChatGPT.app/Contents/Frameworks/Codex Framework.framework/Versions/150.0.7871.182/Helpers/Codex (Renderer).app/Contents/MacOS/Codex (Renderer)",
				[]string{"Codex (Renderer)", "--type=renderer", "--user-data-dir=/Users/dev/Library/Application Support/Codex"}, nil),
			wantDetect: false,
		},
		{
			name: "chatgpt desktop code mode host",
			proc: mk(7893, 81190, "/Applications/ChatGPT.app/Contents/Resources/codex-code-mode-host",
				[]string{"codex-code-mode-host"}, nil),
			wantDetect: false,
		},
		{
			name: "unrelated helper passed --agent codexCLI",
			proc: mk(17836, 81190,
				"/Applications/Pencil.app/Contents/Resources/app.asar.unpacked/out/mcp-server-darwin-arm64",
				[]string{"mcp-server-darwin-arm64", "--app", "desktop", "--agent", "codexCLI"}, nil),
			wantDetect: false,
		},
		{
			name:       "plain node process",
			proc:       mk(5000, 1, "/usr/local/bin/node", []string{"node", "server.js"}, nil),
			wantDetect: false,
		},
		{
			name: "node running an unrelated script called codex.js in a user project",
			proc: mk(5001, 1, "/usr/local/bin/node",
				[]string{"node", "/Users/dev/scratch/codex.js"}, nil),
			// One weak signal only; below the root-detection bar.
			wantDetect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := InspectProcess(tt.proc)
			detected := s.IdentityConfidence() >= adapters.ConfidenceRootDetection
			if detected != tt.wantDetect {
				t.Fatalf("identity confidence %.2f -> detected=%t, want %t; evidence=%v conflicts=%v",
					s.IdentityConfidence(), detected, tt.wantDetect, s.Evidence(), s.Conflicts)
			}
		})
	}
}

// An environment variable is inherited by every descendant, so a long-lived
// process launched from a Codex session months ago still carries the agent's
// session identifier. It is a member of that session's lineage; it is not the
// Codex program, and it must never be promoted to a session root.
//
// This case is taken from a real machine: a service manager, started from a
// Codex session days earlier and since reparented to init, was being reported
// as an active Codex session at 0.90 confidence.
func TestInheritedEnvironmentDoesNotCreateASessionRoot(t *testing.T) {
	p := mk(94209, 1, "/opt/homebrew/Cellar/process-compose/1.120.0/bin/process-compose",
		[]string{"/opt/homebrew/bin/process-compose", "--config", "/Users/dev/.config/svc/process-compose.yaml", "up"},
		map[string]string{"CODEX_THREAD_ID": "019fae08-329a-7bb1-946c-a90e9908c2ae"})

	s := InspectProcess(p)
	if s.IdentityConfidence() >= adapters.ConfidenceRootDetection {
		t.Fatalf("identity confidence %.2f: an inherited environment variable must not make a process a session root",
			s.IdentityConfidence())
	}
	if len(s.Membership) == 0 {
		t.Fatal("the inherited identifier is still worth recording as membership evidence")
	}

	a := New(nil)
	roots, err := a.DetectRootProcesses(context.Background(), graphOf(mk(1, 0, "/sbin/launchd", nil, nil), p))
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 0 {
		t.Fatalf("got %d roots, want none: %+v", len(roots), roots)
	}
}

func TestIdentityEvidenceComesOnlyFromTheExecutableAndArguments(t *testing.T) {
	p := mk(500, 1, "/usr/bin/make", []string{"make", "build"}, map[string]string{
		"CODEX_HOME":                 "/Users/dev/.codex",
		"CODEX_SESSION_ID":           "s-1",
		"CODEX_MANAGED_BY_NPM":       "1",
		"CODEX_MANAGED_PACKAGE_ROOT": "/x",
	})
	s := InspectProcess(p)
	if len(s.Identity) != 0 {
		t.Fatalf("environment variables produced identity evidence: %+v", s.Identity)
	}
	if s.IdentityConfidence() != 0 {
		t.Fatalf("identity confidence = %.2f, want 0", s.IdentityConfidence())
	}
	if s.Confidence() == 0 {
		t.Fatal("membership evidence should still be reflected in the overall confidence")
	}
}

func TestApplicationBundleProducesRecordedConflict(t *testing.T) {
	p := mk(1, 0, "/Applications/ChatGPT.app/Contents/Resources/codex-code-mode-host", []string{"codex-code-mode-host"}, nil)
	s := InspectProcess(p)
	if len(s.Conflicts) == 0 {
		t.Fatal("a near miss must record why it was refused, not fail silently")
	}
}
