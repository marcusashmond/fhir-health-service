package repository

import (
	"testing"
	"time"

	"github.com/marcusashmond/fhir-health-service/internal/models"
)

func TestPostgresAuditLogRepository_InsertAndList(t *testing.T) {
	pool := newTestDB(t)
	repo := NewPostgresAuditLogRepository(pool)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	entries := []*models.AuditLog{
		{Timestamp: base, Method: "GET", Path: "/Patient/p1", ResourceType: "Patient", ResourceID: "p1", StatusCode: 200, SourceIP: "10.0.0.5"},
		{Timestamp: base.Add(time.Second), Method: "POST", Path: "/Observation", ResourceType: "Observation", StatusCode: 201, SourceIP: "2001:db8::1"},
		{Timestamp: base.Add(2 * time.Second), Method: "GET", Path: "/audit-logs", ResourceType: "audit-logs", StatusCode: 200},
	}
	for _, e := range entries {
		if err := repo.Insert(e); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	got, err := repo.List(10, 0)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("List() returned %d entries, want %d", len(got), len(entries))
	}

	// List returns newest first.
	for i, want := range entries {
		g := got[len(got)-1-i]
		if g.ID == 0 {
			t.Errorf("entry %d: ID not assigned", i)
		}
		if !g.Timestamp.Equal(want.Timestamp) {
			t.Errorf("entry %d: Timestamp = %v, want %v", i, g.Timestamp, want.Timestamp)
		}
		if g.Method != want.Method || g.Path != want.Path || g.ResourceType != want.ResourceType ||
			g.ResourceID != want.ResourceID || g.StatusCode != want.StatusCode || g.SourceIP != want.SourceIP {
			t.Errorf("entry %d = %+v, want %+v", i, g, want)
		}
	}
}

func TestPostgresAuditLogRepository_ListPagination(t *testing.T) {
	pool := newTestDB(t)
	repo := NewPostgresAuditLogRepository(pool)

	for i := 0; i < 5; i++ {
		if err := repo.Insert(&models.AuditLog{Timestamp: time.Now().UTC(), Method: "GET", Path: "/Patient", StatusCode: 200, SourceIP: "10.0.0.5"}); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	page, err := repo.List(2, 1)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("List(2, 1) returned %d entries, want 2", len(page))
	}
	if page[0].ID <= page[1].ID {
		t.Errorf("List() not ordered newest first: IDs %d, %d", page[0].ID, page[1].ID)
	}
}
