//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	dir, err := os.UserConfigDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	writer := &macWriter{output: os.Stdout}
	lock, err := acquireMacInstanceLock(filepath.Join(dir, "RepoWatch", "instance.lock"))
	if err != nil {
		message := "无法锁定监控数据：" + err.Error()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			message = "已有一个 Repo Watch 实例正在运行，请从菜单栏打开。"
		}
		_ = writer.send(macResponse{Event: "fatal", Body: message})
		os.Exit(1)
	}
	defer lock.Close()
	m, err := newMonitor(filepath.Join(dir, "RepoWatch", "monitor.json"), fetchWithCredential, func(title, body string) error {
		return writer.send(macResponse{Event: "notification", Title: title, Body: body})
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Run(ctx, 10*time.Minute)
	runMacBridgeWithWriter(ctx, &Service{monitor: m}, os.Stdin, writer)
}
