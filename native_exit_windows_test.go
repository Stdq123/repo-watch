//go:build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

// Run the real native message loop in a child so a stuck exit is observable.
func TestTrayExitEndsProcess(t *testing.T) {
	if os.Getenv("REPOWATCH_EXIT_TEST_CHILD") == "1" {
		runTrayExitChild(t)
		return
	}
	if os.Getenv("REPOWATCH_RUN_NATIVE_UI_TEST") != "1" {
		t.Skip("set REPOWATCH_RUN_NATIVE_UI_TEST=1 to exercise the interactive Windows tray")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTrayExitEndsProcess$", "-test.v")
	cmd.Env = append(os.Environ(), "REPOWATCH_EXIT_TEST_CHILD=1")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("tray exit did not end the process: %v\n%s", ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("tray exit subprocess failed: %v\n%s", err, output)
	}
}

func runTrayExitChild(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	m, err := newMonitor(filepath.Join(t.TempDir(), "monitor.json"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &nativeApp{monitor: m, service: &Service{monitor: m}, model: &repositoryModel{}, settingsPath: filepath.Join(t.TempDir(), "ui.json")}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.window.Dispose()
	defer a.tray.Dispose()
	a.refreshView()
	var closingStates []bool
	a.window.Closing().Attach(func(_ *bool, _ walk.CloseReason) { closingStates = append(closingStates, a.quitting) })
	a.window.Show()
	hwnd := a.window.Handle()
	go func() {
		time.Sleep(400 * time.Millisecond)
		// First close the window to the tray, then open its actual context menu.
		win.PostMessage(hwnd, win.WM_CLOSE, 0, 0)
		time.Sleep(200 * time.Millisecond)
		win.PostMessage(hwnd, win.WM_APP, 0, win.WM_CONTEXTMENU)
		class, _ := windows.UTF16PtrFromString("#32768")
		for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); {
			menu := win.FindWindow(class, nil)
			var pid uint32
			if menu != 0 {
				win.GetWindowThreadProcessId(menu, &pid)
			}
			if pid == uint32(os.Getpid()) {
				win.PostMessage(menu, win.WM_KEYDOWN, win.VK_UP, 0)
				win.PostMessage(menu, win.WM_KEYDOWN, win.VK_RETURN, 0)
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
	}()
	a.window.Run()
	if len(closingStates) != 2 || closingStates[0] || !closingStates[1] || !a.window.IsDisposed() {
		t.Fatalf("expected close-to-tray followed by full exit, got close states %v and disposed=%t", closingStates, a.window.IsDisposed())
	}
}
