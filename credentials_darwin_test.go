//go:build darwin && cgo

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The opt-in test must run only with a disposable default keychain; CI creates
// one for the job before setting REPOWATCH_TEST_KEYCHAIN.
func TestKeychainLegacyCompatibilityAndLifecycle(t *testing.T) {
	if os.Getenv("REPOWATCH_TEST_KEYCHAIN") != "1" {
		t.Skip("requires a disposable default keychain")
	}
	secondary := os.Getenv("REPOWATCH_TEST_SECONDARY_KEYCHAIN")
	if secondary == "" {
		t.Fatal("requires a second disposable keychain")
	}
	req := RepoRequest{Platform: "github", Owner: "repowatch-test", Repo: "legacy"}
	other := RepoRequest{Platform: "github", Owner: "repowatch-test", Repo: "other"}
	defer deleteCredential(req)
	defer deleteCredential(other)
	cmd := exec.Command("/usr/bin/security", "add-generic-password", "-A", "-s", keychainService, "-a", credentialName(req), "-w", "legacy-fixture")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create legacy credential: %v: %s", err, output)
	}
	cmd = exec.Command("/usr/bin/security", "add-generic-password", "-A", "-s", keychainService, "-a", credentialName(req), "-w", "other-keychain-fixture", secondary)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create second legacy credential: %v: %s", err, output)
	}
	if got, err := loadCredential(req); err != nil || got != "legacy-fixture" {
		t.Fatalf("read existing credential: got %q, err %v", got, err)
	}
	if err := saveCredential(req, "updated-fixture"); err != nil {
		t.Fatal(err)
	}
	if got, err := loadCredential(req); err != nil || got != "updated-fixture" {
		t.Fatalf("updated credential: got %q, err %v", got, err)
	}
	cmd = exec.Command("/usr/bin/security", "find-generic-password", "-s", keychainService, "-a", credentialName(req), "-w", secondary)
	if output, err := cmd.CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != "other-keychain-fixture" {
		t.Fatalf("second keychain credential changed: %v: %s", err, output)
	}
	if got, err := loadCredential(other); err != nil || got != "" {
		t.Fatalf("other account was not isolated: got %q, err %v", got, err)
	}
	if err := saveCredential(other, "separate-fixture"); err != nil {
		t.Fatal(err)
	}
	if err := deleteCredential(req); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command("/usr/bin/security", "find-generic-password", "-s", keychainService, "-a", credentialName(req), "-w", secondary)
	if output, err := cmd.CombinedOutput(); err != nil || strings.TrimSpace(string(output)) != "other-keychain-fixture" {
		t.Fatalf("second keychain credential was deleted: %v: %s", err, output)
	}
	if err := deleteCredential(req); err != nil {
		t.Fatalf("delete second matching entry: %v", err)
	}
	if err := deleteCredential(req); err != nil {
		t.Fatalf("delete must be idempotent: %v", err)
	}
	if got, err := loadCredential(req); err != nil || got != "" {
		t.Fatalf("deleted credential: got %q, err %v", got, err)
	}
	if got, err := loadCredential(other); err != nil || got != "separate-fixture" {
		t.Fatalf("other credential changed: got %q, err %v", got, err)
	}
}
