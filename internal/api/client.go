package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ErrDaemonUnreachable reports that nothing is listening on the socket.
var ErrDaemonUnreachable = errors.New("the ghostgc daemon is not running")

// Client talks to the daemon over the Unix-domain socket.
type Client struct {
	socket string
	http   *http.Client
}

// NewClient constructs a client for a socket path.
func NewClient(socket string) *Client {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	return &Client{
		socket: socket,
		http: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return dialer.DialContext(ctx, "unix", socket)
				},
			},
		},
	}
}

// Socket returns the socket path in use.
func (c *Client) Socket() string { return c.socket }

func get[T any](ctx context.Context, c *Client, path string, query url.Values) (T, error) {
	var zero T
	u := "http://ghostgc/" + APIVersion + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return zero, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			return zero, fmt.Errorf("%w (socket %s)", ErrDaemonUnreachable, c.socket)
		}
		return zero, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var e ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&e); err == nil && e.Error != "" {
			return zero, errors.New(e.Error)
		}
		return zero, fmt.Errorf("daemon returned %s", resp.Status)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return zero, fmt.Errorf("decoding response from %s: %w", path, err)
	}
	return out, nil
}

func post[Req, Resp any](ctx context.Context, c *Client, path string, body Req) (Resp, error) {
	var zero Resp
	raw, err := json.Marshal(body)
	if err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"http://ghostgc/"+APIVersion+path, bytes.NewReader(raw))
	if err != nil {
		return zero, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			return zero, fmt.Errorf("%w (socket %s)", ErrDaemonUnreachable, c.socket)
		}
		return zero, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var e ErrorResponse
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Error != "" {
			return zero, errors.New(e.Error)
		}
		return zero, fmt.Errorf("daemon returned %s", resp.Status)
	}
	var out Resp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return zero, fmt.Errorf("decoding response from %s: %w", path, err)
	}
	return out, nil
}

// Status fetches daemon status.
func (c *Client) Status(ctx context.Context) (StatusResponse, error) {
	return get[StatusResponse](ctx, c, "/status", nil)
}

// Sessions lists sessions.
func (c *Client) Sessions(ctx context.Context, opts ListOptions) (SessionsResponse, error) {
	return get[SessionsResponse](ctx, c, "/sessions", listQuery(opts))
}

// Session fetches one session.
func (c *Client) Session(ctx context.Context, id string) (SessionDetail, error) {
	return get[SessionDetail](ctx, c, "/sessions/"+url.PathEscape(id), nil)
}

// Processes lists processes.
func (c *Client) Processes(ctx context.Context, opts ListOptions) (ProcessesResponse, error) {
	return get[ProcessesResponse](ctx, c, "/processes", listQuery(opts))
}

// Explain describes one PID.
func (c *Client) Explain(ctx context.Context, pid int) (ExplainResponse, error) {
	return get[ExplainResponse](ctx, c, "/explain", url.Values{"pid": {strconv.Itoa(pid)}})
}

// Candidates lists cleanup candidates.
func (c *Client) Candidates(ctx context.Context) (CandidatesResponse, error) {
	return get[CandidatesResponse](ctx, c, "/candidates", nil)
}

// Policies lists loaded policies.
func (c *Client) Policies(ctx context.Context) (PoliciesResponse, error) {
	return get[PoliciesResponse](ctx, c, "/policies", nil)
}

// CleanupPreview issues a short-lived approval for one exact recommendation.
func (c *Client) CleanupPreview(ctx context.Context, req CleanupPreviewRequest) (CleanupPreviewResponse, error) {
	return post[CleanupPreviewRequest, CleanupPreviewResponse](ctx, c, "/cleanup/preview", req)
}

// CleanupApply consumes one manual approval.
func (c *Client) CleanupApply(ctx context.Context, req CleanupApplyRequest) (CleanupApplyResponse, error) {
	return post[CleanupApplyRequest, CleanupApplyResponse](ctx, c, "/cleanup/apply", req)
}

// Actions lists durable action history.
func (c *Client) Actions(ctx context.Context, opts ActionOptions) (ActionsResponse, error) {
	q := url.Values{}
	if opts.ProcUID != "" {
		q.Set("process", opts.ProcUID)
	}
	if opts.PolicyID != "" {
		q.Set("policy", opts.PolicyID)
	}
	if opts.Result != "" {
		q.Set("result", opts.Result)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	return get[ActionsResponse](ctx, c, "/actions", q)
}

// Logs fetches audit entries.
func (c *Client) Logs(ctx context.Context, opts LogOptions) (LogsResponse, error) {
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Kind != "" {
		q.Set("kind", opts.Kind)
	}
	if opts.Subject != "" {
		q.Set("subject", opts.Subject)
	}
	if opts.SinceNs > 0 {
		q.Set("since_ns", strconv.FormatInt(opts.SinceNs, 10))
	}
	return get[LogsResponse](ctx, c, "/logs", q)
}

// Doctor runs the daemon-side diagnostics.
func (c *Client) Doctor(ctx context.Context) (DoctorResponse, error) {
	return get[DoctorResponse](ctx, c, "/doctor", nil)
}

// Metrics fetches daemon metrics.
func (c *Client) Metrics(ctx context.Context) (MetricsResponse, error) {
	return get[MetricsResponse](ctx, c, "/metrics", nil)
}

// Activity fetches targeted activity evidence.
func (c *Client) Activity(ctx context.Context, opts ActivityOptions) (ActivityResponse, error) {
	q := url.Values{}
	if opts.ProcUID != "" {
		q.Set("process", opts.ProcUID)
	}
	if opts.SessionID != "" {
		q.Set("session", opts.SessionID)
	}
	if opts.SinceNs > 0 {
		q.Set("since_ns", strconv.FormatInt(opts.SinceNs, 10))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	return get[ActivityResponse](ctx, c, "/activity", q)
}

// Classifications fetches deterministic process conclusions.
func (c *Client) Classifications(ctx context.Context, opts ClassificationOptions) (ClassificationsResponse, error) {
	q := url.Values{}
	if opts.ProcUID != "" {
		q.Set("process", opts.ProcUID)
	}
	if opts.SessionID != "" {
		q.Set("session", opts.SessionID)
	}
	if opts.State != "" {
		q.Set("state", opts.State)
	}
	if opts.SinceNs > 0 {
		q.Set("since_ns", strconv.FormatInt(opts.SinceNs, 10))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Latest {
		q.Set("latest", "true")
	}
	return get[ClassificationsResponse](ctx, c, "/classifications", q)
}

func listQuery(opts ListOptions) url.Values {
	q := url.Values{}
	if opts.SessionID != "" {
		q.Set("session", opts.SessionID)
	}
	if opts.AgentID != "" {
		q.Set("agent", opts.AgentID)
	}
	if opts.State != "" {
		q.Set("state", opts.State)
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.All {
		q.Set("all", "true")
	}
	return q
}
