package api

import (
	"context"
	"net/url"
	"strconv"
)

// Worktrees lists local worktree inventory.
func (c *Client) Worktrees(ctx context.Context, opts WorktreeOptions) (WorktreesResponse, error) {
	q := url.Values{}
	if opts.State != "" {
		q.Set("state", opts.State)
	}
	if opts.Source != "" {
		q.Set("source", opts.Source)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	return get[WorktreesResponse](ctx, c, "/worktrees", q)
}

// Worktree fetches one identity or unambiguous prefix.
func (c *Client) Worktree(ctx context.Context, id string) (WorktreeView, error) {
	return get[WorktreeView](ctx, c, "/worktrees/"+url.PathEscape(id), nil)
}

// WorktreeRemovalPreview requests short-lived manual authority.
func (c *Client) WorktreeRemovalPreview(ctx context.Context, req WorktreeRemovalPreviewRequest) (WorktreeRemovalPreviewResponse, error) {
	return post[WorktreeRemovalPreviewRequest, WorktreeRemovalPreviewResponse](ctx, c, "/worktrees/removal/preview", req)
}

// WorktreeRemovalApply consumes one manual approval.
func (c *Client) WorktreeRemovalApply(ctx context.Context, req WorktreeRemovalApplyRequest) (WorktreeRemovalApplyResponse, error) {
	return post[WorktreeRemovalApplyRequest, WorktreeRemovalApplyResponse](ctx, c, "/worktrees/removal/apply", req)
}

func (c *Client) WorktreeRestorePreview(ctx context.Context, req WorktreeRemovalPreviewRequest) (WorktreeRemovalPreviewResponse, error) {
	return post[WorktreeRemovalPreviewRequest, WorktreeRemovalPreviewResponse](ctx, c, "/worktrees/restore/preview", req)
}

func (c *Client) WorktreeRestoreApply(ctx context.Context, req WorktreeRemovalApplyRequest) (WorktreeRemovalApplyResponse, error) {
	return post[WorktreeRemovalApplyRequest, WorktreeRemovalApplyResponse](ctx, c, "/worktrees/restore/apply", req)
}

func (c *Client) WorktreePurgePreview(ctx context.Context, req WorktreeRemovalPreviewRequest) (WorktreeRemovalPreviewResponse, error) {
	return post[WorktreeRemovalPreviewRequest, WorktreeRemovalPreviewResponse](ctx, c, "/worktrees/purge/preview", req)
}

func (c *Client) WorktreePurgeApply(ctx context.Context, req WorktreeRemovalApplyRequest) (WorktreePurgePrepareResponse, error) {
	return post[WorktreeRemovalApplyRequest, WorktreePurgePrepareResponse](ctx, c, "/worktrees/purge/apply", req)
}

func (c *Client) WorktreePurgeComplete(ctx context.Context, req WorktreePurgeCompleteRequest) (WorktreeRemovalApplyResponse, error) {
	return post[WorktreePurgeCompleteRequest, WorktreeRemovalApplyResponse](ctx, c, "/worktrees/purge/complete", req)
}

// WorktreeActions lists durable removal history.
func (c *Client) WorktreeActions(ctx context.Context, opts WorktreeActionOptions) (WorktreeActionsResponse, error) {
	q := url.Values{}
	if opts.WorktreeID != "" {
		q.Set("worktree", opts.WorktreeID)
	}
	if opts.Result != "" {
		q.Set("result", opts.Result)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	return get[WorktreeActionsResponse](ctx, c, "/worktree-actions", q)
}
