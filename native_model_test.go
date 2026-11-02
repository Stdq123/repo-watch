package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSortRepositoriesByForks(t *testing.T) {
	repos := []Repository{{Repo: "a", Snapshot: &Snapshot{Forks: 1}}, {Repo: "z", Snapshot: &Snapshot{Forks: 20}}, {Repo: "x"}}
	got := sortRepositories(repos, "forks")
	if got[0].Repo != "z" || got[1].Repo != "a" || got[2].Repo != "x" {
		t.Fatalf("unexpected fork ordering: %+v", got)
	}
}

func TestWindowPinSettingPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := savePinSetting(path, true); err != nil {
		t.Fatal(err)
	}
	if !loadPinSetting(path) {
		t.Fatal("pin setting lost")
	}
	if err := savePinSetting(path, false); err != nil {
		t.Fatal(err)
	}
	if loadPinSetting(path) {
		t.Fatal("pin setting was not cleared")
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if loadPinSetting(path) {
		t.Fatal("invalid settings should use unpinned default")
	}
}

func TestParseRepositoryURL(t *testing.T) {
	for _, tt := range []struct {
		input   string
		want    RepoRequest
		invalid bool
	}{
		{"https://github.com/team/project.git", RepoRequest{Platform: "github", Owner: "team", Repo: "project"}, false},
		{"https://codeberg.org/a/b", RepoRequest{Platform: "codeberg", Owner: "a", Repo: "b"}, false},
		{"https://git.example.com/git/team/repo", RepoRequest{Platform: "gitea", BaseURL: "https://git.example.com/git", Owner: "team", Repo: "repo"}, false},
		{"https://user:pass@example.com/a/b", RepoRequest{}, true},
		{"https://github.com/a/b/issues", RepoRequest{}, true},
	} {
		got, err := parseRepositoryURL(tt.input)
		if tt.invalid {
			if err == nil {
				t.Errorf("%s: expected error", tt.input)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: got %+v, %v; want %+v", tt.input, got, err, tt.want)
		}
	}
}

func TestInstanceSocketFallsBackForLongPaths(t *testing.T) {
	got := instanceSocket("", strings.Repeat("a", 120), -1)
	want := filepath.Join("/tmp", "mygo--1-2709ce02d207.sock")
	if got != want {
		t.Fatalf("socket path = %q, want %q", got, want)
	}
}

func TestFilterRepositoriesByTag(t *testing.T) {
	repos := []Repository{{ID: "one", Tags: []string{"工作", "后端"}}, {ID: "two"}, {ID: "three", Tags: []string{"工作"}}}
	if got := filterRepositories(repos, "工作"); !reflect.DeepEqual(got, []Repository{repos[0], repos[2]}) {
		t.Fatalf("filtered: %+v", got)
	}
	if got := filterRepositories(repos, ""); len(got) != 3 {
		t.Fatalf("all: %+v", got)
	}
}

func TestSearchRepositories(t *testing.T) {
	repos := []Repository{{ID: "one", Owner: "Team", Repo: "api", Tags: []string{"后端"}}, {ID: "two", Owner: "other", Repo: "frontend"}}
	if got := searchRepositories(repos, " TEAM/API "); len(got) != 1 || got[0].ID != "one" {
		t.Fatalf("name search: %+v", got)
	}
	if got := searchRepositories(repos, "后端"); len(got) != 1 || got[0].ID != "one" {
		t.Fatalf("tag search: %+v", got)
	}
	if got := searchRepositories(repos, ""); len(got) != 2 {
		t.Fatalf("empty search: %+v", got)
	}
}

func TestRepositorySortDoesNotChangeOriginalAndPutsUnknownDatesLast(t *testing.T) {
	repos := []Repository{
		{ID: "unknown", Owner: "z", Repo: "u"},
		{ID: "older", Owner: "b", Repo: "a", Snapshot: &Snapshot{Stars: 5, PushedAt: "2025-01-01T00:00:00Z"}},
		{ID: "newer", Owner: "a", Repo: "b", Snapshot: &Snapshot{Stars: 100, PushedAt: "2025-03-01T00:00:00+08:00"}},
	}
	byDate := sortRepositories(repos, "updated")
	if byDate[0].ID != "newer" || byDate[1].ID != "older" || byDate[2].ID != "unknown" {
		t.Fatalf("date order: %+v", byDate)
	}
	byStars := sortRepositories(repos, "stars")
	if byStars[0].ID != "newer" || byStars[1].ID != "older" || byStars[2].ID != "unknown" {
		t.Fatalf("star order: %+v", byStars)
	}
	byName := sortRepositories(repos, "name")
	if byName[0].ID != "newer" || byName[2].ID != "unknown" || repos[0].ID != "unknown" {
		t.Fatal("name order or input mutation")
	}
}

func TestCompactCount(t *testing.T) {
	for input, want := range map[int]string{0: "0", 999: "999", 1000: "1k", 12500: "12.5k", 1200000: "1.2m"} {
		if got := compactCount(input); got != want {
			t.Errorf("%d: got %q, want %q", input, got, want)
		}
	}
}

func TestRelativeUpdateTime(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ value, want string }{
		{"", "暂无记录"}, {"invalid", "暂无记录"},
		{"2026-11-02T11:58:00Z", "2 分钟前"},
		{"2026-11-02T09:00:00Z", "3 小时前"},
		{"2026-10-30T12:00:00Z", "3 天前"},
		{"2026-11-03T12:00:00Z", "刚刚"},
	} {
		if got := relativeUpdate(tc.value, now); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.value, got, tc.want)
		}
	}
}

func TestReleaseSummaryUsesReadableFirstContent(t *testing.T) {
	input := "> [!IMPORTANT]\n> \n> **修复**了登录问题。\n\n## 详细信息\n更多内容"
	if got := releaseSummary(input); got != "修复了登录问题。" {
		t.Fatalf("summary: %q", got)
	}
	if got := releaseSummary("# Heading\n\n"); got != "" {
		t.Fatalf("empty summary: %q", got)
	}
	long := strings.Repeat("文", 170)
	if got := []rune(releaseSummary(long)); len(got) != 161 || got[len(got)-1] != '…' {
		t.Fatalf("long summary length: %d", len(got))
	}
}

func TestRepositoryReleasesURL(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"https://github.com/team/project", "https://github.com/team/project/releases"},
		{"https://example.org/git/team/project/?from=app#anchor", "https://example.org/git/team/project/releases"},
		{"javascript:alert(1)", ""}, {"https://user:secret@example.org/team/project", ""}, {"", ""},
	} {
		if got := repositoryReleasesURL(tc.input); got != tc.want {
			t.Errorf("%q => %q, want %q", tc.input, got, tc.want)
		}
	}
}
