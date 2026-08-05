package api

import (
	"context"
	"net/url"
	"strconv"
)

// CacheArtifacts lists cache artifact projections.
func (c *Client) CacheArtifacts(ctx context.Context, opts CacheArtifactOptions) (CacheArtifactsResponse, error) {
	query := url.Values{}
	if opts.Lifecycle != "" {
		query.Set("lifecycle", opts.Lifecycle)
	}
	if opts.Current {
		query.Set("current", "true")
	}
	return get[CacheArtifactsResponse](ctx, c, "/cache/artifacts", query)
}

// CacheArtifact fetches one exact cache artifact.
func (c *Client) CacheArtifact(ctx context.Context, id string) (CacheArtifactResponse, error) {
	return get[CacheArtifactResponse](ctx, c, "/cache/artifacts/"+url.PathEscape(id), nil)
}

// CacheCandidates lists current stale candidates and recommendations.
func (c *Client) CacheCandidates(ctx context.Context) (CacheArtifactsResponse, error) {
	return get[CacheArtifactsResponse](ctx, c, "/cache/candidates", nil)
}

// CacheQuarantines lists durable quarantine records.
func (c *Client) CacheQuarantines(ctx context.Context) (CacheQuarantinesResponse, error) {
	return get[CacheQuarantinesResponse](ctx, c, "/cache/quarantined", nil)
}

// CacheActions lists durable cache action history.
func (c *Client) CacheActions(ctx context.Context, opts CacheActionOptions) (CacheActionsResponse, error) {
	query := url.Values{}
	if opts.ArtifactID != "" {
		query.Set("artifact", opts.ArtifactID)
	}
	if opts.Kind != "" {
		query.Set("kind", opts.Kind)
	}
	if opts.Result != "" {
		query.Set("result", opts.Result)
	}
	if opts.Limit > 0 {
		query.Set("limit", strconv.Itoa(opts.Limit))
	}
	return get[CacheActionsResponse](ctx, c, "/cache/actions", query)
}

func (c *Client) cachePreview(ctx context.Context, action string, request CachePreviewRequest) (CachePreviewResponse, error) {
	return post[CachePreviewRequest, CachePreviewResponse](ctx, c, "/cache/"+action+"/preview", request)
}

func (c *Client) cacheApply(ctx context.Context, action string, request CacheApplyRequest) (CacheApplyResponse, error) {
	return post[CacheApplyRequest, CacheApplyResponse](ctx, c, "/cache/"+action+"/apply", request)
}

func (c *Client) CacheCleanupPreview(ctx context.Context, request CachePreviewRequest) (CachePreviewResponse, error) {
	return c.cachePreview(ctx, "cleanup", request)
}

func (c *Client) CacheCleanupApply(ctx context.Context, request CacheApplyRequest) (CacheApplyResponse, error) {
	return c.cacheApply(ctx, "cleanup", request)
}

func (c *Client) CacheRestorePreview(ctx context.Context, request CachePreviewRequest) (CachePreviewResponse, error) {
	return c.cachePreview(ctx, "restore", request)
}

func (c *Client) CacheRestoreApply(ctx context.Context, request CacheApplyRequest) (CacheApplyResponse, error) {
	return c.cacheApply(ctx, "restore", request)
}

func (c *Client) CachePurgePreview(ctx context.Context, request CachePreviewRequest) (CachePreviewResponse, error) {
	return c.cachePreview(ctx, "purge", request)
}

func (c *Client) CachePurgeApply(ctx context.Context, request CacheApplyRequest) (CachePurgePrepareResponse, error) {
	return post[CacheApplyRequest, CachePurgePrepareResponse](ctx, c, "/cache/purge/apply", request)
}

func (c *Client) CachePurgeComplete(ctx context.Context, request CachePurgeCompleteRequest) (CacheApplyResponse, error) {
	return post[CachePurgeCompleteRequest, CacheApplyResponse](ctx, c, "/cache/purge/complete", request)
}
