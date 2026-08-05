package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

func renderCandidates(r api.CandidatesResponse) {
	if len(r.Enforceable) == 0 {
		fmt.Println("No enforceable cleanup candidates.")
	} else {
		w := newTable()
		_, _ = fmt.Fprintln(w, "PID\tPOLICY\tRESULT\tWOULD EXECUTE")
		for _, c := range r.Enforceable {
			_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", c.PID, c.PolicyID, c.Result, c.Command)
		}
		_ = w.Flush()
	}
	if len(r.Recommended) > 0 {
		fmt.Printf("\n%d manually actionable recommendation(s):\n", len(r.Recommended))
		w := newTable()
		_, _ = fmt.Fprintln(w, "PID\tPROCESS\tPOLICY\tSTATE\tPREVIEW")
		for _, c := range r.Recommended {
			_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", c.PID, c.ProcUID, c.PolicyID, c.State, c.Command)
		}
		_ = w.Flush()
	}
	if len(r.Audited) > 0 {
		fmt.Printf("\n%d current audit decision(s):\n", len(r.Audited))
		for _, c := range r.Audited {
			fmt.Printf("PID %d (%s)\n  Policy: %s\n  State: %s\n  Decision: %s\n  Classification: %s\n  Result: %s\n  Reason: %s\n",
				c.PID, c.ProcUID, c.PolicyID, c.State, time.Unix(0, c.DecisionTsNs).Format(time.RFC3339),
				time.Unix(0, c.ClassificationTsNs).Format(time.RFC3339), c.Result, c.Reason)
			for _, evidence := range c.Evidence {
				fmt.Printf("  Evidence: %s (%s)\n", evidence.Detail, evidence.Rule)
			}
		}
	}
	if r.Note != "" {
		fmt.Printf("\n%s\n", r.Note)
	}
}

func renderCleanupPreview(r api.CleanupPreviewResponse) {
	fmt.Printf("Exact target: %s (pid %d)\nPolicy: %s\nSignal: %s\nExpires: %s\n",
		r.Candidate.ProcUID, r.Candidate.PID, r.Candidate.PolicyID, r.Signal,
		time.Unix(0, r.ExpiresNs).Format(time.RFC3339))
	for _, gate := range r.Revalidation {
		fmt.Printf("Revalidate: %s\n", gate)
	}
	fmt.Printf("\nNo signal sent. To approve exactly this preview:\n%s\n\n%s\n", r.Command, r.Note)
}

func renderCleanupResult(r api.CleanupApplyResponse) {
	fmt.Printf("Action %s: %s\nProcess: %s\nPolicy: %s\nSignal: %s\nReason: %s\n",
		r.ActionID, r.Result, r.ProcUID, r.PolicyID, r.Signal, r.Reason)
}

func renderActions(r api.ActionsResponse) {
	if len(r.Actions) == 0 {
		fmt.Println("No cleanup actions recorded.")
		return
	}
	w := newTable()
	_, _ = fmt.Fprintln(w, "TIME\tACTION\tAUTHORITY\tPROCESS\tPOLICY\tRESULT\tSIGNAL\tREASON")
	for _, a := range r.Actions {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			time.Unix(0, a.RequestedNs).Format(time.RFC3339), a.ActionID, a.Authority, a.ProcUID,
			a.PolicyID, a.Result, a.Signal, a.Reason)
	}
	_ = w.Flush()
}

func renderPolicies(r api.PoliciesResponse) {
	fmt.Printf("Global mode: %s\n", r.GlobalMode)
	if len(r.Policies) == 0 {
		fmt.Println("No policies are loaded.")
	} else {
		w := newTable()
		_, _ = fmt.Fprintln(w, "ID\tENABLED\tMODE\tAUTOMATIC\tSTATES\tAGENTS\tEXECUTABLES\tDETACHED\tSESSION ENDED\tMIN STABLE\tCOOLDOWN\tDESCRIPTION")
		for _, p := range r.Policies {
			_, _ = fmt.Fprintf(w, "%s\t%t\t%s\t%t\t%s\t%s\t%s\t%t\t%t\t%s\t%s\t%s\n",
				p.ID, p.Enabled, p.Mode, p.Automatic, strings.Join(p.States, ","), strings.Join(p.Agents, ","),
				strings.Join(p.Executables, ","), p.RequireDetached, p.RequireSessionEnded,
				time.Duration(p.MinStableNs), time.Duration(p.CooldownNs), p.Description)
		}
		_ = w.Flush()
	}
	if r.Note != "" {
		fmt.Printf("\n%s\n", r.Note)
	}
}

