// Package codex detects OpenAI Codex CLI and macOS app sessions.
//
// Detection is built from independent structural signals — executable
// basename, path segments, the script a JavaScript runtime was handed, and
// environment variables the official launcher sets — combined with a noisy-or.
// No single signal establishes ownership.
//
// Regular expressions over raw command lines are deliberately not used. A
// machine running Codex is also likely to be running several unrelated things
// with "codex" in their command line: the macOS app ships an Electron framework
// literally named "Codex", and editor integrations pass "--agent codexCLI" to
// unrelated helper binaries. Substring matching would attribute all of them to
// a Codex session; exact structural matching does not.
package codex

import (
	"context"
	"fmt"
	"strings"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/repository"
)

// ID is the agent identifier.
const ID = "codex"

// Environment variables the adapter reads. CODEX_MANAGED_BY_NPM,
// CODEX_MANAGED_BY_BUN and CODEX_MANAGED_PACKAGE_ROOT are set by the official
// npm launcher before it execs the native binary, which makes them a far
// better ownership signal than anything visible in the command line.
var envKeys = []string{
	"CODEX_HOME",
	"CODEX_MANAGED_BY_BUN",
	"CODEX_MANAGED_BY_NPM",
	"CODEX_MANAGED_PACKAGE_ROOT",
	"CODEX_SANDBOX",
	"CODEX_SANDBOX_NETWORK_DISABLED",
	"CODEX_SESSION_ID",
	"CODEX_THREAD_ID",
}

// jsRuntimes are the interpreters the npm launcher can be running under.
var jsRuntimes = map[string]bool{"node": true, "bun": true, "deno": true, "node.exe": true}

// Adapter implements adapters.AgentAdapter for Codex.
type Adapter struct {
	repos *repository.Finder
}

// New constructs the Codex adapter.
func New(repos *repository.Finder) *Adapter {
	if repos == nil {
		repos = repository.NewFinder()
	}
	return &Adapter{repos: repos}
}

// ID implements adapters.AgentAdapter.
func (a *Adapter) ID() string { return ID }

// EnvKeys implements adapters.AgentAdapter.
func (a *Adapter) EnvKeys() []string {
	out := make([]string, len(envKeys))
	copy(out, envKeys)
	return out
}

// DetectRootProcesses implements adapters.AgentAdapter.
func (a *Adapter) DetectRootProcesses(ctx context.Context, g adapters.Graph) ([]adapters.AgentRoot, error) {
	if g.Snapshot == nil {
		return nil, nil
	}

	candidates := make(map[int]adapters.AgentRoot)
	for _, p := range g.Snapshot.Processes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !p.Detailed {
			// A process observed without a detail pass cannot be attributed;
			// guessing from the kernel-truncated comm is exactly the kind of
			// name-based inference this daemon must not do.
			continue
		}
		s := InspectProcess(p)
		// Only identity evidence may promote a process to a session root.
		// Membership evidence raises the recorded confidence once identity has
		// been established, and never on its own.
		if s.IdentityConfidence() < adapters.ConfidenceRootDetection {
			continue
		}
		conf := s.Confidence()
		meta, err := a.ExtractSessionMetadata(ctx, p)
		if err != nil {
			return nil, err
		}
		if s.NativeSessionID != "" {
			meta.SessionID = s.NativeSessionID
		}
		candidates[p.PID] = adapters.AgentRoot{
			AgentID:    ID,
			Process:    p,
			Key:        p.Key(),
			Metadata:   meta,
			Confidence: conf,
			Evidence:   s.Evidence(),
			Conflicts:  s.Conflicts,
		}
	}

	// The npm launcher and the native binary it execs are both Codex, and both
	// match. Only the topmost is the session root; the rest are descendants of
	// it. A candidate with its own distinct native session id is kept, because
	// that is the agent telling us it is a separate session.
	var roots []adapters.AgentRoot
	for pid, cand := range candidates {
		shadowed := false
		for _, ancestor := range g.Tree.Ancestors(pid) {
			parent, ok := candidates[ancestor]
			if !ok {
				continue
			}
			if cand.Metadata.SessionID != "" && cand.Metadata.SessionID != parent.Metadata.SessionID {
				continue
			}
			shadowed = true
			break
		}
		if !shadowed {
			roots = append(roots, cand)
		}
	}
	return roots, nil
}

