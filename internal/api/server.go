package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jamesonstone/ghostgc/internal/storage"
)

// Server exposes the Backend over a Unix-domain socket.
type Server struct {
	Backend    Backend
	SocketPath string
	Logger     *slog.Logger

	listener net.Listener
	srv      *http.Server
}

// ErrAlreadyRunning reports that another daemon holds the socket.
var ErrAlreadyRunning = errors.New("api: another ghostgc daemon is already listening on the socket")

// maxSocketPath is the practical limit for sun_path. Exceeding it fails at
// bind time with nothing more helpful than "invalid argument", so it is
// checked here where the path is still in hand.
const maxSocketPath = 100

// Listen binds the socket.
//
// A socket file left behind by a crashed daemon is removed, but only after
// confirming that nothing answers on it. Unlinking a live daemon's socket
// would leave two daemons observing the same machine.
func (s *Server) Listen() error {
	if n := len(s.SocketPath); n > maxSocketPath {
		return fmt.Errorf("api: socket path is %d bytes, which exceeds the %d byte limit the operating system imposes on unix sockets: %s",
			n, maxSocketPath, s.SocketPath)
	}
	if err := os.MkdirAll(filepath.Dir(s.SocketPath), 0o700); err != nil {
		return fmt.Errorf("api: creating socket directory: %w", err)
	}
	if _, err := os.Stat(s.SocketPath); err == nil {
		conn, dialErr := net.DialTimeout("unix", s.SocketPath, 500*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return ErrAlreadyRunning
		}
		if err := os.Remove(s.SocketPath); err != nil {
			return fmt.Errorf("api: removing stale socket %s: %w", s.SocketPath, err)
		}
	}

	ln, err := net.Listen("unix", s.SocketPath)
	if err != nil {
		return fmt.Errorf("api: listening on %s: %w", s.SocketPath, err)
	}
	// Owner-only: the control interface can start and stop observation.
	if err := os.Chmod(s.SocketPath, 0o600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("api: securing socket: %w", err)
	}
	s.listener = ln
	s.srv = &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return nil
}

