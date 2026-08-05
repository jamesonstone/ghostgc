package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

func renderWorktrees(response api.WorktreesResponse) {
	if len(response.Worktrees) == 0 {
		fmt.Println("No worktrees have been inventoried yet.")
		return
	}
	w := newTable()
	_, _ = fmt.Fprintln(w, "ID\tSTATE\tINACTIVE\tBRANCH\tSOURCE\tPATH\tPROTECTION")
	for _, item := range response.Worktrees {
		inactive := "-"
		if item.InactiveSinceNs > 0 {
			inactive = humanDuration(item.InactiveSeconds)
		}
		branch := item.Branch
		if branch == "" {
			branch = "detached"
		}
		protection := strings.Join(item.Protection, ",")
		if protection == "" {
			protection = "-"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", item.ShortID,
			item.State, inactive, branch, strings.Join(item.Sources, ","), item.Path, protection)
	}
	_ = w.Flush()
}

func renderWorktree(item api.WorktreeView) {
	fmt.Printf("Worktree: %s\nPath: %s\nState: %s\nBranch: %s\nHEAD: %s\nSources: %s\n",
		item.WorktreeID, item.Path, item.State, emptyDisplay(item.Branch, "detached"), item.HEAD,
		strings.Join(item.Sources, ", "))
	if item.InactiveSinceNs > 0 {
		fmt.Printf("Inactive: %s (since %s)\n", humanDuration(item.InactiveSeconds),
			time.Unix(0, item.InactiveSinceNs).Format(time.RFC3339))
	}
	fmt.Printf("Last activity: %s\nEvidence complete: %t\n", time.Unix(0, item.LastActivityNs).Format(time.RFC3339), item.Complete)
	for _, reason := range item.Protection {
		fmt.Printf("Protected: %s\n", reason)
	}
	if item.RecreateCommand != "" {
		fmt.Printf("Recreate: %s\n", item.RecreateCommand)
	}
}

func renderWorktreeRemovalPreview(response api.WorktreeRemovalPreviewResponse) {
	fmt.Printf("Exact worktree: %s\nPath: %s\nBranch: %s\nExpires: %s\n",
		response.Worktree.WorktreeID, response.Worktree.Path,
		emptyDisplay(response.Worktree.Branch, "detached"), time.Unix(0, response.ExpiresNs).Format(time.RFC3339))
	for _, gate := range response.Revalidation {
		fmt.Printf("Revalidate: %s\n", gate)
	}
	fmt.Printf("\nNo mutation performed. To approve exactly this preview:\n%s\n\n%s\n", response.Command, response.Note)
}

func renderWorktreeRemoval(response api.WorktreeRemovalApplyResponse) {
	fmt.Printf("Action %s: %s\nWorktree: %s\nPath: %s\nBranch: %s\nReason: %s\n",
		response.ActionID, response.Result, response.WorktreeID, response.Path,
		emptyDisplay(response.Branch, "detached"), response.Reason)
	if response.RecreateCommand != "" {
		fmt.Printf("Recreate: %s\n", response.RecreateCommand)
	}
}

func renderWorktreeActions(response api.WorktreeActionsResponse) {
	if len(response.Actions) == 0 {
		fmt.Println("No worktree lifecycle actions recorded.")
		return
	}
	w := newTable()
	_, _ = fmt.Fprintln(w, "TIME\tACTION\tWORKTREE\tRESULT\tBRANCH\tPATH\tREASON")
	for _, action := range response.Actions {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			time.Unix(0, action.RequestedNs).Format(time.RFC3339), action.ActionID,
			shortDisplayID(action.WorktreeID), action.Result, emptyDisplay(action.Branch, "detached"),
			action.Path, action.Reason)
	}
	_ = w.Flush()
}

func emptyDisplay(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
func shortDisplayID(value string) string {
	if len(value) > 8 {
		return value[:8]
	}
	return value
}
