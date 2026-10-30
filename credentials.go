package main

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"
)

func validateRequest(req RepoRequest) error {
	if !validName.MatchString(req.Owner) || !validName.MatchString(req.Repo) || req.Owner == "." || req.Owner == ".." || req.Repo == "." || req.Repo == ".." {
		return fmt.Errorf("仓库名称无效")
	}
	switch req.Platform {
	case "github", "codeberg":
		if strings.TrimSpace(req.BaseURL) != "" {
			return fmt.Errorf("该平台不支持自定义 API 地址")
		}
	case "gitea":
		u, err := url.Parse(strings.TrimSpace(req.BaseURL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("Gitea 实例地址无效")
		}
	default:
		return fmt.Errorf("不支持的平台")
	}
	return nil
}
func credentialName(req RepoRequest) string {
	identity := req.Platform + "\n" + strings.TrimRight(strings.TrimSpace(req.BaseURL), "/") + "\n" + strings.ToLower(req.Owner) + "\n" + strings.ToLower(req.Repo)
	sum := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("RepoWatch/%x", sum)
}
func (Service) SaveCredential(req RepoRequest, token string) error {
	if err := validateRequest(req); err != nil {
		return err
	}
	if req.Platform == "gitea" && !strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.BaseURL)), "https://") {
		return fmt.Errorf("令牌只能用于 HTTPS 实例")
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, "\r\n \t") {
		return fmt.Errorf("请输入有效的访问令牌")
	}
	if err := saveCredential(req, token); err != nil {
		return fmt.Errorf("保存凭据失败：%w", err)
	}
	return nil
}
func (Service) ClearCredential(req RepoRequest) error {
	if err := validateRequest(req); err != nil {
		return err
	}
	if err := deleteCredential(req); err != nil {
		return fmt.Errorf("删除凭据失败：%w", err)
	}
	return nil
}
