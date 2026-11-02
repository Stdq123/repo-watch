package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

type macRequest struct {
	ID           int      `json:"id"`
	Method       string   `json:"method"`
	URL          string   `json:"url"`
	RepositoryID string   `json:"repositoryID"`
	Tags         []string `json:"tags"`
	Token        string   `json:"token"`
	Path         string   `json:"path"`
}
type macResponse struct {
	ID     int    `json:"id,omitempty"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
	Event  string `json:"event,omitempty"`
	Title  string `json:"title,omitempty"`
	Body   string `json:"body,omitempty"`
}
type macWriter struct {
	mu     sync.Mutex
	output io.Writer
}

func (w *macWriter) send(response macResponse) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return json.NewEncoder(w.output).Encode(response)
}
func runMacBridge(ctx context.Context, service *Service, input io.Reader, output io.Writer) {
	runMacBridgeWithWriter(ctx, service, input, &macWriter{output: output})
}
func runMacBridgeWithWriter(ctx context.Context, service *Service, input io.Reader, output *macWriter) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		var req macRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			_ = output.send(macResponse{Error: "请求格式无效"})
			continue
		}
		response := macResponse{ID: req.ID}
		result, err := handleMacRequest(service, req)
		if err != nil {
			response.Error = err.Error()
		} else {
			response.Result = result
		}
		if output.send(response) != nil {
			return
		}
	}
}
func handleMacRequest(s *Service, req macRequest) (any, error) {
	find := func() (Repository, error) {
		for _, r := range s.monitor.List() {
			if r.ID == req.RepositoryID {
				return r, nil
			}
		}
		return Repository{}, fmt.Errorf("仓库不存在")
	}
	switch req.Method {
	case "list":
		return s.ListRepositories(), nil
	case "refresh":
		s.RefreshNow()
		return true, nil
	case "add":
		parsed, err := parseRepositoryURL(req.URL)
		if err != nil {
			return nil, err
		}
		r := Repository{ID: credentialName(parsed), Platform: parsed.Platform, BaseURL: parsed.BaseURL, Owner: parsed.Owner, Repo: parsed.Repo, Tags: req.Tags}
		return true, s.AddRepository(r, req.Token)
	case "remove":
		r, err := find()
		if err != nil {
			return nil, err
		}
		return true, s.RemoveRepository(r)
	case "tags":
		return true, s.UpdateRepositoryTags(req.RepositoryID, req.Tags)
	case "path":
		return true, s.monitor.UpdateLocalPath(req.RepositoryID, req.Path)
	case "credential":
		r, err := find()
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(req.Token) == "" {
			err = s.ClearCredential(r.request())
		} else {
			err = s.SaveCredential(r.request(), req.Token)
		}
		if err == nil {
			s.RefreshNow()
		}
		return true, err
	default:
		return nil, fmt.Errorf("未知操作")
	}
}
