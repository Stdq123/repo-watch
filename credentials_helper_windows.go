//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const helperMarkerPrefix = "@repowatch:"

func allowedCredentialSource(req RepoRequest, marker string) bool {
	switch marker {
	case "@repowatch:gh":
		return req.Platform == "github" && req.BaseURL == ""
	case "@repowatch:tea:codeberg":
		return req.Platform == "codeberg" && req.BaseURL == ""
	case "@repowatch:tea:gitea":
		return req.Platform == "gitea" && strings.EqualFold(strings.TrimRight(req.BaseURL, "/"), "https://gitea.com")
	default:
		return false
	}
}
func parseHelperCredential(output string) (string, error) {
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "password=") {
			token := strings.TrimPrefix(line, "password=")
			if token != "" && !strings.ContainsAny(token, "\r\n \t") {
				return token, nil
			}
			break
		}
	}
	return "", fmt.Errorf("登录工具未返回有效凭据")
}
func resolveCredential(req RepoRequest, stored string) (string, error) {
	if !strings.HasPrefix(stored, helperMarkerPrefix) {
		return stored, nil
	}
	if !allowedCredentialSource(req, stored) {
		return "", fmt.Errorf("凭据来源与仓库平台不匹配")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if stored == "@repowatch:gh" {
		path, err := exec.LookPath("gh.exe")
		if err != nil {
			return "", fmt.Errorf("找不到 GitHub CLI")
		}
		cmd = exec.CommandContext(ctx, path, "auth", "token", "--hostname", "github.com")
	} else {
		login, host := "codeberg", "codeberg.org"
		if stored == "@repowatch:tea:gitea" {
			login, host = "gitea", "gitea.com"
		}
		path := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "tea", "tea.exe")
		if _, err := os.Stat(path); err != nil {
			path, err = exec.LookPath("tea.exe")
			if err != nil {
				return "", fmt.Errorf("找不到 tea 客户端")
			}
		}
		cmd = exec.CommandContext(ctx, path, "logins", "helper", "get", "--login", login)
		cmd.Stdin = strings.NewReader("protocol=https\nhost=" + host + "\n\n")
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("读取已保存的登录失败，请检查 CLI 登录状态")
	}
	if stored != "@repowatch:gh" {
		return parseHelperCredential(string(output))
	}
	token := strings.TrimSpace(string(output))
	if token == "" || strings.ContainsAny(token, "\r\n \t") {
		return "", fmt.Errorf("GitHub CLI 未返回有效凭据")
	}
	return token, nil
}
