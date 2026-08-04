package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jamesonstone/ghostgc/internal/api"
)

func cmdWorktrees(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "worktrees", "[flags]")
	var opts api.WorktreeOptions
	fs.StringVar(&opts.State, "state", "", "filter by inventory state")
	fs.StringVar(&opts.Source, "source", "", "filter by session or configured_root source")
	fs.IntVar(&opts.Limit, "limit", 100, "maximum worktrees")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("worktrees accepts flags only")
	}
	response, err := e.api().Worktrees(ctx, opts)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(response)
	}
	renderWorktrees(response)
	return nil
}

func cmdWorktree(ctx context.Context, e *env, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ghostgc worktree show <id-or-prefix> | remove [flags] | actions [flags]")
	}
	switch args[0] {
	case "show":
		if len(args) != 2 {
			return errors.New("usage: ghostgc worktree show <id-or-prefix>")
		}
		response, err := e.api().Worktree(ctx, args[1])
		if err != nil {
			return err
		}
		if e.jsonOut {
			return emitJSON(response)
		}
		renderWorktree(response)
		return nil
	case "remove":
		return cmdWorktreeRemove(ctx, e, args[1:])
	case "actions":
		return cmdWorktreeActions(ctx, e, args[1:])
	default:
		return fmt.Errorf("unknown worktree subcommand %q", args[0])
	}
}

func cmdWorktreeRemove(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "worktree remove", "--dry-run --worktree <id-or-prefix> | --apply --approval <token> --yes")
	dryRun := fs.Bool("dry-run", false, "issue an exact short-lived removal preview")
	apply := fs.Bool("apply", false, "consume a preview approval")
	worktreeID := fs.String("worktree", "", "worktree identity or unambiguous prefix")
	approval := fs.String("approval", "", "single-use approval from --dry-run")
	yes := fs.Bool("yes", false, "confirm the approved native removal")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *dryRun == *apply {
		return errors.New("choose exactly one of --dry-run or --apply")
	}
	if *dryRun {
		if *worktreeID == "" || *approval != "" || *yes {
			return errors.New("--dry-run requires only --worktree")
		}
		response, err := e.api().WorktreeRemovalPreview(ctx, api.WorktreeRemovalPreviewRequest{WorktreeID: *worktreeID})
		if err != nil {
			return err
		}
		if e.jsonOut {
			return emitJSON(response)
		}
		renderWorktreeRemovalPreview(response)
		return nil
	}
	if *approval == "" || !*yes || *worktreeID != "" {
		return errors.New("--apply requires only --approval and --yes")
	}
	response, err := e.api().WorktreeRemovalApply(ctx, api.WorktreeRemovalApplyRequest{Approval: *approval})
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(response)
	}
	renderWorktreeRemoval(response)
	return nil
}

func cmdWorktreeActions(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "worktree actions", "[flags]")
	var opts api.WorktreeActionOptions
	fs.StringVar(&opts.WorktreeID, "worktree", "", "filter by worktree identity or prefix")
	fs.StringVar(&opts.Result, "result", "", "filter by result")
	fs.IntVar(&opts.Limit, "limit", 50, "maximum actions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("worktree actions accepts flags only")
	}
	response, err := e.api().WorktreeActions(ctx, opts)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(response)
	}
	renderWorktreeActions(response)
	return nil
}
