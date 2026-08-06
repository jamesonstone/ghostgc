package sessions

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/adapters/codex"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

const providerThreadID = "019fd729-549c-7de2-97f5-8681668ced15"

type providerCodexAdapter struct {
	*codex.Adapter
	state adapters.ProviderSessionState
}

func (a *providerCodexAdapter) DiscoverProviderSessions(_ context.Context, graph adapters.Graph) []adapters.ProviderSession {
	root, ok := graph.Roots[100]
	if !ok {
		return nil
	}
	return []adapters.ProviderSession{{
		AgentID: codex.ID, NativeID: providerThreadID, SessionID: providerThreadID,
		Root: root, State: a.state, StartedAt: t0.Add(time.Minute), ChangedAt: t0.Add(5 * time.Minute),
		Metadata: adapters.SessionMetadata{SessionID: providerThreadID, WorkingDir: "/repo/task", Invocation: "Codex task"},
		Evidence: []adapters.Evidence{{Kind: adapters.EvidenceProcessInfo, Detail: "fixture provider lifecycle"}},
	}}
}

func TestProviderTaskSessionOverridesHostAncestryAndSurvivesReparenting(t *testing.T) {
	adapter := &providerCodexAdapter{Adapter: codex.New(nil), state: adapters.ProviderSessionCompleted}
	r := New(adapters.NewRegistry(adapter), 99, selfUID, nil)
	init := mkProc(1, 0, "/sbin/launchd", 0)
	root := mkProc(100, 1, "/Applications/ChatGPT.app/Contents/Resources/codex", time.Second)
	root.Env = map[string]string{
		"CODEX_HOME": "/Users/dev/.codex", "CODEX_MANAGED_PACKAGE_ROOT": "/opt/@openai/codex",
	}
	child := mkProc(200, 100, "/opt/browser/chrome-headless-shell", 2*time.Second)
	child.Env = map[string]string{"CODEX_THREAD_ID": providerThreadID}

	first, firstTree := snap(10*time.Minute, init, root, child)
	result, err := r.Reconcile(t.Context(), first, firstTree, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Sessions) != 2 {
		t.Fatalf("sessions = %+v, want host plus provider task", result.Sessions)
	}
	task := sessionRecord(result, providerThreadID)
	if task == nil || task.State != string(StateCompleted) || task.EndedNs == nil || task.RootPID != root.PID {
		t.Fatalf("provider task = %+v", task)
	}
	childAttribution := result.Attributions[child.Key().UID()]
	if childAttribution.SessionID != providerThreadID || childAttribution.Relation != adapters.RelationDescendant ||
		childAttribution.Confidence < adapters.ConfidencePolicyEligible {
		t.Fatalf("task child attribution = %+v", childAttribution)
	}
	if rootAttribution := result.Attributions[root.Key().UID()]; rootAttribution.SessionID == providerThreadID {
		t.Fatalf("long-lived host root was collapsed into the task: %+v", rootAttribution)
	}
	r.Commit(result)

	detached := child
	detached.PPID = 1
	second, secondTree := snap(11*time.Minute, init, root, detached)
	result, err = r.Reconcile(t.Context(), second, secondTree, true)
	if err != nil {
		t.Fatal(err)
	}
	retained := result.Attributions[detached.Key().UID()]
	if retained.SessionID != providerThreadID || retained.Relation != adapters.RelationRecorded {
		t.Fatalf("detached ownership = %+v, want recorded provider task", retained)
	}
}

func TestKnownProviderSessionRetainsExactHomeAcrossRestart(t *testing.T) {
	metadata, err := json.Marshal(adapters.SessionMetadata{
		Extra: map[string]string{"CODEX_HOME": "/custom/codex-home"},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := New(adapters.NewRegistry(codex.New(nil)), 99, selfUID, nil)
	r.Seed([]storage.SessionRecord{{
		SessionID: providerThreadID, AgentID: codex.ID, State: string(StateActive),
		RootProcUID:     mkProc(100, 1, "/codex", time.Second).Key().UID(),
		NativeSessionID: providerThreadID, MetadataJSON: string(metadata),
	}}, nil)

	known := r.knownProviderSessions()
	if len(known) != 1 || known[0].Metadata.Extra["CODEX_HOME"] != "/custom/codex-home" {
		t.Fatalf("known sessions = %+v", known)
	}
}

func sessionRecord(result *Result, id string) *storage.SessionRecord {
	for i := range result.Sessions {
		if result.Sessions[i].SessionID == id {
			return &result.Sessions[i]
		}
	}
	return nil
}
