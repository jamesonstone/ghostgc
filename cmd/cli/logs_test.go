package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/ghostgc/internal/api"
	"github.com/jamesonstone/ghostgc/internal/storage"
)

func TestLogsFollowByDefaultAndDrainCursorPages(t *testing.T) {
	previousInterval := logPollInterval
	logPollInterval = time.Millisecond
	t.Cleanup(func() { logPollInterval = previousInterval })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requests []api.LogOptions
	responses := []api.LogsResponse{
		{Entries: []storage.AuditRecord{auditEntry(2, "two"), auditEntry(1, "one")}},
		{Entries: []storage.AuditRecord{auditEntry(3, "three"), auditEntry(4, "four")}},
		{Entries: []storage.AuditRecord{auditEntry(5, "five")}},
	}
	e := &env{fetchLogs: func(_ context.Context, opts api.LogOptions) (api.LogsResponse, error) {
		requests = append(requests, opts)
		response := responses[len(requests)-1]
		if len(requests) == len(responses) {
			cancel()
		}
		return response, nil
	}}

	output, code := captureStdout(t, func() int {
		if err := cmdLogs(ctx, e, []string{"--limit", "2", "--kind", "scan", "--subject", "daemon"}); err != nil {
			t.Fatal(err)
		}
		return 0
	})
	if code != 0 {
		t.Fatalf("logs exit code = %d, want 0", code)
	}
	if len(requests) != 3 {
		t.Fatalf("log requests = %d, want initial plus two cursor pages", len(requests))
	}
	assertCursor(t, requests[0].AfterID, nil)
	assertCursor(t, requests[1].AfterID, int64Pointer(2))
	assertCursor(t, requests[2].AfterID, int64Pointer(4))
	for _, request := range requests {
		if request.Limit != 2 || request.Kind != "scan" || request.Subject != "daemon" {
			t.Fatalf("filters changed while following: %+v", request)
		}
		if request.ExcludeKind != "" {
			t.Fatalf("an explicit kind unexpectedly retained the default exclusion: %+v", request)
		}
	}
	assertSummaryOrder(t, output, "one", "two", "three", "four", "five")
}

func TestLogsFollowFlagsCanDisableFollowing(t *testing.T) {
	for _, flag := range []string{"--follow=false", "-f=false"} {
		calls := 0
		var request api.LogOptions
		e := &env{fetchLogs: func(_ context.Context, opts api.LogOptions) (api.LogsResponse, error) {
			calls++
			request = opts
			return api.LogsResponse{}, nil
		}}
		if err := cmdLogs(context.Background(), e, []string{flag}); err != nil {
			t.Fatalf("cmdLogs(%q): %v", flag, err)
		}
		if calls != 1 {
			t.Fatalf("cmdLogs(%q) made %d requests, want one", flag, calls)
		}
		if request.ExcludeKind != "" {
			t.Fatalf("one-shot logs excluded %q, want complete history", request.ExcludeKind)
		}
	}
}

func TestFollowedLogsHideAttributionUnlessRequested(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantExclude string
		wantKind    string
	}{
		{name: "default", wantExclude: "process.attributed"},
		{name: "verbose", args: []string{"--verbose"}},
		{name: "short verbose", args: []string{"-v"}},
		{name: "explicit attribution", args: []string{"--kind", "process.attributed"}, wantKind: "process.attributed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			var request api.LogOptions
			e := &env{fetchLogs: func(_ context.Context, opts api.LogOptions) (api.LogsResponse, error) {
				request = opts
				cancel()
				return api.LogsResponse{}, nil
			}}
			if err := cmdLogs(ctx, e, tt.args); err != nil {
				t.Fatal(err)
			}
			if request.ExcludeKind != tt.wantExclude || request.Kind != tt.wantKind {
				t.Fatalf("log request = %+v, want exclusion %q kind %q", request, tt.wantExclude, tt.wantKind)
			}
		})
	}
}

func TestFollowedJSONIsOneObjectPerLine(t *testing.T) {
	e := &env{jsonOut: true}
	output, _ := captureStdout(t, func() int {
		for _, id := range []int64{1, 2} {
			if err := emitFollowedLogs(e, api.LogsResponse{Entries: []storage.AuditRecord{auditEntry(id, "entry")}}); err != nil {
				t.Fatal(err)
			}
		}
		return 0
	})
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("JSON stream has %d lines, want 2: %q", len(lines), output)
	}
	for _, line := range lines {
		var response api.LogsResponse
		if err := json.Unmarshal([]byte(line), &response); err != nil || len(response.Entries) != 1 {
			t.Fatalf("invalid streamed response %q: %+v, %v", line, response, err)
		}
	}
}

func auditEntry(id int64, summary string) storage.AuditRecord {
	return storage.AuditRecord{ID: id, TsNs: 1, Kind: "scan", Subject: "daemon", Summary: summary}
}

func int64Pointer(value int64) *int64 { return &value }

func assertCursor(t *testing.T, got, want *int64) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("cursor = %v, want %v", got, want)
		}
		return
	}
	if *got != *want {
		t.Fatalf("cursor = %d, want %d", *got, *want)
	}
}

func assertSummaryOrder(t *testing.T, output string, summaries ...string) {
	t.Helper()
	last := -1
	for _, summary := range summaries {
		if count := strings.Count(output, summary); count != 1 {
			t.Fatalf("summary %q appears %d times in %q, want once", summary, count, output)
		}
		index := strings.Index(output, summary)
		if index <= last {
			t.Fatalf("summary %q is missing or out of order in %q", summary, output)
		}
		last = index
	}
}
