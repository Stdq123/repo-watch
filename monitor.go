package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type RepoRequest struct {
	Platform string `json:"platform"`
	BaseURL  string `json:"baseUrl"`
	Owner    string `json:"owner"`
	Repo     string `json:"repo"`
}
type Snapshot struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Stars       int    `json:"stars"`
	Forks       int    `json:"forks"`
	Issues      int    `json:"issues"`
	PushedAt    string `json:"pushedAt"`
	Release     string `json:"release"`
	ReleaseURL  string `json:"releaseUrl"`
	CheckedAt   string `json:"checkedAt"`
}
type Service struct {
	setPinned func(bool)
	monitor   *monitor
}

func (s *Service) SetPinned(pinned bool) {
	if s.setPinned != nil {
		s.setPinned(pinned)
	}
}

func (Service) Fetch(req RepoRequest) (Snapshot, error) {
	return fetchWithCredential(context.Background(), req)
}

func fetchWithCredential(parent context.Context, req RepoRequest) (Snapshot, error) {
	if err := validateRequest(req); err != nil {
		return Snapshot{}, err
	}
	token, err := loadCredential(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("读取本机凭据失败：%w", err)
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	return fetchRepositoryWithToken(ctx, &http.Client{Timeout: 18 * time.Second}, req, token)
}

var validName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func fetchRepository(ctx context.Context, client *http.Client, req RepoRequest) (Snapshot, error) {
	return fetchRepositoryWithToken(ctx, client, req, "")
}
func fetchRepositoryWithToken(ctx context.Context, client *http.Client, req RepoRequest, token string) (Snapshot, error) {
	if !validName.MatchString(req.Owner) || !validName.MatchString(req.Repo) || req.Owner == "." || req.Owner == ".." || req.Repo == "." || req.Repo == ".." {
		return Snapshot{}, fmt.Errorf("仓库名称无效，请填写 owner 和 repo")
	}
	base := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	switch req.Platform {
	case "github":
		if base == "" {
			base = "https://api.github.com"
		}
	case "codeberg":
		if base == "" {
			base = "https://codeberg.org"
		}
	case "gitea":
		if base == "" {
			return Snapshot{}, fmt.Errorf("请填写 Gitea 实例地址")
		}
	default:
		return Snapshot{}, fmt.Errorf("不支持的平台")
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Snapshot{}, fmt.Errorf("实例地址无效")
	}
	if token != "" && u.Scheme != "https" {
		return Snapshot{}, fmt.Errorf("私有仓库令牌只能通过 HTTPS 发送")
	}
	auth := ""
	if token != "" {
		if req.Platform == "github" {
			auth = "Bearer " + token
		} else {
			auth = "token " + token
		}
	}
	clientCopy := *client
	if token != "" {
		clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	prefix := base
	if req.Platform != "github" {
		prefix += "/api/v1"
	}
	endpoint := prefix + "/repos/" + url.PathEscape(req.Owner) + "/" + url.PathEscape(req.Repo)
	var repo struct {
		FullName    string `json:"full_name"`
		Description string `json:"description"`
		HTMLURL     string `json:"html_url"`
		Stargazers  int    `json:"stargazers_count"`
		Stars       int    `json:"stars_count"`
		Forks       int    `json:"forks_count"`
		Issues      int    `json:"open_issues_count"`
		PushedAt    string `json:"pushed_at"`
		UpdatedAt   string `json:"updated_at"`
	}
	if _, err = readJSON(ctx, &clientCopy, endpoint, auth, &repo); err != nil {
		return Snapshot{}, err
	}
	var release struct {
		Tag string `json:"tag_name"`
		URL string `json:"html_url"`
	}
	status, err := readJSON(ctx, &clientCopy, endpoint+"/releases/latest", auth, &release)
	if err != nil && status != http.StatusNotFound {
		return Snapshot{}, fmt.Errorf("读取最新发行版：%w", err)
	}
	if req.Platform != "github" && repo.PushedAt == "" {
		repo.PushedAt = repo.UpdatedAt
	}
	stars := repo.Stars
	if req.Platform == "github" {
		stars = repo.Stargazers
	}
	return Snapshot{Name: repo.FullName, Description: repo.Description, URL: repo.HTMLURL, Stars: stars, Forks: repo.Forks, Issues: repo.Issues, PushedAt: repo.PushedAt, Release: release.Tag, ReleaseURL: release.URL, CheckedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}
func readJSON(ctx context.Context, client *http.Client, address, auth string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "RepoWatch-Desktop/1.0")
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("网络请求失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("服务返回 HTTP %d", resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("响应解析失败：%w", err)
	}
	return resp.StatusCode, nil
}
