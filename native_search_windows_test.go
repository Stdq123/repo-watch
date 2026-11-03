//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/lxn/win"
)

func TestSearchPositionCallbackCentersAndRespectsFlags(t *testing.T) {
	if os.Getenv("REPOWATCH_RUN_NATIVE_UI_TEST") != "1" {
		t.Skip("set REPOWATCH_RUN_NATIVE_UI_TEST=1 to exercise native controls")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if highContrastEnabled() {
		t.Skip("search subclass is disabled in high contrast mode")
	}
	m, err := newMonitor(filepath.Join(t.TempDir(), "monitor.json"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &nativeApp{monitor: m, service: &Service{monitor: m}, model: &repositoryModel{}, settingsPath: filepath.Join(t.TempDir(), "settings.json")}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.window.Dispose()
	defer a.tray.Dispose()
	time.AfterFunc(300*time.Millisecond, func() {
		a.window.Synchronize(func() {
			defer func() { a.quitting = true; _ = a.window.Close() }()
			hwnd := a.search.Handle()
			dc := win.GetDC(hwnd)
			if dc == 0 {
				t.Error("cannot read search font metrics")
				return
			}
			font := win.SendMessage(hwnd, win.WM_GETFONT, 0, 0)
			old := win.SelectObject(dc, win.HGDIOBJ(font))
			var metrics win.TEXTMETRIC
			ok := win.GetTextMetrics(dc, &metrics)
			win.SelectObject(dc, old)
			win.ReleaseDC(hwnd, dc)
			if !ok || metrics.TmHeight <= 0 {
				t.Error("cannot measure search font")
				return
			}
			initial := win.WINDOWPOS{X: 11, Y: 20, Cx: 240, Cy: metrics.TmHeight + 42}
			position := initial
			win.SendMessage(hwnd, win.WM_WINDOWPOSCHANGING, 0, uintptr(unsafe.Pointer(&position)))
			runtime.KeepAlive(&position)
			expectedHeight := metrics.TmHeight + 2
			if position.Cy != expectedHeight || position.Y != initial.Y+(initial.Cy-expectedHeight)/2 || position.X != initial.X || position.Cx != initial.Cx {
				t.Errorf("unexpected centered position: got %+v, original %+v, font height %d", position, initial, metrics.TmHeight)
			}
			for _, flag := range []uint32{win.SWP_NOMOVE, win.SWP_NOSIZE} {
				position = initial
				position.Flags = flag
				win.SendMessage(hwnd, win.WM_WINDOWPOSCHANGING, 0, uintptr(unsafe.Pointer(&position)))
				runtime.KeepAlive(&position)
				if position.Y != initial.Y || position.Cy != initial.Cy {
					t.Errorf("flag %#x unexpectedly moved or resized position: %+v", flag, position)
				}
			}
		})
	})
	a.window.Run()
}
