//go:build windows

package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestWindowsCredentialRoundTripAndIsolation(t *testing.T) {
	req := RepoRequest{Platform: "gitea", BaseURL: "https://example.invalid", Owner: "repo-watch-test", Repo: fmt.Sprintf("test-%d", time.Now().UnixNano())}
	defer deleteCredential(req)
	if err := saveCredential(req, "temporary-test-token"); err != nil {
		t.Fatal(err)
	}
	got, err := loadCredential(req)
	if err != nil || got != "temporary-test-token" {
		t.Fatalf("credential read failed: %v", err)
	}
	other := req
	other.BaseURL = "https://other.invalid"
	got, err = loadCredential(other)
	if err != nil || got != "" {
		t.Fatalf("credentials crossed host boundary: %v", err)
	}
	if err := deleteCredential(req); err != nil {
		t.Fatal(err)
	}
	got, err = loadCredential(req)
	if err != nil || got != "" {
		t.Fatalf("credential was not deleted: %v", err)
	}
}
func TestCredentialRejectsInsecureInstance(t *testing.T) {
	err := (Service{}).SaveCredential(RepoRequest{Platform: "gitea", BaseURL: "http://example.invalid", Owner: "a", Repo: "b"}, "test")
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("expected HTTPS validation: %v", err)
	}
}

func TestCredentialSourceIsBoundToConfiguredHost(t *testing.T) {
	cases := []struct {
		req     RepoRequest
		marker  string
		allowed bool
	}{
		{RepoRequest{Platform: "github", Owner: "a", Repo: "b"}, "@repowatch:gh", true},
		{RepoRequest{Platform: "codeberg", Owner: "a", Repo: "b"}, "@repowatch:tea:codeberg", true},
		{RepoRequest{Platform: "gitea", BaseURL: "https://gitea.com", Owner: "a", Repo: "b"}, "@repowatch:tea:gitea", true},
		{RepoRequest{Platform: "gitea", BaseURL: "https://attacker.invalid", Owner: "a", Repo: "b"}, "@repowatch:tea:gitea", false},
	}
	for _, tc := range cases {
		if allowedCredentialSource(tc.req, tc.marker) != tc.allowed {
			t.Fatalf("wrong source binding for %+v", tc.req)
		}
	}
}
func TestParseCredentialHelperOutput(t *testing.T) {
	token, err := parseHelperCredential("username=example\npassword=placeholder-token\n\n")
	if err != nil || token != "placeholder-token" {
		t.Fatalf("invalid helper response: %v", err)
	}
	if _, err = parseHelperCredential("username=example\n"); err == nil {
		t.Fatal("missing password accepted")
	}
}
