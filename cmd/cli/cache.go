package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jamesonstone/ghostgc/internal/api"
)

func cmdCache(ctx context.Context, e *env, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ghostgc cache artifacts|explain|candidates|cleanup|quarantined|restore|purge|actions")
	}
	switch args[0] {
	case "artifacts":
		return cmdCacheArtifacts(ctx, e, args[1:])
	case "explain":
		return cmdCacheExplain(ctx, e, args[1:])
	case "candidates":
		return cmdCacheCandidates(ctx, e, args[1:])
	case "quarantined":
		return cmdCacheQuarantined(ctx, e, args[1:])
	case "actions":
		return cmdCacheActions(ctx, e, args[1:])
	case "cleanup", "restore", "purge":
		return cmdCacheAction(ctx, e, args[0], args[1:])
	default:
		return fmt.Errorf("unknown cache command %q", args[0])
	}
}

func cmdCacheArtifacts(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "cache artifacts", "[--lifecycle <state>] [--current]")
	var opts api.CacheArtifactOptions
	fs.StringVar(&opts.Lifecycle, "lifecycle", "", "filter by lifecycle")
	fs.BoolVar(&opts.Current, "current", false, "show only the newest committed evaluation")
	if err := fs.Parse(args); err != nil {
		return err
	}
	response, err := e.api().CacheArtifacts(ctx, opts)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(response)
	}
	renderCacheArtifacts(response)
	return nil
}

func cmdCacheExplain(ctx context.Context, e *env, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ghostgc cache explain <artifact-id>")
	}
	response, err := e.api().CacheArtifact(ctx, args[0])
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(response)
	}
	renderCacheArtifact(response)
	return nil
}

func cmdCacheCandidates(ctx context.Context, e *env, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: ghostgc cache candidates")
	}
	response, err := e.api().CacheCandidates(ctx)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(response)
	}
	renderCacheArtifacts(response)
	return nil
}

func cmdCacheQuarantined(ctx context.Context, e *env, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: ghostgc cache quarantined")
	}
	response, err := e.api().CacheQuarantines(ctx)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(response)
	}
	renderCacheQuarantines(response)
	return nil
}

func cmdCacheActions(ctx context.Context, e *env, args []string) error {
	fs := newFlagSet(e, "cache actions", "[flags]")
	var opts api.CacheActionOptions
	fs.StringVar(&opts.ArtifactID, "artifact", "", "filter by artifact id")
	fs.StringVar(&opts.Kind, "kind", "", "filter by action kind")
	fs.StringVar(&opts.Result, "result", "", "filter by result")
	fs.IntVar(&opts.Limit, "limit", 50, "maximum actions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	response, err := e.api().CacheActions(ctx, opts)
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(response)
	}
	renderCacheActions(response)
	return nil
}

func cmdCacheAction(ctx context.Context, e *env, action string, args []string) error {
	fs := newFlagSet(e, "cache "+action, "--dry-run --artifact <id> [--policy <id>] | --apply --approval <token> --yes")
	dryRun := fs.Bool("dry-run", false, "issue one exact short-lived preview")
	apply := fs.Bool("apply", false, "consume a preview approval")
	artifactID := fs.String("artifact", "", "exact opaque artifact id")
	policyID := fs.String("policy", "", "exact cache policy id")
	approval := fs.String("approval", "", "single-use approval from --dry-run")
	yes := fs.Bool("yes", false, "confirm the exact approved action")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dryRun == *apply {
		return errors.New("choose exactly one of --dry-run or --apply")
	}
	if *dryRun {
		if *artifactID == "" || *approval != "" || *yes || (action != "restore" && *policyID == "") {
			return errors.New("--dry-run requires --artifact and requires --policy for cleanup or purge")
		}
		response, err := cachePreview(ctx, e.api(), action, api.CachePreviewRequest{ArtifactID: *artifactID, PolicyID: *policyID})
		if err != nil {
			return err
		}
		if e.jsonOut {
			return emitJSON(response)
		}
		renderCachePreview(response)
		return nil
	}
	if *approval == "" || !*yes || *artifactID != "" || *policyID != "" {
		return errors.New("--apply requires only --approval and --yes")
	}
	response, err := cacheApply(ctx, e.api(), action, api.CacheApplyRequest{Approval: *approval})
	if err != nil {
		return err
	}
	if e.jsonOut {
		return emitJSON(response)
	}
	renderCacheResult(response)
	return nil
}

func cachePreview(ctx context.Context, client *api.Client, action string, request api.CachePreviewRequest) (api.CachePreviewResponse, error) {
	switch action {
	case "cleanup":
		return client.CacheCleanupPreview(ctx, request)
	case "restore":
		return client.CacheRestorePreview(ctx, request)
	default:
		return client.CachePurgePreview(ctx, request)
	}
}

func cacheApply(ctx context.Context, client *api.Client, action string, request api.CacheApplyRequest) (api.CacheApplyResponse, error) {
	switch action {
	case "cleanup":
		return client.CacheCleanupApply(ctx, request)
	case "restore":
		return client.CacheRestoreApply(ctx, request)
	default:
		return client.CachePurgeApply(ctx, request)
	}
}
