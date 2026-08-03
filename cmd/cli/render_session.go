package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

func renderSessionDetail(d api.SessionDetail) {
	s := d.Session
	fmt.Printf("Session %s (%s)\n", s.SessionID, s.AgentID)
	fmt.Printf("State: %s\n", s.State)
	fmt.Printf("Confidence: %.2f\n", s.Confidence)
	if s.PreviousState != "" && s.StateChangedNs > 0 {
		fmt.Printf("Previous state: %s (changed %s)\n", s.PreviousState,
			time.Unix(0, s.StateChangedNs).Format(time.RFC3339))
	}
	fmt.Printf("Root pid: %d\n", s.RootPID)
	if s.NativeSessionID != "" {
		fmt.Printf("Agent session id: %s\n", s.NativeSessionID)
	}
	if s.LaunchedBy != "" {
		fmt.Printf("Launched by: %s (pid %d)\n", s.LaunchedBy, s.LaunchedByPID)
	} else {
		fmt.Println("Launched by: unknown (the root was already reparented when ghostgc first saw it)")
	}
	if s.RepositoryPath != "" {
		line := s.RepositoryPath
		if s.Branch != "" {
			line += " on branch " + s.Branch
		}
		if s.RepositoryBusy {
			line += "  [a git operation is in flight]"
		}
		fmt.Printf("Repository: %s\n", line)
	}
	if s.WorkingDir != "" {
		fmt.Printf("Working directory: %s\n", s.WorkingDir)
	}
	if s.TTY != "" {
		fmt.Printf("Terminal: %s\n", s.TTY)
	}
	fmt.Printf("Age: %s\n", humanDuration(s.AgeSeconds))
	if s.EndedNs != nil {
		fmt.Printf("Ended: %s\n", time.Unix(0, *s.EndedNs).Format(time.RFC3339))
	}

	if len(d.Evidence) > 0 {
		fmt.Println("\nEvidence for this session:")
		for _, ev := range d.Evidence {
			if ev.Weight > 0 {
				fmt.Printf("- [%s, weight %.2f] %s\n", ev.Kind, ev.Weight, ev.Detail)
			} else {
				fmt.Printf("- [%s] %s\n", ev.Kind, ev.Detail)
			}
		}
	}

	fmt.Printf("\nProcesses (%d):\n", len(d.Processes))
	w := newTable()
	_, _ = fmt.Fprintln(w, "PID\tPPID\tCREATED BY\tNAME\tRELATION\tCONF\tSTATE\tAGE")
	for _, p := range d.Processes {
		_, _ = fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\t%.2f\t%s\t%s\n",
			p.PID, p.PPID, creator(p), p.Name, p.Relation, p.Confidence, p.State, humanDuration(p.AgeSeconds))
	}
	_ = w.Flush()

	renderRelationships(d.Relationships)

	if len(d.Audit) > 0 {
		fmt.Println("\nRecent audit entries:")
		for _, a := range d.Audit {
			fmt.Printf("  %s  %-28s %s\n", time.Unix(0, a.TsNs).Format(time.RFC3339), a.Kind, a.Summary)
		}
	}
}

// creator renders the original parent, or "unknown" when ghostgc never saw it.
// Printing 1 for a process that was already reparented would present the init
// process as the creator, which is not a fact anyone observed.
func creator(p api.ProcessSummary) string {
	if !p.OriginalParentObserved {
		return "unknown"
	}
	return fmt.Sprintf("%d", p.OriginalPPID)
}

func renderRelationships(rels []api.RelationshipView) {
	if len(rels) == 0 {
		return
	}
	fmt.Printf("\nRelationships (%d):\n", len(rels))
	w := newTable()
	_, _ = fmt.Fprintln(w, "KIND\tFROM\tTO\tOWNERSHIP\tDETAIL")
	for _, rel := range rels {
		to := "-"
		if rel.ToPID != 0 {
			to = fmt.Sprintf("%d", rel.ToPID)
		}
		ownership := "context"
		if rel.Attributing {
			ownership = "attributing"
		}
		_, _ = fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n", rel.Kind, rel.FromPID, to, ownership, rel.Detail)
	}
	_ = w.Flush()
	fmt.Println("\nOnly edges marked \"attributing\" can establish ownership. A shared terminal or")
	fmt.Println("repository says a human was in the same place, not that a session owns a process.")
}

