package daemon_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
)

// The socket is the only interface, so it must actually work over a socket.
func TestAPIRoundTripOverUnixSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := newHarness(t, snapshot(time.Minute, mk(1, 0, "/sbin/launchd", 0), codexRoot(100, 1, time.Second)))
	h.d.ScanNow(ctx)

	server := &api.Server{Backend: h.d, SocketPath: h.paths.Socket}
	if err := server.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx) }()

	client := api.NewClient(h.paths.Socket)
	status, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("Status over the socket: %v", err)
	}
	if status.Mode != "audit" {
		t.Fatalf("mode = %q", status.Mode)
	}

	sess, err := client.Sessions(ctx, api.ListOptions{})
	if err != nil {
		t.Fatalf("Sessions over the socket: %v", err)
	}
	if len(sess.Sessions) != 1 {
		t.Fatalf("got %d sessions over the socket", len(sess.Sessions))
	}

	detail, err := client.Session(ctx, sess.Sessions[0].ShortID)
	if err != nil {
		t.Fatalf("Session by short id: %v", err)
	}
	if detail.Session.SessionID != sess.Sessions[0].SessionID {
		t.Fatal("short-id lookup resolved to the wrong session")
	}

	if _, err := client.Explain(ctx, 100); err != nil {
		t.Fatalf("Explain over the socket: %v", err)
	}
	activity, err := client.Activity(ctx, api.ActivityOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Activity over the socket: %v", err)
	}
	if len(activity.Samples) != 1 {
		t.Fatalf("got %d activity samples over the socket, want 1", len(activity.Samples))
	}
	classifications, err := client.Classifications(ctx, api.ClassificationOptions{Latest: true, Limit: 10})
	if err != nil {
		t.Fatalf("Classifications over the socket: %v", err)
	}
	if len(classifications.Classifications) != 1 || classifications.Classifications[0].State != "unknown" {
		t.Fatalf("initial classification = %+v, want one baseline-unknown result", classifications.Classifications)
	}
	if _, err := client.Session(ctx, "does-not-exist"); err == nil {
		t.Fatal("an unknown session must be an error")
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Serve: %v", err)
	}
}

func TestSecondDaemonCannotTakeTheSocket(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := newHarness(t, snapshot(time.Minute, mk(1, 0, "/sbin/launchd", 0)))
	first := &api.Server{Backend: h.d, SocketPath: h.paths.Socket}
	if err := first.Listen(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- first.Serve(ctx) }()

	second := &api.Server{Backend: h.d, SocketPath: h.paths.Socket}
	if err := second.Listen(); err == nil {
		t.Fatal("a second daemon must not be able to bind the same socket")
	}

	cancel()
	<-done
}

func TestStaleSocketFileIsReclaimed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := newHarness(t, snapshot(time.Minute, mk(1, 0, "/sbin/launchd", 0)))
	first := &api.Server{Backend: h.d, SocketPath: h.paths.Socket}
	if err := first.Listen(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- first.Serve(ctx) }()
	cancel()
	<-done

	// Recreate the file to simulate a crash that left it behind.
	if err := writeFile(h.paths.Socket); err != nil {
		t.Fatal(err)
	}
	second := &api.Server{Backend: h.d, SocketPath: h.paths.Socket}
	if err := second.Listen(); err != nil {
		t.Fatalf("a socket file with nothing behind it must be reclaimed: %v", err)
	}
}

func TestRetentionRunsAndIsAudited(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t, snapshot(time.Minute, mk(1, 0, "/sbin/launchd", 0), codexRoot(100, 1, time.Second)))
	h.d.ScanNow(ctx)
	h.d.RunRetentionNow(ctx)

	metrics, err := h.d.Metrics(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.RetentionRuns != 1 {
		t.Fatalf("retention runs = %d, want 1", metrics.RetentionRuns)
	}
	if metrics.DatabaseBytes <= 0 {
		t.Fatal("metrics must report the database size so growth is observable")
	}
}

func writeFile(path string) error {
	return os.WriteFile(path, nil, 0o600)
}

// ---------------------------------------------------------------------------
// Delivery phase 2: the session graph, end to end.
// ---------------------------------------------------------------------------
