//go:build windows

package main

import (
	"errors"
	"github.com/danieljoos/wincred"
)

func loadCredential(req RepoRequest) (string, error) {
	c, err := wincred.GetGenericCredential(credentialName(req))
	if errors.Is(err, wincred.ErrElementNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return resolveCredential(req, string(c.CredentialBlob))
}
func saveCredential(req RepoRequest, token string) error {
	c := wincred.NewGenericCredential(credentialName(req))
	c.CredentialBlob = []byte(token)
	c.Persist = wincred.PersistLocalMachine
	return c.Write()
}
func deleteCredential(req RepoRequest) error {
	err := wincred.NewGenericCredential(credentialName(req)).Delete()
	if errors.Is(err, wincred.ErrElementNotFound) {
		return nil
	}
	return err
}
