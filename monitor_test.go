package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchRepositoryGitHubAndRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/octo/demo":
			fmt.Fprint(w, `{"full_name":"octo/demo","description":"A repo","html_url":"https://github.com/octo/demo","stargazers_count":12,"forks_count":3,"open_issues_count":4,"pushed_at":"2026-01-02T00:00:00Z"}`)
		case "/repos/octo/demo/releases/latest":
			fmt.Fprint(w, `{"tag_name":"v1.0","html_url":"https://github.com/octo/demo/releases/tag/v1.0"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	result, err := fetchRepository(context.Background(), server.Client(), RepoRequest{Platform: "github", BaseURL: server.URL, Owner: "octo", Repo: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stars != 12 || result.Forks != 3 || result.Release != "v1.0" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestFetchRepositoryGiteaWithoutRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/team/project":
			fmt.Fprint(w, `{"full_name":"team/project","stars_count":8,"forks_count":2,"open_issues_count":1}`)
		case "/api/v1/repos/team/project/releases/latest":
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	result, err := fetchRepository(context.Background(), server.Client(), RepoRequest{Platform: "gitea", BaseURL: server.URL, Owner: "team", Repo: "project"})
	if err != nil || result.Stars != 8 || result.Release != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestRejectInvalidRepository(t *testing.T) {
	_, err := fetchRepository(context.Background(), http.DefaultClient, RepoRequest{Platform: "github", Owner: "../x", Repo: "demo"})
	if err == nil || !strings.Contains(err.Error(), "仓库") {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestGiteaUsesRepositoryUpdatedAt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"full_name":"team/project","updated_at":"2026-10-29T03:23:27+08:00"}`)
	}))
	defer server.Close()
	result, err := fetchRepository(context.Background(), server.Client(), RepoRequest{Platform: "gitea", BaseURL: server.URL, Owner: "team", Repo: "project"})
	if err != nil || result.PushedAt != "2026-10-29T03:23:27+08:00" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestPrivateRepositoryUsesPlatformAuthorization(t *testing.T) {
	for _, tc := range []struct{ platform, expected string }{{"github", "Bearer sample-token"}, {"gitea", "token sample-token"}, {"codeberg", "token sample-token"}} {
		t.Run(tc.platform, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Header.Get("Authorization") != tc.expected {
					t.Errorf("missing or wrong authorization")
				}
				if strings.HasSuffix(r.URL.Path, "/releases/latest") {
					fmt.Fprint(w, `{"tag_name":"v1"}`)
					return
				}
				fmt.Fprint(w, `{"full_name":"team/private","stars_count":1,"stargazers_count":1}`)
			}))
			defer server.Close()
			_, err := fetchRepositoryWithToken(context.Background(), server.Client(), RepoRequest{Platform: tc.platform, BaseURL: server.URL, Owner: "team", Repo: "private"}, "sample-token")
			if err != nil || calls != 2 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
func TestTokenRejectedOverHTTP(t *testing.T) {
	_, err := fetchRepositoryWithToken(context.Background(), http.DefaultClient, RepoRequest{Platform: "gitea", BaseURL: "http://127.0.0.1", Owner: "a", Repo: "b"}, "sample-token")
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("expected HTTPS error, got %v", err)
	}
}
func TestCredentialDoesNotFollowRedirect(t *testing.T) {
	targetCalled := false
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalled = true; fmt.Fprint(w, `{}`) }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	_, err := fetchRepositoryWithToken(context.Background(), source.Client(), RepoRequest{Platform: "gitea", BaseURL: source.URL, Owner: "a", Repo: "b"}, "sample-token")
	if err == nil || targetCalled {
		t.Fatalf("redirect must not be followed: targetCalled=%v err=%v", targetCalled, err)
	}
}
