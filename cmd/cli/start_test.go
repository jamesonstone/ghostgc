package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/config"
	"github.com/jamesonstone/ghostgc/internal/platform/platformtest"
)

func TestStartOptionsPreserveModeAndLogIntent(t *testing.T) {
	for _, tt := range []struct {
		args     []string
		wantMode config.StartupMode
		wantLogs bool
	}{
		{wantMode: config.StartupAudit},
		{args: []string{"--logs"}, wantMode: config.StartupAudit, wantLogs: true},
		{args: []string{"--mode", "reconcile", "--logs"}, wantMode: config.StartupReconcile, wantLogs: true},
		{args: []string{"--mode", "shadow", "--logs"}, wantMode: config.StartupAudit, wantLogs: true},
		{args: []string{"--mode", "live", "--logs"}, wantMode: config.StartupReconcile, wantLogs: true},
	} {
		got, err := parseStartOptions(&env{}, tt.args)
		if err != nil || got.mode != tt.wantMode || got.logs != tt.wantLogs {
			t.Fatalf("parseStartOptions(%v) = %+v, %v; want mode %q logs=%t",
				tt.args, got, err, tt.wantMode, tt.wantLogs)
		}
	}
	if _, err := parseStartOptions(&env{}, []string{"--mode", "enforce", "--logs"}); err == nil {
		t.Fatal("start accepted an automatic enforcement mode")
	}
}

func TestStartWithLogsWaitsForDaemonAndFollowsDefaultStream(t *testing.T) {
	setStartLogTiming(t, time.Millisecond, time.Second)
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, link := executableFixture(t)
	stubExecutablePath(t, link)
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	paths.Socket = "/tmp/ghostgc-start-logs-test.sock"
	fake := platformtest.New(uint32(os.Getuid()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requests []api.LogOptions
	e := &env{paths: paths, socket: paths.Socket, fetchLogs: func(_ context.Context, opts api.LogOptions) (api.LogsResponse, error) {
		requests = append(requests, opts)
		if len(requests) == 1 {
			return api.LogsResponse{}, api.ErrDaemonUnreachable
		}
		cancel()
		return api.LogsResponse{}, nil
	}}

	_, _ = captureStdout(t, func() int {
		if err := startWithPlatform(ctx, e, fake, startOptions{mode: config.StartupAudit, logs: true}); err != nil {
			t.Fatal(err)
		}
		return 0
	})
	if len(requests) != 2 {
		t.Fatalf("log requests = %d, want one readiness retry", len(requests))
	}
	for _, request := range requests {
		if request.Limit != 50 || request.ExcludeKind != "process.attributed" {
			t.Fatalf("start log request = %+v, want default followed-log filters", request)
		}
	}
	state, err := fake.ServiceStatus(context.Background(), config.ServiceLabel)
	if err != nil || !state.Installed {
		t.Fatalf("service status after cancelling logs = %+v, %v", state, err)
	}
}

func TestStartWithLogsDoesNotFollowAfterInstallFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, link := executableFixture(t)
	stubExecutablePath(t, link)
	paths, err := config.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	e := &env{paths: paths, fetchLogs: func(context.Context, api.LogOptions) (api.LogsResponse, error) {
		calls++
		return api.LogsResponse{}, nil
	}}
	failing := installFailurePlatform{Platform: platformtest.New(uint32(os.Getuid()))}
	err = startWithPlatform(context.Background(), e, failing, startOptions{mode: config.StartupAudit, logs: true})
	if err == nil || calls != 0 {
		t.Fatalf("install failure = %v after %d log calls", err, calls)
	}
}

func TestStartedLogReadinessIsBoundedAndDoesNotHideErrors(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		setStartLogTiming(t, time.Millisecond, 5*time.Millisecond)
		e := &env{fetchLogs: func(context.Context, api.LogOptions) (api.LogsResponse, error) {
			return api.LogsResponse{}, api.ErrDaemonUnreachable
		}}
		err := followStartedLogs(context.Background(), e)
		if !errors.Is(err, api.ErrDaemonUnreachable) || !strings.Contains(err.Error(), "logs did not become available") {
			t.Fatalf("timeout error = %v", err)
		}
	})

	t.Run("unrelated error", func(t *testing.T) {
		setStartLogTiming(t, time.Millisecond, time.Second)
		want := errors.New("log query failed")
		calls := 0
		e := &env{fetchLogs: func(context.Context, api.LogOptions) (api.LogsResponse, error) {
			calls++
			return api.LogsResponse{}, want
		}}
		if err := followStartedLogs(context.Background(), e); !errors.Is(err, want) || calls != 1 {
			t.Fatalf("unrelated error = %v after %d calls", err, calls)
		}
	})

	t.Run("post-readiness error", func(t *testing.T) {
		setStartLogTiming(t, time.Millisecond, time.Second)
		previousPoll := logPollInterval
		logPollInterval = time.Millisecond
		t.Cleanup(func() { logPollInterval = previousPoll })
		calls := 0
		e := &env{fetchLogs: func(context.Context, api.LogOptions) (api.LogsResponse, error) {
			calls++
			if calls == 1 {
				return api.LogsResponse{}, nil
			}
			return api.LogsResponse{}, api.ErrDaemonUnreachable
		}}
		err := followStartedLogs(context.Background(), e)
		if !errors.Is(err, api.ErrDaemonUnreachable) || strings.Contains(err.Error(), "did not become available") || calls != 2 {
			t.Fatalf("post-readiness error = %v after %d calls", err, calls)
		}
	})
}

func setStartLogTiming(t *testing.T, interval, timeout time.Duration) {
	t.Helper()
	previousInterval := startLogRetryInterval
	previousTimeout := startLogReadyTimeout
	startLogRetryInterval = interval
	startLogReadyTimeout = timeout
	t.Cleanup(func() {
		startLogRetryInterval = previousInterval
		startLogReadyTimeout = previousTimeout
	})
}
