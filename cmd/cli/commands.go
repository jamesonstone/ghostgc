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
	return fmt.Errorf(
		"there are no cleanup policies to %s: the policy engine arrives in delivery phase 5, and this build has no policy storage, no evaluation and no action path",
		args[0])
}

func cmdCleanup(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "cleanup", "--dry-run | --apply")
	dryRun := fs.Bool("dry-run", false, "show what would be done")
	apply := fs.Bool("apply", false, "perform the cleanup")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *apply {
		return errors.New(
			"refusing: this build cannot terminate a process. Cleanup is introduced in delivery phase 6 as a manually approved SIGTERM behind full pre-action revalidation, and phase 7 adds narrow enforcement. Neither the policy engine nor the safety gates exist yet")
	}
	if !*dryRun {
		return errors.New("specify --dry-run; --apply is not available in this build")
	}
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
	fmt.Printf("delivery phase %s\n", version.Phase)
	return nil
}
