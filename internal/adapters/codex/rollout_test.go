package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
)

const rolloutThreadID = "019fd729-549c-7de2-97f5-8681668ced15"

func TestDiscoverProviderSessionUsesLatestBoundedLifecycle(t *testing.T) {
	home := physicalTempDir(t)
	writeRolloutFixture(t, home, rolloutThreadID, rolloutThreadID, "task_started", "task_complete")
	if lifecycle, path, ok := readRollout(home, rolloutThreadID, uint32(os.Getuid())); !ok {
		secureHome, homeOK := secureDirectory(home, uint32(os.Getuid()))
		secureSessions, sessionsOK := secureDirectory(filepath.Join(home, "sessions"), uint32(os.Getuid()))
		matches, _ := filepath.Glob(filepath.Join(home, "sessions", "*", "*", "*", "rollout-*-"+rolloutThreadID+".jsonl"))
		secureFile := len(matches) == 1 && securePath(matches[0], filepath.Join(home, "sessions"), uint32(os.Getuid()))
		t.Fatalf("trusted rollout was not readable: lifecycle=%+v path=%q home=%q/%t sessions=%q/%t matches=%v secure=%t",
			lifecycle, path, secureHome, homeOK, secureSessions, sessionsOK, matches, secureFile)
	}
	adapter, graph := rolloutGraph(t, home, rolloutThreadID)

	sessions := adapter.DiscoverProviderSessions(t.Context(), graph)
	if len(sessions) != 1 {
		t.Fatalf("provider sessions = %+v, want one", sessions)
	}
	got := sessions[0]
	if got.NativeID != rolloutThreadID || got.SessionID != rolloutThreadID ||
		got.State != adapters.ProviderSessionCompleted || got.Root.Process.PID != 100 {
		t.Fatalf("provider session = %+v", got)
	}
	if got.Metadata.WorkingDir != "/repo/task" || got.Metadata.Extra["CODEX_HOME"] != home {
		t.Fatalf("provider metadata = %+v", got.Metadata)
	}
}

func TestDiscoverProviderSessionRefreshesKnownCustomHome(t *testing.T) {
	home := physicalTempDir(t)
	writeRolloutFixture(t, home, rolloutThreadID, rolloutThreadID, "task_complete")
	adapter, graph := rolloutGraph(t, home, rolloutThreadID)
	root := graph.Roots[100]
	graph.Snapshot = process.NewSnapshot(graph.Snapshot.Taken, []process.Process{root.Process}, 1)
	graph.Tree = process.BuildTree(graph.Snapshot)
	graph.KnownSessions = []adapters.KnownSession{{
		AgentID: ID, NativeID: rolloutThreadID, SessionID: rolloutThreadID, RootKey: root.Key,
		Metadata: adapters.SessionMetadata{Extra: map[string]string{"CODEX_HOME": home}},
	}}

	sessions := adapter.DiscoverProviderSessions(t.Context(), graph)
	if len(sessions) != 1 || sessions[0].Metadata.Extra["CODEX_HOME"] != home {
		t.Fatalf("known provider session = %+v, want custom home %q", sessions, home)
	}
}

func TestDiscoverProviderSessionFailsClosedOnUntrustedRollout(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string)
	}{
		{
			name: "metadata id mismatch",
			setup: func(t *testing.T, home string) {
				writeRolloutFixture(t, home, rolloutThreadID, "019fd729-549c-7de2-97f5-8681668ced16", "task_complete")
			},
		},
		{
			name: "ambiguous files",
			setup: func(t *testing.T, home string) {
				writeRolloutAt(t, home, "2026/08/05", rolloutThreadID, rolloutThreadID, "task_complete")
				writeRolloutAt(t, home, "2026/08/06", rolloutThreadID, rolloutThreadID, "task_complete")
			},
		},
		{
			name: "linked file",
			setup: func(t *testing.T, home string) {
				targetHome := physicalTempDir(t)
				target := writeRolloutAt(t, targetHome, "2026/08/06", rolloutThreadID, rolloutThreadID, "task_complete")
				path := rolloutFixturePath(home, "2026/08/06", rolloutThreadID)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "writable parent",
			setup: func(t *testing.T, home string) {
				writeRolloutFixture(t, home, rolloutThreadID, rolloutThreadID, "task_complete")
				if err := os.Chmod(filepath.Join(home, "sessions", "2026"), 0o777); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := physicalTempDir(t)
			tt.setup(t, home)
			adapter, graph := rolloutGraph(t, home, rolloutThreadID)
			if got := adapter.DiscoverProviderSessions(t.Context(), graph); len(got) != 0 {
				t.Fatalf("untrusted rollout produced sessions: %+v", got)
			}
		})
	}
}

func rolloutGraph(t *testing.T, home, threadID string) (*Adapter, adapters.Graph) {
	t.Helper()
	uid := uint32(os.Getuid())
	root := mk(100, 1, "/Applications/ChatGPT.app/Contents/Resources/codex",
		[]string{"codex", "app-server"}, nil)
	root.UID = uid
	child := mk(200, 100, "/usr/local/bin/node", []string{"node", "worker.js"}, map[string]string{
		"CODEX_HOME": home, "CODEX_THREAD_ID": threadID,
	})
	child.UID = uid
	unreadable := mk(150, 1, "/usr/bin/unreadable", []string{"unreadable"}, nil)
	unreadable.UID = uid
	unreadable.Detailed = false
	init := mk(1, 0, "/sbin/launchd", []string{"launchd"}, nil)
	init.UID = uid
	snapshot := process.NewSnapshot(start.Add(time.Hour), []process.Process{init, root, unreadable, child}, 4)
	graph := adapters.Graph{Snapshot: snapshot, Tree: process.BuildTree(snapshot), Roots: map[int]adapters.AgentRoot{}}
	adapter := New(nil)
	roots, err := adapter.DetectRootProcesses(t.Context(), graph)
	if err != nil || len(roots) != 1 {
		t.Fatalf("Codex roots = %+v, %v", roots, err)
	}
	graph.Roots[root.PID] = roots[0]
	return adapter, graph
}

func writeRolloutFixture(t *testing.T, home, pathID, metadataID string, events ...string) string {
	t.Helper()
	return writeRolloutAt(t, home, "2026/08/06", pathID, metadataID, events...)
}

func writeRolloutAt(t *testing.T, home, date, pathID, metadataID string, events ...string) string {
	t.Helper()
	path := rolloutFixturePath(home, date, pathID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	meta := map[string]any{
		"timestamp": start.Add(time.Minute).Format(time.RFC3339Nano), "type": "session_meta",
		"payload": map[string]any{"id": metadataID, "timestamp": start.Add(time.Minute).Format(time.RFC3339Nano), "cwd": "/repo/task"},
	}
	lines := [][]byte{mustJSON(t, meta)}
	for i, event := range events {
		lines = append(lines, mustJSON(t, map[string]any{
			"timestamp": start.Add(time.Duration(i+2) * time.Minute).Format(time.RFC3339Nano),
			"type":      "event_msg", "payload": map[string]any{"type": event},
		}))
	}
	content := append([]byte{}, lines[0]...)
	for _, line := range lines[1:] {
		content = append(content, '\n')
		content = append(content, line...)
	}
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func rolloutFixturePath(home, date, id string) string {
	return filepath.Join(home, "sessions", filepath.FromSlash(date), "rollout-2026-08-06T09-00-38-"+id+".jsonl")
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func physicalTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}
