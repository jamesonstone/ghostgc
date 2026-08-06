package storage

import (
	"context"
	"testing"
)

func TestListAuditAfterIDPagesInInsertionOrder(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	for _, record := range []AuditRecord{
		{TsNs: 30, Kind: "scan", Subject: "daemon", Summary: "first"},
		{TsNs: 10, Kind: "other", Subject: "daemon", Summary: "second"},
		{TsNs: 30, Kind: "scan", Subject: "daemon", Summary: "third"},
	} {
		if err := s.AppendAudit(ctx, record); err != nil {
			t.Fatal(err)
		}
	}

	zero := int64(0)
	firstPage, err := s.ListAudit(ctx, AuditFilter{AfterID: &zero, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(firstPage) != 2 || firstPage[0].ID != 1 || firstPage[1].ID != 2 {
		t.Fatalf("first cursor page = %+v, want IDs 1,2", firstPage)
	}

	after := firstPage[1].ID
	secondPage, err := s.ListAudit(ctx, AuditFilter{AfterID: &after, Kind: "scan", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(secondPage) != 1 || secondPage[0].ID != 3 {
		t.Fatalf("second filtered cursor page = %+v, want ID 3", secondPage)
	}
}

func TestListAuditCanExcludeAttributionNoise(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	for _, kind := range []string{"session.started", "process.attributed", "policy.candidate"} {
		if err := s.AppendAudit(ctx, AuditRecord{TsNs: 1, Kind: kind, Subject: "x", Summary: kind}); err != nil {
			t.Fatal(err)
		}
	}
	zero := int64(0)
	entries, err := s.ListAudit(ctx, AuditFilter{AfterID: &zero, ExcludeKind: "process.attributed", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Kind != "session.started" || entries[1].Kind != "policy.candidate" {
		t.Fatalf("filtered audit = %+v", entries)
	}
}