// Serve runs until the context is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	if s.listener == nil {
		return errors.New("api: Listen must be called before Serve")
	}
	errCh := make(chan error, 1)
	go func() {
		err := s.srv.Serve(s.listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr := s.srv.Shutdown(shutdownCtx)
		// Wait for Serve to finish closing its UnixListener. UnixListener unlinks
		// the socket on close; returning first would let a caller recreate the
		// path only for the old listener to remove the new file afterward.
		serveErr := <-errCh
		_ = os.Remove(s.SocketPath)
		if shutdownErr != nil {
			return fmt.Errorf("api: shutting down server: %w", shutdownErr)
		}
		return serveErr
	case err := <-errCh:
		_ = os.Remove(s.SocketPath)
		return err
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	p := "/" + APIVersion

	mux.HandleFunc("GET "+p+"/status", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.Status(r.Context())
	}))
	mux.HandleFunc("GET "+p+"/sessions", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.Sessions(r.Context(), listOptions(r))
	}))
	mux.HandleFunc("GET "+p+"/sessions/{id}", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.Session(r.Context(), r.PathValue("id"))
	}))
	mux.HandleFunc("GET "+p+"/processes", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.Processes(r.Context(), listOptions(r))
	}))
	mux.HandleFunc("GET "+p+"/explain", s.handle(func(r *http.Request) (any, error) {
		pid, err := strconv.Atoi(r.URL.Query().Get("pid"))
		if err != nil {
			return nil, badRequest("pid must be an integer")
		}
		return s.Backend.Explain(r.Context(), pid)
	}))
	mux.HandleFunc("GET "+p+"/candidates", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.Candidates(r.Context())
	}))
	mux.HandleFunc("GET "+p+"/policies", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.Policies(r.Context())
	}))
	mux.HandleFunc("POST "+p+"/cleanup/preview", s.handle(func(r *http.Request) (any, error) {
		var req CleanupPreviewRequest
		if err := decodeRequest(r, &req); err != nil {
			return nil, err
		}
		return s.Backend.CleanupPreview(r.Context(), req)
	}))
	mux.HandleFunc("POST "+p+"/cleanup/apply", s.handle(func(r *http.Request) (any, error) {
		var req CleanupApplyRequest
		if err := decodeRequest(r, &req); err != nil {
			return nil, err
		}
		return s.Backend.CleanupApply(r.Context(), req)
	}))
	mux.HandleFunc("GET "+p+"/actions", s.handle(func(r *http.Request) (any, error) {
		q := r.URL.Query()
		opts := ActionOptions{ProcUID: q.Get("process"), PolicyID: q.Get("policy"), Result: q.Get("result")}
		opts.Limit, _ = strconv.Atoi(q.Get("limit"))
		return s.Backend.Actions(r.Context(), opts)
	}))
	mux.HandleFunc("GET "+p+"/logs", s.handle(func(r *http.Request) (any, error) {
		q := r.URL.Query()
		opts := LogOptions{Kind: q.Get("kind"), Subject: q.Get("subject")}
		opts.Limit, _ = strconv.Atoi(q.Get("limit"))
		opts.SinceNs, _ = strconv.ParseInt(q.Get("since_ns"), 10, 64)
		if q.Has("after_id") {
			afterID, err := strconv.ParseInt(q.Get("after_id"), 10, 64)
			if err != nil || afterID < 0 {
				return nil, badRequest("after_id must be a non-negative integer")
			}
			opts.AfterID = &afterID
		}
		return s.Backend.Logs(r.Context(), opts)
	}))
	mux.HandleFunc("GET "+p+"/doctor", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.Doctor(r.Context())
	}))
	mux.HandleFunc("GET "+p+"/metrics", s.handle(func(r *http.Request) (any, error) {
		return s.Backend.Metrics(r.Context())
	}))
	mux.HandleFunc("GET "+p+"/activity", s.handle(func(r *http.Request) (any, error) {
		q := r.URL.Query()
		opts := ActivityOptions{ProcUID: q.Get("process"), SessionID: q.Get("session")}
		opts.Limit, _ = strconv.Atoi(q.Get("limit"))
		opts.SinceNs, _ = strconv.ParseInt(q.Get("since_ns"), 10, 64)
		return s.Backend.Activity(r.Context(), opts)
	}))
	mux.HandleFunc("GET "+p+"/classifications", s.handle(func(r *http.Request) (any, error) {
		q := r.URL.Query()
		opts := ClassificationOptions{ProcUID: q.Get("process"), SessionID: q.Get("session"), State: q.Get("state"), Latest: q.Get("latest") == "true"}
		opts.Limit, _ = strconv.Atoi(q.Get("limit"))
		opts.SinceNs, _ = strconv.ParseInt(q.Get("since_ns"), 10, 64)
		return s.Backend.Classifications(r.Context(), opts)
	}))
	s.registerCacheRoutes(mux, p)
	s.registerWorktreeRoutes(mux, p)

	return mux
}

func listOptions(r *http.Request) ListOptions {
	q := r.URL.Query()
	opts := ListOptions{
		SessionID: q.Get("session"),
		AgentID:   q.Get("agent"),
		State:     q.Get("state"),
		All:       q.Get("all") == "true",
	}
	opts.Limit, _ = strconv.Atoi(q.Get("limit"))
	return opts
}

type statusError struct {
	code int
	msg  string
}

func (e *statusError) Error() string { return e.msg }

func badRequest(msg string) error { return &statusError{code: http.StatusBadRequest, msg: msg} }

func decodeRequest(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return badRequest("invalid JSON request: " + err.Error())
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return badRequest("request body must contain exactly one JSON object")
	}
	return nil
}

func (s *Server) handle(fn func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := fn(r)
		if err != nil {
			code := http.StatusInternalServerError
			var se *statusError
			switch {
			case errors.As(err, &se):
				code = se.code
			case errors.Is(err, storage.ErrNotFound):
				code = http.StatusNotFound
			case errors.Is(err, storage.ErrAmbiguous):
				code = http.StatusConflict
			}
			if s.Logger != nil && code >= 500 {
				s.Logger.Error("api request failed", "path", r.URL.Path, "error", err)
			}
			writeJSON(w, code, ErrorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, body)
	}
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

// SocketPathHint returns a human-readable description for error messages.
func SocketPathHint(path string) string {
	return strings.TrimSpace(path)
}
