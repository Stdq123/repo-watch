//go:build !windows

package main

import "errors"

func loadCredential(RepoRequest) (string, error) { return "", nil }
func saveCredential(RepoRequest, string) error {
	return errors.New("该平台尚未支持安全凭据存储")
}
func deleteCredential(RepoRequest) error { return nil }
