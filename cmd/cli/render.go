package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func newTable() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}

// humanDuration formats an age the way the specification's examples do.
func humanDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func renderStatus(s api.StatusResponse) {
	fmt.Printf("Daemon: %s\n", s.Health)
	fmt.Printf("Mode: %s\n", s.Mode)
	fmt.Printf("Agents: %d (%s)\n", len(s.Agents), strings.Join(s.Agents, ", "))
	fmt.Printf("Sessions: %d\n", s.Sessions)

	states := make([]string, 0, len(s.SessionsByState))
	for state := range s.SessionsByState {
		states = append(states, state)
	}
	sort.Strings(states)
	for _, state := range states {
		fmt.Printf("%s: %d\n", strings.ToUpper(state[:1])+state[1:], s.SessionsByState[state])
	}

	fmt.Printf("Cleanup candidates: %d\n", s.CleanupCandidates)
	if s.LastScan != nil {
		fmt.Printf("Last scan: %s ago (%d visible, %d inspected, %d attributed, %.0f ms)\n",
			humanDuration(s.LastScan.AgeSeconds), s.LastScan.VisibleProcesses,
			s.LastScan.InspectedProcesses, s.LastScan.AttributedProcesses, s.LastScan.DurationMs)
	} else {
		fmt.Println("Last scan: none yet")
	}
	fmt.Printf("Uptime: %s\n", humanDuration(s.UptimeSeconds))

	for _, reason := range s.Degraded {
		fmt.Printf("\nDegraded: %s\n", reason)
	}
	if !s.SignallingEnabled {
		fmt.Printf("\nThis build observes only. No process can be signalled.\nDelivery phase %s\n", s.Phase)
	}
}

func renderSessions(r api.SessionsResponse) {
	if len(r.Sessions) == 0 {
		fmt.Println("No agent sessions have been observed.")
		fmt.Println("A session appears once an enabled adapter recognises an agent process; run `ghostgc doctor` if you expected one.")
		return
	}
	w := newTable()
	_, _ = fmt.Fprintln(w, "ID\tAGENT\tREPOSITORY\tSTATE\tCONF\tAGE\tPROCESSES\tROOT\tLAUNCHED BY")
	for _, s := range r.Sessions {
		repo := s.Repository
		switch {
		case repo == "":
			repo = "-"
		case s.Branch != "":
			repo += "@" + s.Branch
		}
		if s.RepositoryBusy {
			repo += " (busy)"
		}
		procs := fmt.Sprintf("%d", s.Processes)
		if s.LiveProcesses != s.Processes {
			procs = fmt.Sprintf("%d (%d live)", s.Processes, s.LiveProcesses)
		}
		launched := s.LaunchedBy
		if launched == "" {
			launched = "unknown"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.2f\t%s\t%s\t%d\t%s\n",
			s.ShortID, s.AgentID, repo, s.State, s.Confidence, humanDuration(s.AgeSeconds),
			procs, s.RootPID, launched)
	}
	_ = w.Flush()
	if r.Note != "" {
		fmt.Printf("\n%s\n", r.Note)
	}
}