// ExtractSessionMetadata implements adapters.AgentAdapter.
func (a *Adapter) ExtractSessionMetadata(ctx context.Context, p process.Process) (adapters.SessionMetadata, error) {
	meta := adapters.SessionMetadata{
		WorkingDir: p.CWD,
		TTY:        p.TTY,
	}
	meta.RepositoryPath = a.repos.Root(p.CWD)
	if len(p.Args) > 0 {
		redacted := process.RedactArgs(p.Args)
		meta.Invocation = strings.Join(redacted, " ")
		if len(meta.Invocation) > 512 {
			meta.Invocation = meta.Invocation[:512] + "…"
		}
	}
	if home := codexHome(p); home != "" {
		meta.Extra = map[string]string{"CODEX_HOME": home}
	}
	return meta, nil
}

func codexHome(p process.Process) string {
	if home := p.Env["CODEX_HOME"]; home != "" {
		return home
	}
	return defaultCodexHome()
}

// AttributeProcess implements adapters.AgentAdapter.
func (a *Adapter) AttributeProcess(ctx context.Context, p process.Process, g adapters.Graph) adapters.Attribution {
	if root, ok := g.Roots[p.PID]; ok && root.Key == p.Key() && root.AgentID == ID {
		return adapters.Attribution{
			AgentID:    ID,
			SessionID:  DeriveSessionID(root),
			Confidence: root.Confidence,
			Evidence:   root.Evidence,
			Conflicts:  root.Conflicts,
			Relation:   adapters.RelationRoot,
			RootKey:    root.Key,
		}
	}

	chain := g.Tree.Ancestors(p.PID)
	for depth, ancestor := range chain {
		root, ok := g.Roots[ancestor]
		if !ok || root.AgentID != ID {
			continue
		}
		ev := []adapters.Evidence{{
			Kind:   adapters.EvidenceAncestry,
			Detail: fmt.Sprintf("process is a descendant of Codex session root pid %d through %d intact parent link(s)", ancestor, depth+1),
		}}
		ev = append(ev, adapters.Evidence{
			Kind:   adapters.EvidenceProcessInfo,
			Detail: fmt.Sprintf("session root was identified with confidence %.2f", root.Confidence),
			Weight: root.Confidence,
		})
		return adapters.Attribution{
			AgentID: ID,
			// A descendant can never be more certainly attributed than the
			// session it descends from.
			SessionID:  DeriveSessionID(root),
			Confidence: root.Confidence,
			Evidence:   ev,
			Relation:   adapters.RelationDescendant,
			RootKey:    root.Key,
		}
	}
	return adapters.Attribution{}
}

// NativeSessionID implements adapters.AgentAdapter.
func (a *Adapter) NativeSessionID(p process.Process) (string, bool) {
	for _, key := range []string{"CODEX_SESSION_ID", "CODEX_THREAD_ID"} {
		if v := p.Env[key]; v != "" {
			return sanitize(v), true
		}
	}
	return "", false
}

// ProtectedPatterns implements adapters.AgentAdapter.
func (a *Adapter) ProtectedPatterns() []adapters.ProtectionRule {
	return []adapters.ProtectionRule{
		{
			ID:        "codex-session-root-v1",
			Reason:    "an active Codex session root is the user's session; terminating it would end their work",
			ExecNames: []string{"codex"},
		},
		{
			ID:          "codex-npm-launcher-v1",
			Reason:      "the @openai/codex launcher forwards signals to the native binary and owns the session lifetime",
			PathSegment: "@openai/codex",
		},
	}
}
