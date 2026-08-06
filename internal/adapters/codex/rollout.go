package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
)

const (
	maxRolloutMetadataBytes = 1 << 20
	maxRolloutTailBytes     = 2 << 20
)

type rolloutCandidate struct {
	root      adapters.AgentRoot
	homes     map[string]bool
	ambiguous bool
}

type rolloutLifecycle struct {
	id        string
	cwd       string
	home      string
	state     adapters.ProviderSessionState
	startedAt time.Time
	changedAt time.Time
}

// DiscoverProviderSessions implements adapters.SessionLifecycleAdapter. It
// considers only native IDs already visible in a process environment or known
// from a committed session; it never inventories all Codex rollouts.
func (a *Adapter) DiscoverProviderSessions(ctx context.Context, g adapters.Graph) []adapters.ProviderSession {
	candidates := make(map[string]*rolloutCandidate)
	for _, p := range g.Snapshot.Processes {
		if ctx.Err() != nil {
			break
		}
		if !p.Detailed {
			continue
		}
		native, ok := a.NativeSessionID(p)
		if !ok || !validThreadID(native) {
			continue
		}
		root, ok := providerHostRoot(p, g)
		if !ok || root.Metadata.SessionID == native {
			continue
		}
		addRolloutCandidate(candidates, native, root, codexHome(p))
	}
	for _, known := range g.KnownSessions {
		if known.AgentID != ID || !validThreadID(known.NativeID) {
			continue
		}
		root, ok := g.Roots[known.RootKey.PID]
		if !ok || root.Key != known.RootKey || root.Metadata.SessionID == known.NativeID {
			continue
		}
		home := known.Metadata.Extra["CODEX_HOME"]
		if home == "" {
			continue
		}
		addRolloutCandidate(candidates, known.NativeID, root, home)
	}

	ids := make([]string, 0, len(candidates))
	for id := range candidates {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []adapters.ProviderSession
	for _, id := range ids {
		candidate := candidates[id]
		if candidate.ambiguous {
			continue
		}
		lifecycle, ok := resolveRollout(candidate.homes, id, candidate.root.Process.UID)
		if !ok {
			continue
		}
		evidence := []adapters.Evidence{
			{Kind: adapters.EvidenceAncestry, Detail: fmt.Sprintf("native Codex task %s is hosted by identified Codex root pid %d", id, candidate.root.Process.PID)},
			{Kind: adapters.EvidenceEnvironment, Detail: "the process environment and rollout session_meta carry the same exact Codex thread ID"},
			{Kind: adapters.EvidenceProcessInfo, Detail: fmt.Sprintf("the latest bounded rollout lifecycle event is %s at %s", lifecycle.state, lifecycle.changedAt.Format(time.RFC3339))},
		}
		out = append(out, adapters.ProviderSession{
			AgentID: ID, NativeID: id, SessionID: sanitize(id), Root: candidate.root,
			State: lifecycle.state, StartedAt: lifecycle.startedAt, ChangedAt: lifecycle.changedAt,
			Metadata: adapters.SessionMetadata{
				SessionID: id, WorkingDir: lifecycle.cwd, Invocation: "Codex task " + id,
				Extra: map[string]string{"CODEX_HOME": lifecycle.home, "lifecycle_source": "codex-rollout"},
			},
			Evidence: evidence,
		})
	}
	return out
}

func addRolloutCandidate(all map[string]*rolloutCandidate, id string, root adapters.AgentRoot, home string) {
	candidate := all[id]
	if candidate == nil {
		candidate = &rolloutCandidate{root: root, homes: make(map[string]bool)}
		all[id] = candidate
	} else if candidate.root.Key != root.Key {
		candidate.ambiguous = true
	}
	if home != "" {
		candidate.homes[home] = true
	}
}

func providerHostRoot(p process.Process, g adapters.Graph) (adapters.AgentRoot, bool) {
	for _, pid := range g.Tree.Ancestors(p.PID) {
		root, ok := g.Roots[pid]
		if ok && root.AgentID == ID && root.Process.UID == p.UID {
			return root, true
		}
	}
	return adapters.AgentRoot{}, false
}

func resolveRollout(homes map[string]bool, id string, uid uint32) (rolloutLifecycle, bool) {
	var found []rolloutLifecycle
	paths := make(map[string]bool)
	for home := range homes {
		lifecycle, path, ok := readRollout(home, id, uid)
		if !ok || paths[path] {
			continue
		}
		paths[path] = true
		found = append(found, lifecycle)
	}
	if len(found) != 1 {
		return rolloutLifecycle{}, false
	}
	return found[0], true
}

func readRollout(home, id string, uid uint32) (rolloutLifecycle, string, bool) {
	home, ok := secureDirectory(home, uid)
	if !ok {
		return rolloutLifecycle{}, "", false
	}
	sessionsRoot, ok := secureDirectory(filepath.Join(home, "sessions"), uid)
	if !ok || !within(home, sessionsRoot) {
		return rolloutLifecycle{}, "", false
	}
	pattern := filepath.Join(sessionsRoot, "*", "*", "*", "rollout-*-"+id+".jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) != 1 || !securePath(matches[0], sessionsRoot, uid) {
		return rolloutLifecycle{}, "", false
	}
	metadata, tail, ok := boundedRolloutRead(matches[0], uid)
	if !ok {
		return rolloutLifecycle{}, "", false
	}
	lifecycle, ok := parseRollout(metadata, tail, id)
	if !ok {
		return rolloutLifecycle{}, "", false
	}
	lifecycle.home = home
	return lifecycle, matches[0], true
}

func boundedRolloutRead(path string, uid uint32) ([]byte, []byte, bool) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, false
	}
	file := os.NewFile(uintptr(fd), path)
	defer func() { _ = file.Close() }()
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Uid != uid || stat.Nlink != 1 || stat.Mode&0o022 != 0 {
		return nil, nil, false
	}
	metadataLimit := int64(maxRolloutMetadataBytes + 1)
	metadata := make([]byte, metadataLimit)
	n, err := file.ReadAt(metadata, 0)
	if err != nil && n == 0 {
		return nil, nil, false
	}
	metadata = metadata[:n]
	newline := bytes.IndexByte(metadata, '\n')
	if newline < 0 || newline > maxRolloutMetadataBytes {
		return nil, nil, false
	}
	metadata = metadata[:newline]
	tailSize := int64(maxRolloutTailBytes)
	if stat.Size < tailSize {
		tailSize = stat.Size
	}
	tail := make([]byte, tailSize)
	if tailSize > 0 {
		n, _ = file.ReadAt(tail, stat.Size-tailSize)
		tail = tail[:n]
	}
	return metadata, tail, true
}

