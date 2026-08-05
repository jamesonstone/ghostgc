package api

import (
	"net/http"
	"strconv"
)

func (s *Server) registerCacheRoutes(mux *http.ServeMux, prefix string) {
	mux.HandleFunc("GET "+prefix+"/cache/artifacts", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.CacheArtifacts(r.Context(), CacheArtifactOptions{
			Lifecycle: r.URL.Query().Get("lifecycle"), Current: r.URL.Query().Get("current") == "true",
		})
	}))
	mux.HandleFunc("GET "+prefix+"/cache/artifacts/{id}", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.CacheArtifact(r.Context(), r.PathValue("id"))
	}))
	mux.HandleFunc("GET "+prefix+"/cache/candidates", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.CacheCandidates(r.Context())
	}))
	mux.HandleFunc("GET "+prefix+"/cache/quarantined", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.CacheQuarantines(r.Context())
	}))
	mux.HandleFunc("GET "+prefix+"/cache/actions", s.handle(func(r *http.Request) (any, error) {
		opts := CacheActionOptions{
			ArtifactID: r.URL.Query().Get("artifact"), Kind: r.URL.Query().Get("kind"), Result: r.URL.Query().Get("result"),
		}
		opts.Limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
		return s.Backend.CacheActions(r.Context(), opts)
	}))
	registerCacheAction := func(path string,
		preview func(*http.Request, CachePreviewRequest) (any, error),
		apply func(*http.Request, CacheApplyRequest) (any, error)) {
		mux.HandleFunc("POST "+prefix+path+"/preview", s.handle(func(r *http.Request) (any, error) {
			var request CachePreviewRequest
			if err := decodeRequest(r, &request); err != nil {
				return nil, err
			}
			return preview(r, request)
		}))
		mux.HandleFunc("POST "+prefix+path+"/apply", s.handle(func(r *http.Request) (any, error) {
			var request CacheApplyRequest
			if err := decodeRequest(r, &request); err != nil {
				return nil, err
			}
			return apply(r, request)
		}))
	}
	registerCacheAction("/cache/cleanup",
		func(r *http.Request, request CachePreviewRequest) (any, error) {
			return s.Backend.CacheCleanupPreview(r.Context(), request)
		},
		func(r *http.Request, request CacheApplyRequest) (any, error) {
			return s.Backend.CacheCleanupApply(r.Context(), request)
		})
	registerCacheAction("/cache/restore",
		func(r *http.Request, request CachePreviewRequest) (any, error) {
			return s.Backend.CacheRestorePreview(r.Context(), request)
		},
		func(r *http.Request, request CacheApplyRequest) (any, error) {
			return s.Backend.CacheRestoreApply(r.Context(), request)
		})
	registerCacheAction("/cache/purge",
		func(r *http.Request, request CachePreviewRequest) (any, error) {
			return s.Backend.CachePurgePreview(r.Context(), request)
		},
		func(r *http.Request, request CacheApplyRequest) (any, error) {
			return s.Backend.CachePurgeApply(r.Context(), request)
		})
	mux.HandleFunc("POST "+prefix+"/cache/purge/complete", s.handle(func(r *http.Request) (any, error) {
		var request CachePurgeCompleteRequest
		if err := decodeRequest(r, &request); err != nil {
			return nil, err
		}
		return s.Backend.CachePurgeComplete(r.Context(), request)
	}))
}
