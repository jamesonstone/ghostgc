package codex

import (
	"path"
	"strings"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
)

// Signals is the evidence gathered about one process, before combination.
//
// Identity and membership are kept apart because they answer different
// questions and only one of them can create a session.
//
// Identity evidence — the executable, its path, the script a runtime was
// handed — is evidence that this process *is* the Codex program. Membership
// evidence — environment variables the agent sets — is evidence that this
// process is *inside* a Codex session. The distinction is not academic:
// environment variables are inherited by every descendant, so a service
// manager started from a Codex session months ago still carries
// CODEX_THREAD_ID. Treating that as identity promotes an unrelated long-lived
// daemon to "Codex session root", which is exactly the kind of confident
// misattribution this daemon exists to avoid.
type Signals struct {
	Identity   []adapters.Evidence
	Membership []adapters.Evidence
	Conflicts  []adapters.Evidence
	// NativeSessionID is the agent's own session identifier when the process
	// carries one.
	NativeSessionID string
}

// IdentityConfidence is how sure we are that this process is the Codex program
// itself. Only this may promote a process to a session root.
func (s Signals) IdentityConfidence() float64 { return adapters.CombineWeights(s.Identity) }

// Confidence combines every piece of evidence. It describes how sure we are
// that the process has something to do with Codex, which is a weaker claim
// than IdentityConfidence and must never be used to detect a root.
func (s Signals) Confidence() float64 { return adapters.CombineWeights(s.Evidence()) }

// Evidence returns identity followed by membership evidence, for reporting.
func (s Signals) Evidence() []adapters.Evidence {
	out := make([]adapters.Evidence, 0, len(s.Identity)+len(s.Membership))
	out = append(out, s.Identity...)
	return append(out, s.Membership...)
}

// InspectProcess gathers Codex detection signals for a single process.
//
// It is a pure function of the observation so that it can be tested against
// recorded real-world command lines, including the ones that must not match.
func InspectProcess(p process.Process) Signals {
	var s Signals

	execBase := path.Base(p.ExecPath)
	segs := segments(p.ExecPath)

	// A GUI application bundle is never the Codex CLI. The ChatGPT desktop
	// app ships "Codex Framework.framework" and helper executables whose names
	// begin with "Codex"; recording the conflict makes the refusal visible in
	// `ghostgc explain` rather than silent.
	inAppBundle := containsSegmentSuffix(segs, ".app") && containsSegment(segs, "Contents")
	if inAppBundle && strings.HasPrefix(strings.ToLower(execBase), "codex") {
		s.Conflicts = append(s.Conflicts, adapters.Evidence{
			Kind:   adapters.EvidenceExecutable,
			Detail: "executable lives inside a macOS application bundle (" + p.ExecPath + "); the Codex CLI does not",
		})
		return s
	}

	if p.ExecPath != "" && execBase == "codex" {
		s.Identity = append(s.Identity, adapters.Evidence{
			Kind:   adapters.EvidenceExecutable,
			Detail: "executable basename is exactly \"codex\" (" + p.ExecPath + ")",
			Weight: 0.55,
		})
	}

	if hasSegmentPair(segs, "@openai", "codex") {
		s.Identity = append(s.Identity, adapters.Evidence{
			Kind:   adapters.EvidenceExecutable,
			Detail: "executable path contains the @openai/codex package directory",
			Weight: 0.6,
		})
	} else if containsSegment(segs, "@openai") {
		s.Identity = append(s.Identity, adapters.Evidence{
			Kind:   adapters.EvidenceExecutable,
			Detail: "executable path contains an @openai package directory",
			Weight: 0.3,
		})
	}

	// The npm wrapper is a JavaScript runtime handed bin/codex.js.
	if len(p.Args) >= 2 && jsRuntimes[path.Base(p.Args[0])] {
		script := p.Args[1]
		scriptSegs := segments(script)
		switch {
		case path.Base(script) == "codex.js" && hasSegmentPair(scriptSegs, "@openai", "codex"):
			s.Identity = append(s.Identity, adapters.Evidence{
				Kind:   adapters.EvidenceArgv,
				Detail: "javascript runtime was given @openai/codex/bin/codex.js",
				Weight: 0.7,
			})
		case path.Base(script) == "codex.js":
			s.Identity = append(s.Identity, adapters.Evidence{
				Kind:   adapters.EvidenceArgv,
				Detail: "javascript runtime was given a script named codex.js",
				Weight: 0.4,
			})
		}
	}

	if v, ok := p.Env["CODEX_MANAGED_PACKAGE_ROOT"]; ok && v != "" {
		s.Membership = append(s.Membership, adapters.Evidence{
			Kind:   adapters.EvidenceEnvironment,
			Detail: "CODEX_MANAGED_PACKAGE_ROOT is set by the official Codex launcher before it execs the native binary",
			Weight: 0.85,
		})
	}
	if p.Env["CODEX_MANAGED_BY_NPM"] == "1" || p.Env["CODEX_MANAGED_BY_BUN"] == "1" {
		s.Membership = append(s.Membership, adapters.Evidence{
			Kind:   adapters.EvidenceEnvironment,
			Detail: "the Codex launcher marked this process as package-manager managed",
			Weight: 0.7,
		})
	}
	if v, ok := p.Env["CODEX_HOME"]; ok && v != "" {
		s.Membership = append(s.Membership, adapters.Evidence{
			Kind:   adapters.EvidenceEnvironment,
			Detail: "CODEX_HOME is set",
			Weight: 0.45,
		})
	}
	for _, key := range []string{"CODEX_SESSION_ID", "CODEX_THREAD_ID"} {
		v, ok := p.Env[key]
		if !ok || v == "" {
			continue
		}
		s.NativeSessionID = v
		s.Membership = append(s.Membership, adapters.Evidence{
			Kind:   adapters.EvidenceEnvironment,
			Detail: key + " names a Codex session directly",
			Weight: 0.9,
		})
		break
	}

	return s
}
