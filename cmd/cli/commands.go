package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/version"
)

func cmdStatus(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "status", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resp, err := e.api().Status(ctx)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderStatus(resp)
	return nil
}

func cmdSessions(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "sessions", "[flags]")
	var opts api.ListOptions
	fs.StringVar(&opts.State, "state", "", "filter by session state")
	fs.StringVar(&opts.AgentID, "agent", "", "filter by agent id")
	fs.IntVar(&opts.Limit, "limit", 0, "maximum sessions to list")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resp, err := e.api().Sessions(ctx, opts)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderSessions(resp)
	return nil
}

func cmdSession(ctx context.Context, e *env, args []string) error {
	if len(args) < 2 || args[0] != "show" {
		return fmt.Errorf("usage: ghostgc session show <session-id>")
	}
	resp, err := e.api().Session(ctx, args[1])
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderSessionDetail(resp)
	return nil
}

func cmdProcesses(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "processes", "[flags]")
	var opts api.ListOptions
	fs.StringVar(&opts.SessionID, "session", "", "filter by session id")
	fs.StringVar(&opts.AgentID, "agent", "", "filter by agent id")
	fs.BoolVar(&opts.All, "all", false, "include processes that have exited")
	fs.IntVar(&opts.Limit, "limit", 0, "maximum processes to list")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resp, err := e.api().Processes(ctx, opts)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderProcesses(resp)
	return nil
}

func cmdExplain(ctx context.Context, e *env, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ghostgc explain <pid>")
	}
	pid, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("%q is not a pid", args[0])
	}
	resp, err := e.api().Explain(ctx, pid)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderExplain(resp)
	return nil
}

func cmdCandidates(ctx context.Context, e *env, args []string) error {
	resp, err := e.api().Candidates(ctx)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderCandidates(resp)
	return nil
}

func cmdPolicies(ctx context.Context, e *env, args []string) error {
	resp, err := e.api().Policies(ctx)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderPolicies(resp)
	return nil
}

func cmdPolicy(ctx context.Context, e *env, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: ghostgc policy enable|disable <policy-id>")
	}
	return fmt.Errorf("policies are configuration-managed: set enabled and mode in %s, then restart the daemon; runtime %s is not supported", e.paths.Config, args[0])
}

func cmdCleanup(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "cleanup", "--dry-run --process <pid:start> --policy <id> | --apply --approval <token> --yes")
	dryRun := fs.Bool("dry-run", false, "issue an exact short-lived cleanup preview")
	apply := fs.Bool("apply", false, "consume a preview approval")
	procUID := fs.String("process", "", "exact process identity pid:start_time_ns")
	policyID := fs.String("policy", "", "exact policy id")
	approval := fs.String("approval", "", "single-use approval from --dry-run")
	yes := fs.Bool("yes", false, "confirm the approved SIGTERM")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dryRun == *apply {
		return errors.New("choose exactly one of --dry-run or --apply")
	}
	if *dryRun {
		if *procUID == "" || *policyID == "" || *approval != "" || *yes {
			return errors.New("--dry-run requires --process and --policy only")
		}
		resp, err := e.api().CleanupPreview(ctx, api.CleanupPreviewRequest{PolicyID: *policyID, ProcUID: *procUID})
		if err != nil {
			return err
		}
		if e.jsonOut {
			return emitJSON(resp)
		}
		renderCleanupPreview(resp)
		return nil
	}
	if *approval == "" || !*yes || *procUID != "" || *policyID != "" {
		return errors.New("--apply requires only --approval and --yes")
	}
	resp, err := e.api().CleanupApply(ctx, api.CleanupApplyRequest{Approval: *approval})
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderCleanupResult(resp)
	return nil
}

func cmdActions(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "actions", "[flags]")
	var opts api.ActionOptions
	fs.StringVar(&opts.ProcUID, "process", "", "filter by exact process identity")
	fs.StringVar(&opts.PolicyID, "policy", "", "filter by policy id")
	fs.StringVar(&opts.Result, "result", "", "filter by result")
	fs.IntVar(&opts.Limit, "limit", 50, "maximum actions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resp, err := e.api().Actions(ctx, opts)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderActions(resp)
	return nil
}

func cmdLogs(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "logs", "[flags]")
	var opts api.LogOptions
	fs.IntVar(&opts.Limit, "limit", 50, "maximum entries")
	fs.StringVar(&opts.Kind, "kind", "", "filter by entry kind")
	fs.StringVar(&opts.Subject, "subject", "", "filter by subject")
	if err := fs.Parse(args); err != nil {
		return err
	}
	resp, err := e.api().Logs(ctx, opts)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderLogs(resp)
	return nil
}

func cmdMetrics(ctx context.Context, e *env, args []string) error {
	resp, err := e.api().Metrics(ctx)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(resp)
	}
	renderMetrics(resp)
	return nil
}

func cmdVersion(ctx context.Context, e *env, args []string) error {
	fmt.Printf("ghostgc %s\n", version.String())
	return nil
}
