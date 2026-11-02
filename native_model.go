package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func loadPinSetting(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var settings struct {
		Pinned bool `json:"pinned"`
	}
	return json.Unmarshal(data, &settings) == nil && settings.Pinned
}

// Release cards show a short readable excerpt. The release link remains the
// source for the full Markdown and avoids huge tooltips or accessibility text.
func releaseSummary(notes string) string {
	for _, line := range strings.Split(strings.ReplaceAll(notes, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		for strings.HasPrefix(line, ">") {
			line = strings.TrimSpace(strings.TrimPrefix(line, ">"))
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[!") || line == "---" {
			continue
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "*- "))
		line = strings.ReplaceAll(line, "**", "")
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		characters := []rune(line)
		if len(characters) > 160 {
			return string(characters[:160]) + "…"
		}
		return line
	}
	return ""
}

func repositoryReleasesURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/releases"
	u.RawPath, u.RawQuery, u.Fragment = "", "", ""
	return u.String()
}

func savePinSetting(path string, pinned bool) error {
	data, err := json.Marshal(struct {
		Pinned bool `json:"pinned"`
	}{Pinned: pinned})
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func instanceSocket(runtimeDir, tempDir string, uid int) string {
	sum := sha256.Sum256([]byte("RepoWatch"))
	name := "mygo-" + strconv.Itoa(uid) + "-" + hex.EncodeToString(sum[:6]) + ".sock"
	for _, dir := range []string{runtimeDir, tempDir, "/tmp"} {
		if dir != "" && len(filepath.Join(dir, name)) < 100 {
			return filepath.Join(dir, name)
		}
	}
	return filepath.Join("/tmp", name)
}

func parseRepositoryURL(raw string) (RepoRequest, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return RepoRequest{}, fmt.Errorf("请输入完整的 HTTP(S) 仓库链接")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return RepoRequest{}, fmt.Errorf("仓库链接必须包含 owner 和仓库名")
	}
	req := RepoRequest{Owner: parts[len(parts)-2], Repo: strings.TrimSuffix(parts[len(parts)-1], ".git")}
	switch strings.ToLower(u.Hostname()) {
	case "github.com":
		req.Platform = "github"
	case "codeberg.org":
		req.Platform = "codeberg"
	default:
		req.Platform = "gitea"
		req.BaseURL = u.Scheme + "://" + u.Host
		if len(parts) > 2 {
			req.BaseURL += "/" + strings.Join(parts[:len(parts)-2], "/")
		}
	}
	if req.Platform != "gitea" && len(parts) != 2 {
		return RepoRequest{}, fmt.Errorf("请输入仓库首页链接")
	}
	if err := validateRequest(req); err != nil {
		return RepoRequest{}, err
	}
	return req, nil
}

func filterRepositories(repos []Repository, tag string) []Repository {
	if tag == "" {
		return repos
	}
	var result []Repository
	for _, r := range repos {
		for _, t := range r.Tags {
			if t == tag {
				result = append(result, r)
				break
			}
		}
	}
	return result
}

func searchRepositories(repos []Repository, query string) []Repository {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return repos
	}
	var result []Repository
	for _, r := range repos {
		text := r.Owner + "/" + r.Repo + " " + r.Platform + " " + strings.Join(r.Tags, " ")
		if r.Snapshot != nil {
			text += " " + r.Snapshot.Description
		}
		if strings.Contains(strings.ToLower(text), query) {
			result = append(result, r)
		}
	}
	return result
}

func sortRepositories(repos []Repository, order string) []Repository {
	result := append([]Repository(nil), repos...)
	sort.SliceStable(result, func(i, j int) bool {
		a, b := result[i], result[j]
		switch order {
		case "name":
			return strings.ToLower(a.Owner+"/"+a.Repo) < strings.ToLower(b.Owner+"/"+b.Repo)
		case "stars":
			if (a.Snapshot == nil) != (b.Snapshot == nil) {
				return a.Snapshot != nil
			}
			if a.Snapshot != nil && a.Snapshot.Stars != b.Snapshot.Stars {
				return a.Snapshot.Stars > b.Snapshot.Stars
			}
		case "forks":
			if (a.Snapshot == nil) != (b.Snapshot == nil) {
				return a.Snapshot != nil
			}
			if a.Snapshot != nil && a.Snapshot.Forks != b.Snapshot.Forks {
				return a.Snapshot.Forks > b.Snapshot.Forks
			}
		case "updated":
			var at, bt time.Time
			if a.Snapshot != nil {
				at, _ = time.Parse(time.RFC3339, a.Snapshot.PushedAt)
			}
			if b.Snapshot != nil {
				bt, _ = time.Parse(time.RFC3339, b.Snapshot.PushedAt)
			}
			if !at.Equal(bt) {
				return at.After(bt)
			}
		}
		return strings.ToLower(a.Owner+"/"+a.Repo) < strings.ToLower(b.Owner+"/"+b.Repo)
	})
	return result
}

func compactCount(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	value := float64(n) / 1000
	suffix := "k"
	if n >= 1000000 {
		value = float64(n) / 1000000
		suffix = "m"
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", value), ".0") + suffix
}

func relativeUpdate(value string, now time.Time) string {
	date, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "暂无记录"
	}
	age := now.Sub(date)
	if age < time.Minute {
		return "刚刚"
	}
	if age < time.Hour {
		return fmt.Sprintf("%d 分钟前", int(age/time.Minute))
	}
	if age < 24*time.Hour {
		return fmt.Sprintf("%d 小时前", int(age/time.Hour))
	}
	if age < 30*24*time.Hour {
		return fmt.Sprintf("%d 天前", int(age/(24*time.Hour)))
	}
	return date.Local().Format("2006-01-02")
}
