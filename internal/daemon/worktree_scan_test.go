package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/storage"
	"github.com/jamesonstone/ghostgc/internal/worktree"
)

func TestInventoryMergesSessionAndConfiguredRootSources(t *testing.T) {
	h := newRemovalHarness(t, false)
	h.daemon.cfg.Worktrees.Roots = []string{h.root}
	now := time.Now()
	agent := process.Process{
		PID: 4242, PPID: 1, PGID: 4242, SID: 4242, UID: 501,
		StartTime: now.Add(-time.Hour), ExecPath: "/opt/homebrew/bin/codex", Comm: "codex",
		Args: []string{"/opt/homebrew/bin/codex"}, CWD: h.secondary, Detailed: true,
		EnvReadable: true, Env: map[string]string{
			"CODEX_MANAGED_PACKAGE_ROOT": "/opt/homebrew/lib/node_modules/@openai/codex",
			"CODEX_HOME":                 "/tmp/codex-home",
		},
		Status: process.StatusSleeping,
	}
	h.fake.Push(process.NewSnapshot(now, []process.Process{agent}, 1))
	h.daemon.ScanNow(context.Background())
	records, err := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	var found storage.WorktreeRecord
	for _, record := range records {
		if record.Path == h.secondary {
			found = record
		}
	}
	if found.WorktreeID == "" {
		t.Fatalf("secondary absent from inventory: %+v", records)
	}
	if found.State != string(worktree.StateActive) {
		t.Fatalf("secondary state = %s", found.State)
	}
	var sources []string
	if err := json.Unmarshal([]byte(found.SourcesJSON), &sources); err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 || sources[0] != "configured_root" || sources[1] != "session" {
		t.Fatalf("sources = %v", sources)
	}
}

func TestInventorySurvivesUnsupportedProcessCollectionButCannotBecomeStale(t *testing.T) {
	h := newRemovalHarness(t, false)
	h.fake.PlatformName = "linux"
	h.daemon.ScanNow(context.Background())
	records, err := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) < 2 {
		t.Fatalf("inventory was not refreshed: %+v", records)
	}
	for _, record := range records {
		if record.State == string(worktree.StateRemoved) {
			continue
		}
		if record.State != string(worktree.StateUnknown) || record.Complete || record.InactiveSinceNs != 0 {
			t.Fatalf("incomplete platform evidence produced unsafe record: %+v", record)
		}
	}
}

func TestIncompletePerProcessEvidenceResetsInventory(t *testing.T) {
	h := newRemovalHarness(t, false)
	h.fake.Push(process.NewSnapshot(time.Now(), []process.Process{{
		PID: 44, UID: 501, StartTime: time.Now().Add(-time.Minute), Detailed: true,
		Status: process.StatusSleeping,
	}}, 1))
	h.daemon.ScanNow(context.Background())
	records, err := h.store.ListWorktrees(context.Background(), storage.WorktreeFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if record.State != string(worktree.StateUnknown) || record.Complete || record.InactiveSinceNs != 0 {
			t.Fatalf("incomplete CWD evidence produced unsafe record: %+v", record)
		}
	}
}

func TestWithinWorktreeBoundaries(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "workspace", "lane")
	tests := map[string]bool{
		root:                                  true,
		filepath.Join(root, "nested", "file"): true,
		filepath.Dir(root):                    false,
		filepath.Join(filepath.Dir(root), "other"): false,
	}
	for path, want := range tests {
		if got := withinWorktree(root, path); got != want {
			t.Errorf("withinWorktree(%q, %q) = %t, want %t", root, path, got, want)
		}
	}
}

func TestIncompleteInventoryAuditDoesNotRetainFilename(t *testing.T) {
	h := newRemovalHarness(t, false)
	sentinel := "private-client-filename"
	cause := &os.PathError{Op: "open", Path: filepath.Join(h.root, sentinel), Err: errors.New("denied")}
	batch := h.daemon.unknownWorktreeBatch(nil, time.Now(), cause)
	if len(batch.audit) != 1 || strings.Contains(batch.audit[0].EvidenceJSON, sentinel) {
		t.Fatalf("durable incomplete evidence retained filename: %+v", batch.audit)
	}
	if err := h.store.AppendAudit(context.Background(), batch.audit[0]); err != nil {
		t.Fatal(err)
	}
	stored, err := h.store.ListAudit(context.Background(), storage.AuditFilter{Kind: "worktree.scan.incomplete"})
	if err != nil || len(stored) != 1 || strings.Contains(stored[0].EvidenceJSON, sentinel) {
		t.Fatalf("persisted incomplete evidence retained filename: %+v, %v", stored, err)
	}
}

func TestAbsentRegistrationPreservesLastObservedTime(t *testing.T) {
	lastSeen := time.Now().Add(-time.Hour).UnixNano()
	record := storage.WorktreeRecord{State: string(worktree.StateObserving), LastSeenNs: lastSeen, Registered: true}
	missing := missingWorktreeRecord(record, time.Now(), time.Now())
	if missing.LastSeenNs != lastSeen || missing.Registered {
		t.Fatalf("absent registration = %+v", missing)
	}
}