func renderLogs(r api.LogsResponse) {
	if len(r.Entries) == 0 {
		fmt.Println("The audit log is empty.")
		return
	}
	// Oldest first reads better in a terminal.
	for i := len(r.Entries) - 1; i >= 0; i-- {
		a := r.Entries[i]
		fmt.Printf("%s  %-30s %s\n", time.Unix(0, a.TsNs).Format(time.RFC3339), a.Kind, a.Summary)
	}
}

func renderMetrics(m api.MetricsResponse) {
	w := newTable()
	rows := [][2]string{
		{"scans", fmt.Sprintf("%d (%d failed)", m.ScanCount, m.ScanFailures)},
		{"scan duration", fmt.Sprintf("last %.1f ms, mean %.1f ms, max %.1f ms", m.LastScanDurationMs, m.MeanScanDurationMs, m.MaxScanDurationMs)},
		{"reconcile / persist", fmt.Sprintf("%.1f ms / %.1f ms", m.LastReconcileMs, m.LastPersistMs)},
		{"activity", fmt.Sprintf("%.1f ms last pass, %d samples", m.LastActivityMs, m.ActivitySamples)},
		{"classifications", fmt.Sprintf("%d conclusions", m.Classifications)},
		{"policy decisions", fmt.Sprintf("%d decisions", m.PolicyDecisions)},
		{"processes", fmt.Sprintf("%d visible, %d inspected, %d attributed", m.VisibleProcesses, m.InspectedProcesses, m.AttributedProcesses)},
		{"sessions", fmt.Sprintf("%d active", m.ActiveSessions)},
		{"cleanup candidates", fmt.Sprintf("%d", m.CleanupCandidates)},
		{"actions", fmt.Sprintf("%d attempted, %d rejected, %d completed", m.ActionsAttempted, m.ActionsRejected, m.ActionsCompleted)},
		{"cache scans", fmt.Sprintf("%d (%d failed), last %.1f ms", m.CacheScanCount, m.CacheScanFailures, m.LastCacheScanMs)},
		{"cache observations", fmt.Sprintf("%d inspected, %d protected, %d candidates", m.CacheInspected, m.CacheProtected, m.CacheCandidates)},
		{"cache bytes", fmt.Sprintf("%s quarantined, %s purged", humanBytes(uint64(m.CacheQuarantinedBytes)), humanBytes(uint64(m.CachePurgedBytes)))},
		{"worktrees", fmt.Sprintf("%d inventoried, %d stale, %d protected", m.WorktreeInventory, m.WorktreeStale, m.WorktreeProtected)},
		{"worktree actions", fmt.Sprintf("%d attempted, %d rejected, %d removed, %d failed", m.WorktreeActionsAttempted, m.WorktreeActionsRejected, m.WorktreeActionsRemoved, m.WorktreeActionsFailed)},
		{"database", fmt.Sprintf("%s (%d sessions, %d processes, %d observations, %d activity, %d classifications, %d policy decisions, %d edges, %d audit)",
			humanBytes(uint64(m.DatabaseBytes)), m.DatabaseCounts.Sessions, m.DatabaseCounts.Processes,
			m.DatabaseCounts.Observations, m.DatabaseCounts.ActivitySamples, m.DatabaseCounts.Classifications, m.DatabaseCounts.PolicyDecisions, m.DatabaseCounts.Relationships, m.DatabaseCounts.AuditEntries)},
		{"cache database", fmt.Sprintf("%d artifacts, %d candidates, %d quarantined, %d actions", m.DatabaseCounts.CacheArtifacts, m.DatabaseCounts.CacheCandidates, m.DatabaseCounts.CacheQuarantine, m.DatabaseCounts.CacheActions)},
		{"retention", fmt.Sprintf("%d runs, %d rows removed last pass", m.RetentionRuns, m.LastRetentionDeleted)},
		{"daemon memory", humanBytes(m.RSSBytes)},
		{"goroutines", fmt.Sprintf("%d", m.Goroutines)},
	}
	for _, row := range rows {
		_, _ = fmt.Fprintf(w, "%s\t%s\n", row[0], row[1])
	}
	_ = w.Flush()
}

func renderDoctor(r api.DoctorResponse) {
	w := newTable()
	for _, c := range r.Checks {
		marker := "ok  "
		switch c.Status {
		case api.CheckWarn:
			marker = "warn"
		case api.CheckError:
			marker = "FAIL"
		}
		_, _ = fmt.Fprintf(w, "[%s]\t%s\t%s\n", marker, c.Name, c.Detail)
	}
	_ = w.Flush()

	remedies := false
	for _, c := range r.Checks {
		if c.Remedy == "" {
			continue
		}
		if !remedies {
			fmt.Println("\nSuggested actions:")
			remedies = true
		}
		fmt.Printf("- %s: %s\n", c.Name, c.Remedy)
	}
	if r.OK {
		fmt.Println("\nNo problems found.")
	}
}