func parseRollout(metadata, tail []byte, wantID string) (rolloutLifecycle, bool) {
	var meta struct {
		Type    string `json:"type"`
		Payload struct {
			ID        string `json:"id"`
			Timestamp string `json:"timestamp"`
			CWD       string `json:"cwd"`
		} `json:"payload"`
	}
	if json.Unmarshal(metadata, &meta) != nil || meta.Type != "session_meta" || meta.Payload.ID != wantID {
		return rolloutLifecycle{}, false
	}
	startedAt, err := time.Parse(time.RFC3339Nano, meta.Payload.Timestamp)
	if err != nil {
		return rolloutLifecycle{}, false
	}
	lines := bytes.Split(tail, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		var event struct {
			Timestamp string `json:"timestamp"`
			Type      string `json:"type"`
			Payload   struct {
				Type string `json:"type"`
			} `json:"payload"`
		}
		if json.Unmarshal(lines[i], &event) != nil || event.Type != "event_msg" {
			continue
		}
		var state adapters.ProviderSessionState
		switch event.Payload.Type {
		case "task_started":
			state = adapters.ProviderSessionActive
		case "task_complete":
			state = adapters.ProviderSessionCompleted
		default:
			continue
		}
		changedAt, err := time.Parse(time.RFC3339Nano, event.Timestamp)
		if err != nil || changedAt.Before(startedAt) {
			return rolloutLifecycle{}, false
		}
		return rolloutLifecycle{id: wantID, cwd: meta.Payload.CWD, state: state, startedAt: startedAt, changedAt: changedAt}, true
	}
	return rolloutLifecycle{}, false
}

func validThreadID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, r := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}
