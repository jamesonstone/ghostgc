package sessions

import (
	"encoding/json"
	"fmt"

	"github.com/jamesonstone/ghostgc/internal/adapters"
	"github.com/jamesonstone/ghostgc/internal/process"
	"github.com/jamesonstone/ghostgc/internal/repository"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

func originalParentEvidence(ppid int, observed bool, link process.LinkState) adapters.Evidence {
	if !observed {
		return adapters.Evidence{
			Kind: adapters.EvidenceAncestry,
			Detail: fmt.Sprintf("the original parent is unknown: the process was already %s when ghostgc first observed it, so no creator was ever seen",
				link),
		}
	}
	return adapters.Evidence{
		Kind:   adapters.EvidenceAncestry,
		Detail: fmt.Sprintf("original parent was pid %d; the current parent link is %s", ppid, link),
	}
}

func launchEvidence(l LaunchContext) adapters.Evidence {
	return adapters.Evidence{
		Kind:   adapters.EvidenceAncestry,
		Detail: "session root was launched by " + l.Describe(),
	}
}

func repositoryEvidence(info repository.Info) adapters.Evidence {
	if info.Root == "" {
		return adapters.Evidence{
			Kind:   adapters.EvidenceWorkingDir,
			Detail: "the session root's working directory is not inside a repository",
		}
	}
	return adapters.Evidence{Kind: adapters.EvidenceWorkingDir, Detail: repositoryDetail(info)}
}

func repositoryDetail(info repository.Info) string {
	detail := "repository " + info.Root
	switch {
	case info.Branch != "":
		detail += " on branch " + info.Branch
	case info.Detached:
		detail += " with a detached HEAD"
	}
	if info.Busy() {
		detail += fmt.Sprintf(" (a git operation is in flight: %v)", info.Locks)
	}
	return detail
}

// compactEvidence drops entries with no detail so an evidence list never
// contains a blank line.
func compactEvidence(in []adapters.Evidence) []adapters.Evidence {
	out := in[:0]
	for _, e := range in {
		if e.Detail == "" {
			continue
		}
		out = append(out, e)
	}
	return out
}

func auditEntry(tsNs int64, kind, subject, summary string, evidence []adapters.Evidence) storage.AuditRecord {
	b, err := json.Marshal(evidence)
	if err != nil {
		b = []byte("[]")
	}
	return storage.AuditRecord{
		TsNs:         tsNs,
		Kind:         kind,
		Subject:      subject,
		Summary:      summary,
		EvidenceJSON: string(b),
	}
}
