package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Repository struct {
	ID       string    `json:"id"`
	Platform string    `json:"platform"`
	BaseURL  string    `json:"baseUrl"`
	Owner    string    `json:"owner"`
	Repo     string    `json:"repo"`
	Snapshot *Snapshot `json:"snapshot"`
	Error    string    `json:"error"`
	Change   string    `json:"change"`
}

func (r Repository) request() RepoRequest {
	return RepoRequest{Platform: r.Platform, BaseURL: r.BaseURL, Owner: r.Owner, Repo: r.Repo}
}

type monitorState struct {
	Migrated     bool         `json:"migrated"`
	Repositories []Repository `json:"repositories"`
}
type monitor struct {
	mu        sync.Mutex
	refreshMu sync.Mutex
	path      string
	wake      chan struct{}
	state     monitorState
	fetch     func(context.Context, RepoRequest) (Snapshot, error)
	notify    func(string, string) error
}

func newMonitor(path string, fetch func(context.Context, RepoRequest) (Snapshot, error), notify func(string, string) error) (*monitor, error) {
	m := &monitor{path: path, fetch: fetch, notify: notify, wake: make(chan struct{}, 1), state: monitorState{Repositories: []Repository{}}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &m.state); err != nil {
		return nil, fmt.Errorf("监控数据无法读取：%w", err)
	}
	if m.state.Repositories == nil {
		m.state.Repositories = []Repository{}
	}
	return m, nil
}
func (m *monitor) persistLocked() error {
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.path), ".repo-watch-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, m.path)
}
func (m *monitor) List() []Repository {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Repository, len(m.state.Repositories))
	copy(out, m.state.Repositories)
	for i := range out {
		if out[i].Snapshot != nil {
			s := *out[i].Snapshot
			out[i].Snapshot = &s
		}
	}
	return out
}
func (m *monitor) NeedsMigration() bool { m.mu.Lock(); defer m.mu.Unlock(); return !m.state.Migrated }
func (m *monitor) ImportLegacy(repos []Repository) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Migrated {
		return nil
	}
	for _, r := range repos {
		if r.ID == "" {
			return fmt.Errorf("旧仓库数据缺少标识")
		}
		if err := validateRequest(r.request()); err != nil {
			return fmt.Errorf("旧仓库数据无效：%w", err)
		}
	}
	old := m.state
	m.state = monitorState{Migrated: true, Repositories: append([]Repository{}, repos...)}
	if err := m.persistLocked(); err != nil {
		m.state = old
		return err
	}
	return nil
}
func (m *monitor) Add(r Repository) error {
	if r.ID == "" {
		return fmt.Errorf("仓库标识为空")
	}
	if err := validateRequest(r.request()); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.state.Repositories {
		if x.ID == r.ID || credentialName(x.request()) == credentialName(r.request()) {
			return fmt.Errorf("这个仓库已经添加")
		}
	}
	old := m.state.Repositories
	m.state.Repositories = append(append([]Repository{}, old...), r)
	if err := m.persistLocked(); err != nil {
		m.state.Repositories = old
		return err
	}
	return nil
}
func (m *monitor) Remove(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old := m.state.Repositories
	next := make([]Repository, 0, len(old))
	found := false
	for _, r := range old {
		if r.ID == id {
			found = true
			continue
		}
		next = append(next, r)
	}
	if !found {
		return fmt.Errorf("仓库不存在")
	}
	m.state.Repositories = next
	if err := m.persistLocked(); err != nil {
		m.state.Repositories = old
		return err
	}
	return nil
}
func diffSnapshot(old, next Snapshot, platform string) string {
	var changes []string
	for _, v := range []struct {
		name string
		a, b int
	}{{"Stars", old.Stars, next.Stars}, {"Forks", old.Forks, next.Forks}, {"Issues", old.Issues, next.Issues}} {
		if d := v.b - v.a; d != 0 {
			changes = append(changes, fmt.Sprintf("%s %+d", v.name, d))
		}
	}
	if next.Release != "" && next.Release != old.Release {
		changes = append(changes, "新版本 "+next.Release)
	}
	if next.PushedAt != "" && next.PushedAt != old.PushedAt {
		if platform == "github" {
			changes = append(changes, "有新推送")
		} else {
			changes = append(changes, "仓库已更新")
		}
	}
	return strings.Join(changes, " · ")
}
func (m *monitor) Refresh(ctx context.Context) {
	if !m.refreshMu.TryLock() {
		return
	}
	defer m.refreshMu.Unlock()
	if m.fetch == nil {
		return
	}
	for _, item := range m.List() {
		if ctx.Err() != nil {
			return
		}
		next, err := m.fetch(ctx, item.request())
		m.mu.Lock()
		index := -1
		for i := range m.state.Repositories {
			if m.state.Repositories[i].ID == item.ID {
				index = i
				break
			}
		}
		if index < 0 {
			m.mu.Unlock()
			continue
		}
		r := &m.state.Repositories[index]
		previous := *r
		var change string
		if err != nil {
			r.Error = "刷新失败：" + err.Error()
		} else {
			if r.Snapshot != nil {
				change = diffSnapshot(*r.Snapshot, next, r.Platform)
			}
			r.Snapshot = &next
			r.Error = ""
			if previous.Snapshot == nil {
				r.Change = "已建立监控基线"
			} else if change == "" {
				r.Change = "本次无变化"
			} else {
				r.Change = change
			}
		}
		if persistErr := m.persistLocked(); persistErr != nil {
			*r = previous
			r.Error = "保存监控结果失败：" + persistErr.Error()
			m.mu.Unlock()
			continue
		}
		m.mu.Unlock()
		if err == nil && change != "" && m.notify != nil {
			if notifyErr := m.notify(item.Owner+"/"+item.Repo+" 有更新", change); notifyErr != nil {
				m.mu.Lock()
				for i := range m.state.Repositories {
					if m.state.Repositories[i].ID == item.ID {
						m.state.Repositories[i].Error = "通知发送失败：" + notifyErr.Error()
						_ = m.persistLocked()
						break
					}
				}
				m.mu.Unlock()
			}
		}
	}
}
func (m *monitor) Run(ctx context.Context, interval time.Duration) {
	go func() {
		m.Refresh(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.Refresh(ctx)
			case <-m.wake:
				m.Refresh(ctx)
			}
		}
	}()
}

func (m *monitor) Trigger() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
