package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMacBridgeAddListAndRemove(t *testing.T) {
	m, err := newMonitor(filepath.Join(t.TempDir(), "monitor.json"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	input := strings.NewReader("{\"id\":1,\"method\":\"add\",\"url\":\"https://github.com/example/project\",\"tags\":[\"Go\"]}\n{\"id\":2,\"method\":\"list\"}\n{\"id\":3,\"method\":\"remove\",\"repositoryID\":\"missing\"}\n")
	runMacBridge(context.Background(), &Service{monitor: m}, input, &out)
	dec := json.NewDecoder(&out)
	var a, b, c macResponse
	for _, v := range []*macResponse{&a, &b, &c} {
		if err := dec.Decode(v); err != nil {
			t.Fatal(err)
		}
	}
	if a.Error != "" || b.Error != "" || c.Error == "" {
		t.Fatalf("unexpected responses: %+v %+v %+v", a, b, c)
	}
	data, _ := json.Marshal(b.Result)
	var repos []Repository
	if err := json.Unmarshal(data, &repos); err != nil || len(repos) != 1 || repos[0].Repo != "project" || len(repos[0].Tags) != 1 {
		t.Fatalf("unexpected list %s: %v", data, err)
	}
}

func TestMacBridgeRefreshesAfterCredentialChange(t *testing.T) {
	m, err := newMonitor(filepath.Join(t.TempDir(), "monitor.json"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := Repository{ID: "credential-test", Platform: "github", Owner: "repowatchtest", Repo: fmt.Sprintf("repo%d", time.Now().UnixNano())}
	if err := m.Add(r); err != nil {
		t.Fatal(err)
	}
	if _, err := handleMacRequest(&Service{monitor: m}, macRequest{Method: "credential", RepositoryID: r.ID}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-m.wake:
	default:
		t.Fatal("credential change did not request a refresh")
	}
}

func TestMacBridgeRejectsInvalidRequest(t *testing.T) {
	m, _ := newMonitor(filepath.Join(t.TempDir(), "monitor.json"), nil, nil)
	var out bytes.Buffer
	runMacBridge(context.Background(), &Service{monitor: m}, strings.NewReader("{\"id\":1,\"method\":\"add\",\"url\":\"invalid\"}\n"), &out)
	var response macResponse
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &response); err != nil || response.Error == "" {
		t.Fatalf("response %s: %v", out.String(), err)
	}
	if len(m.List()) != 0 {
		t.Fatal("invalid repository was saved")
	}
}
