package main

import (
	"context"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestBackgroundMonitorBaselineChangesAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	value := Snapshot{Name: "team/demo", Stars: 1, Release: "v1"}
	notices := []string{}
	fetch := func(context.Context, RepoRequest) (Snapshot, error) { return value, nil }
	notify := func(title, body string) error { notices = append(notices, title+":"+body); return nil }
	m, err := newMonitor(path, fetch, notify)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.ImportLegacy([]Repository{{ID: "a", Platform: "github", Owner: "team", Repo: "demo"}}); err != nil {
		t.Fatal(err)
	}
	m.Refresh(context.Background())
	if len(notices) != 0 {
		t.Fatal("first observation must not notify")
	}
	value.Stars = 2
	value.Release = "v2"
	m.Refresh(context.Background())
	if len(notices) != 1 {
		t.Fatalf("expected one notification, got %v", notices)
	}
	m.Refresh(context.Background())
	if len(notices) != 1 {
		t.Fatal("unchanged data notified twice")
	}
	restored, err := newMonitor(path, fetch, notify)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.List()) != 1 || restored.List()[0].Snapshot.Stars != 2 {
		t.Fatal("snapshot was not persisted")
	}
	restored.Refresh(context.Background())
	if len(notices) != 1 {
		t.Fatal("restart lost baseline")
	}
}
func TestBackgroundMonitorRetainsLastSnapshotOnFailure(t *testing.T) {
	fail := false
	fetch := func(context.Context, RepoRequest) (Snapshot, error) {
		if fail {
			return Snapshot{}, fmt.Errorf("unavailable")
		}
		return Snapshot{Name: "a/b", Stars: 10}, nil
	}
	m, err := newMonitor(filepath.Join(t.TempDir(), "state.json"), fetch, func(string, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = m.ImportLegacy([]Repository{{ID: "a", Platform: "github", Owner: "a", Repo: "b"}}); err != nil {
		t.Fatal(err)
	}
	m.Refresh(context.Background())
	fail = true
	m.Refresh(context.Background())
	got := m.List()[0]
	if got.Snapshot == nil || got.Snapshot.Stars != 10 || got.Error == "" {
		t.Fatalf("expected old data and error: %+v", got)
	}
}
func TestMigrationIsIdempotent(t *testing.T) {
	m, err := newMonitor(filepath.Join(t.TempDir(), "state.json"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	repo := Repository{ID: "old", Platform: "github", Owner: "a", Repo: "b"}
	if err = m.ImportLegacy([]Repository{repo}); err != nil {
		t.Fatal(err)
	}
	if err = m.Remove("old"); err != nil {
		t.Fatal(err)
	}
	if err = m.ImportLegacy([]Repository{repo}); err != nil {
		t.Fatal(err)
	}
	if len(m.List()) != 0 {
		t.Fatal("migration restored deleted repository")
	}
}

func TestBackgroundRunnerChecksAfterFirstObservation(t *testing.T) {
	var calls atomic.Int32
	notified := make(chan string, 1)
	m, err := newMonitor(filepath.Join(t.TempDir(), "state.json"), func(context.Context, RepoRequest) (Snapshot, error) {
		n := calls.Add(1)
		return Snapshot{Stars: int(n)}, nil
	}, func(_, body string) error { notified <- body; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = m.ImportLegacy([]Repository{{ID: "x", Platform: "github", Owner: "a", Repo: "b"}}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Run(ctx, 20*time.Millisecond)
	select {
	case <-notified:
	case <-time.After(2 * time.Second):
		t.Fatal("background runner did not check for changes")
	}
}
