package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/worktree"
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
		return errors.New("usage: ghostgc worktree show <id-or-prefix> | remove|restore|purge [flags] | actions [flags]")
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
		return cmdWorktreeAction(ctx, e, "remove", args[1:])
	case "restore":
		return cmdWorktreeAction(ctx, e, "restore", args[1:])
	case "purge":
		return cmdWorktreeAction(ctx, e, "purge", args[1:])
	case "actions":
		return cmdWorktreeActions(ctx, e, args[1:])
	default:
		return fmt.Errorf("unknown worktree subcommand %q", args[0])
	}
}

func cmdWorktreeAction(ctx context.Context, e *env, action string, args []string) error {
	fs := newFlagSet(e, "worktree "+action, "--dry-run --worktree <id-or-prefix> | --apply --approval <token> --yes [--confirm <full-id>]")
	dryRun := fs.Bool("dry-run", false, "issue an exact short-lived lifecycle preview")
	apply := fs.Bool("apply", false, "consume a preview approval")
	worktreeID := fs.String("worktree", "", "worktree identity or unambiguous prefix")
	approval := fs.String("approval", "", "single-use approval from --dry-run")
	yes := fs.Bool("yes", false, "confirm the approved lifecycle action")
	confirmation := fs.String("confirm", "", "full worktree id required for permanent purge")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *dryRun == *apply {
		return errors.New("choose exactly one of --dry-run or --apply")
	}
	if *dryRun {
		if *worktreeID == "" || *approval != "" || *yes || *confirmation != "" {
			return errors.New("--dry-run requires only --worktree")
		}
		response, err := worktreePreview(ctx, e.api(), action, api.WorktreeRemovalPreviewRequest{WorktreeID: *worktreeID})
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
		return errors.New("--apply requires --approval and --yes")
	}
	if action == "purge" && *confirmation == "" {
		return errors.New("purge --apply also requires --confirm with the full worktree id")
	}
	if action != "purge" && *confirmation != "" {
		return errors.New("--confirm is accepted only for permanent purge")
	}
	response, err := worktreeApply(ctx, e, action, api.WorktreeRemovalApplyRequest{Approval: *approval, Confirmation: *confirmation})
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(response)
	}
	renderWorktreeRemoval(response)
	return nil
}

func worktreePreview(ctx context.Context, client *api.Client, action string,
	req api.WorktreeRemovalPreviewRequest) (api.WorktreeRemovalPreviewResponse, error) {
	switch action {
	case "restore":
		return client.WorktreeRestorePreview(ctx, req)
	case "purge":
		return client.WorktreePurgePreview(ctx, req)
	default:
		return client.WorktreeRemovalPreview(ctx, req)
	}
}

func worktreeApply(ctx context.Context, e *env, action string,
	req api.WorktreeRemovalApplyRequest) (api.WorktreeRemovalApplyResponse, error) {
	switch action {
	case "restore":
		return e.api().WorktreeRestoreApply(ctx, req)
	case "purge":
		return executeWorktreePurge(ctx, e, req)
	default:
		return e.api().WorktreeRemovalApply(ctx, req)
	}
}

func executeWorktreePurge(ctx context.Context, e *env,
	req api.WorktreeRemovalApplyRequest) (api.WorktreeRemovalApplyResponse, error) {
	prepared, err := e.api().WorktreePurgeApply(ctx, req)
	if err != nil || prepared.Action.Result != "purging" {
		return prepared.Action, err
	}
	if time.Now().UnixNano() >= prepared.Plan.ExpiresNs {
		return e.api().WorktreePurgeComplete(ctx, api.WorktreePurgeCompleteRequest{
			ActionID: prepared.Plan.ActionID, Completion: prepared.Plan.Completion,
			ExecutionError: "foreground worktree purge plan expired before execution",
		})
	}
	finalizer, err := worktree.NewFinalizer(filepath.Join(e.paths.StateDir, "git-exec"))
	if err != nil {
		return e.api().WorktreePurgeComplete(ctx, api.WorktreePurgeCompleteRequest{
			ActionID: prepared.Plan.ActionID, Completion: prepared.Plan.Completion,
			ExecutionError: "foreground finalizer unavailable: " + err.Error(),
		})
	}
	executionErr := finalizer.Finalize(ctx, prepared.Plan.PrimaryPath, prepared.Plan.RetiredPath,
		prepared.Plan.GitIdentity, prepared.Plan.PathIdentity, prepared.Plan.ApprovedLinks)
	errText := ""
	if executionErr != nil {
		errText = executionErr.Error()
	}
	result, completeErr := e.api().WorktreePurgeComplete(ctx, api.WorktreePurgeCompleteRequest{
		ActionID: prepared.Plan.ActionID, Completion: prepared.Plan.Completion, ExecutionError: errText,
	})
	if completeErr != nil {
		return api.WorktreeRemovalApplyResponse{}, errors.Join(executionErr, completeErr)
	}
	return result, nil
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
