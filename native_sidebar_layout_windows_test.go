//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

func TestSidebarStatusDoesNotReduceRepositoryPanelHeight(t *testing.T) {
	if os.Getenv("REPOWATCH_RUN_NATIVE_UI_TEST") != "1" {
		t.Skip("set REPOWATCH_RUN_NATIVE_UI_TEST=1 to exercise native controls")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	m, err := newMonitor(filepath.Join(t.TempDir(), "monitor.json"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.state.Repositories = []Repository{{ID: "layout", Platform: "github", Owner: "team", Repo: "demo", Snapshot: &Snapshot{CheckedAt: "2026-11-03T10:30:00Z"}}}
	a := &nativeApp{monitor: m, service: &Service{monitor: m}, model: &repositoryModel{}, settingsPath: filepath.Join(t.TempDir(), "settings.json")}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.window.Dispose()
	defer a.tray.Dispose()
	a.refreshView()
	time.AfterFunc(300*time.Millisecond, func() {
		a.window.Synchronize(func() {
			assertSidebarStatusLayout(t, a, "normal")
			dpi := a.window.DPI()
			win.SetWindowPos(a.window.Handle(), 0, 0, 0, int32(walk.IntFrom96DPI(1050, dpi)), int32(walk.IntFrom96DPI(650, dpi)), win.SWP_NOMOVE|win.SWP_NOZORDER)
			time.AfterFunc(300*time.Millisecond, func() {
				a.window.Synchronize(func() {
					assertSidebarStatusLayout(t, a, "compact")
					a.quitting = true
					_ = a.window.Close()
				})
			})
		})
	})
	a.window.Run()
}

func assertSidebarStatusLayout(t *testing.T, a *nativeApp, mode string) {
	t.Helper()
	var body, center, count, status win.RECT
	for _, item := range []struct {
		name   string
		handle win.HWND
		rect   *win.RECT
	}{
		{"body", a.bodyPanel.Handle(), &body}, {"center", a.centerFrame.Handle(), &center},
		{"count", a.count.Handle(), &count}, {"status", a.status.Handle(), &status},
	} {
		if !win.GetWindowRect(item.handle, item.rect) {
			t.Errorf("%s: cannot inspect %s bounds", mode, item.name)
			return
		}
	}
	if body.Bottom <= body.Top || center.Bottom <= center.Top {
		t.Errorf("%s: layout has no height: body=%+v center=%+v", mode, body, center)
		return
	}
	if count.Right > center.Left || status.Right > center.Left {
		t.Errorf("%s: status is not contained in the left sidebar: count=%+v status=%+v center=%+v", mode, count, status, center)
	}
	if center.Bottom < body.Bottom-4 {
		t.Errorf("%s: repository panel stops above the bottom: center=%+v body=%+v", mode, center, body)
	}
}
