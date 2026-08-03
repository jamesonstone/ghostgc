package main

import (
	"fmt"
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
	if len(r.Audited) > 0 {
		fmt.Printf("\n%d audit match(es):\n", len(r.Audited))
		for _, c := range r.Audited {
			fmt.Printf("PID %d\n  Policy: %s\n  Result: %s\n  Reason: %s\n", c.PID, c.PolicyID, c.Result, c.Reason)
		}
	}
	if r.Note != "" {
		fmt.Printf("\n%s\n", r.Note)
	}
}

func renderPolicies(r api.PoliciesResponse) {
	fmt.Printf("Global mode: %s\n", r.GlobalMode)
	if len(r.Policies) == 0 {
		fmt.Println("No policies are loaded.")
	} else {
		w := newTable()
		_, _ = fmt.Fprintln(w, "ID\tMODE\tDESCRIPTION")
		for _, p := range r.Policies {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", p.ID, p.Mode, p.Description)
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
		{"processes", fmt.Sprintf("%d visible, %d inspected, %d attributed", m.VisibleProcesses, m.InspectedProcesses, m.AttributedProcesses)},
		{"sessions", fmt.Sprintf("%d active", m.ActiveSessions)},
		{"cleanup candidates", fmt.Sprintf("%d", m.CleanupCandidates)},
		{"actions", fmt.Sprintf("%d attempted, %d rejected, %d completed", m.ActionsAttempted, m.ActionsRejected, m.ActionsCompleted)},
		{"database", fmt.Sprintf("%s (%d sessions, %d processes, %d observations, %d activity, %d edges, %d audit)",
			humanBytes(uint64(m.DatabaseBytes)), m.DatabaseCounts.Sessions, m.DatabaseCounts.Processes,
			m.DatabaseCounts.Observations, m.DatabaseCounts.ActivitySamples, m.DatabaseCounts.Relationships, m.DatabaseCounts.AuditEntries)},
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
