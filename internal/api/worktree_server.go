package api

import (
	"context"
	"net/http"
	"strconv"
)

func (s *Server) registerWorktreeRoutes(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix+"/worktrees", s.handle(func(r *http.Request) (any, error) {
		q := r.URL.Query()
		opts := WorktreeOptions{State: q.Get("state"), Source: q.Get("source")}
		opts.Limit, _ = strconv.Atoi(q.Get("limit"))
		return s.Backend.Worktrees(r.Context(), opts)
	}))
	mux.HandleFunc("GET "+prefix+"/worktrees/{id}", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.Worktree(r.Context(), r.PathValue("id"))
	}))
	register := func(path string,
		preview func(context.Context, WorktreeRemovalPreviewRequest) (WorktreeRemovalPreviewResponse, error),
		apply func(context.Context, WorktreeRemovalApplyRequest) (any, error)) {
		mux.HandleFunc("POST "+prefix+path+"/preview", s.handle(func(r *http.Request) (any, error) {
			var req WorktreeRemovalPreviewRequest
			if err := decodeRequest(r, &req); err != nil {
				return nil, err
			}
			return preview(r.Context(), req)
		}))
		mux.HandleFunc("POST "+prefix+path+"/apply", s.handle(func(r *http.Request) (any, error) {
			var req WorktreeRemovalApplyRequest
			if err := decodeRequest(r, &req); err != nil {
				return nil, err
			}
			return apply(r.Context(), req)
		}))
	}
	register("/worktrees/removal", s.Backend.WorktreeRemovalPreview,
		func(ctx context.Context, req WorktreeRemovalApplyRequest) (any, error) {
			return s.Backend.WorktreeRemovalApply(ctx, req)
		})
	register("/worktrees/restore", s.Backend.WorktreeRestorePreview,
		func(ctx context.Context, req WorktreeRemovalApplyRequest) (any, error) {
			return s.Backend.WorktreeRestoreApply(ctx, req)
		})
	register("/worktrees/purge", s.Backend.WorktreePurgePreview,
		func(ctx context.Context, req WorktreeRemovalApplyRequest) (any, error) {
			return s.Backend.WorktreePurgeApply(ctx, req)
		})
	mux.HandleFunc("POST "+prefix+"/worktrees/purge/complete", s.handle(func(r *http.Request) (any, error) {
		var req WorktreePurgeCompleteRequest
		if err := decodeRequest(r, &req); err != nil {
			return nil, err
		}
		return s.Backend.WorktreePurgeComplete(r.Context(), req)
	}))
	mux.HandleFunc("GET "+prefix+"/worktree-actions", s.handle(func(r *http.Request) (any, error) {
		q := r.URL.Query()
		opts := WorktreeActionOptions{WorktreeID: q.Get("worktree"), Result: q.Get("result")}
		opts.Limit, _ = strconv.Atoi(q.Get("limit"))
		return s.Backend.WorktreeActions(r.Context(), opts)
	}))
}
