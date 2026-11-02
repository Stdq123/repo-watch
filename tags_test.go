package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRepositoryTagsPersistAndSurviveRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	// Old state files do not have a tags field.
	if err := os.WriteFile(path, []byte(`{"migrated":true,"repositories":[{"id":"a","platform":"github","owner":"team","repo":"demo"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := newMonitor(path, func(context.Context, RepoRequest) (Snapshot, error) { return Snapshot{Name: "team/demo"}, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateTags("a", []string{" 工作 ", "后端", "工作", ""}); err != nil {
		t.Fatal(err)
	}
	m.Refresh(context.Background())
	restored, err := newMonitor(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.List()[0].Tags; !reflect.DeepEqual(got, []string{"工作", "后端"}) {
		t.Fatalf("tags after restart: %q", got)
	}
	copy := restored.List()
	copy[0].Tags[0] = "changed"
	if got := restored.List()[0].Tags[0]; got != "工作" {
		t.Fatalf("list exposed internal tags: %q", got)
	}
	if err := restored.UpdateTags("a", nil); err != nil {
		t.Fatal(err)
	}
	if got := restored.List()[0].Tags; len(got) != 0 {
		t.Fatalf("tags not cleared: %q", got)
	}
	if err := restored.UpdateTags("missing", []string{"x"}); err == nil {
		t.Fatal("updating a missing repository succeeded")
	}
}