func renderProcesses(r api.ProcessesResponse) {
	if len(r.Processes) == 0 {
		fmt.Println("No processes are attributed to an agent session.")
		if r.Note != "" {
			fmt.Printf("\n%s\n", r.Note)
		}
		return
	}
	w := newTable()
	_, _ = fmt.Fprintln(w, "PID\tSESSION\tAGENT\tRELATION\tCONF\tNAME\tSTATE\tRSS\tAGE")
	for _, p := range r.Processes {
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%.2f\t%s\t%s\t%s\t%s\n",
			p.PID, p.ShortID, p.AgentID, p.Relation, p.Confidence, p.Name, p.State,
			humanBytes(p.RSSBytes), humanDuration(p.AgeSeconds))
	}
	_ = w.Flush()
	if r.Note != "" {
		fmt.Printf("\n%s\n", r.Note)
	}
}

func renderExplain(r api.ExplainResponse) {
	fmt.Printf("Classification: %s\n", r.Classification)
	if r.Found {
		fmt.Printf("Process: pid %d (%s)\n", r.PID, r.Name)
		if r.ExecPath != "" {
			fmt.Printf("Executable: %s\n", r.ExecPath)
		}
		if len(r.Cmdline) > 0 {
			fmt.Printf("Command line: %s\n", strings.Join(r.Cmdline, " "))
		}
		fmt.Printf("Identity: %s (pid plus start time, so a recycled pid is a different process)\n", r.ProcUID)
		fmt.Printf("Parent link: %s\n", r.ParentLink)
		if r.OriginalParentObserved && r.OriginalPPID > 0 {
			fmt.Printf("Created by: pid %d (observed alive at the time)\n", r.OriginalPPID)
		} else if r.SessionID != "" {
			fmt.Println("Created by: unknown — the process was already reparented when ghostgc first observed it")
		}
		if r.RepositoryPath != "" {
			fmt.Printf("Repository: %s\n", r.RepositoryPath)
		}
		if !r.EnvironmentReadable {
			fmt.Println("Environment: not readable (the operating system withholds it for system binaries)")
		}
		if len(r.Descendants) > 0 {
			fmt.Printf("Live descendants: %d\n", len(r.Descendants))
		}
	}
	if r.SessionID != "" {
		fmt.Printf("Session: %s (%s, state %s), relation %s, confidence %.2f\n",
			r.SessionID, r.AgentID, r.SessionState, r.Relation, r.Confidence)
	}
	if r.Message != "" {
		fmt.Printf("\n%s\n", r.Message)
	}

	if len(r.Evidence) > 0 {
		fmt.Println("\nEvidence:")
		for _, ev := range r.Evidence {
			if ev.Weight > 0 {
				fmt.Printf("- [%s, weight %.2f] %s\n", ev.Kind, ev.Weight, ev.Detail)
			} else {
				fmt.Printf("- [%s] %s\n", ev.Kind, ev.Detail)
			}
		}
	}
	if len(r.Conflicts) > 0 {
		fmt.Println("\nConflicting evidence:")
		for _, ev := range r.Conflicts {
			fmt.Printf("- [%s] %s\n", ev.Kind, ev.Detail)
		}
	}
	if len(r.Relationships) > 0 {
		renderRelationships(r.Relationships)
	}
	if len(r.Protection.Rules) > 0 {
		fmt.Println("\nProtections that apply:")
		for _, rule := range r.Protection.Rules {
			fmt.Printf("- %s: %s\n", rule.ID, rule.Reason)
		}
	}
	fmt.Printf("\n%s\n", r.PolicyNote)
}
