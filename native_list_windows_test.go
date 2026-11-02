//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

func TestRepositoryListAccessibleText(t *testing.T) {
	m := &repositoryModel{items: []Repository{{Owner: "team", Repo: "demo", Platform: "github", Tags: []string{"工作"}, Snapshot: &Snapshot{Stars: 42}}}}
	text, ok := m.Value(0).(string)
	if !ok || !strings.Contains(text, "team/demo") || !strings.Contains(text, "工作") || !strings.Contains(text, "42") {
		t.Fatalf("repository list item does not expose its content: %v", m.Value(0))
	}
}

// The repository and sidebar rows do not use a hover style. Mouse movement
// over them should not repeatedly erase and redraw the row.
func TestNativeListMouseMovementDoesNotInvalidateRow(t *testing.T) {
	if os.Getenv("REPOWATCH_RUN_NATIVE_UI_TEST") != "1" {
		t.Skip("set REPOWATCH_RUN_NATIVE_UI_TEST=1 to exercise native controls")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	m, err := newMonitor(filepath.Join(t.TempDir(), "monitor.json"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	description := strings.Repeat("A detailed repository description with readable information. ", 8)
	m.state.Repositories = []Repository{
		{ID: "one", Platform: "github", Owner: "team", Repo: "one", Snapshot: &Snapshot{Description: description}},
		{ID: "two", Platform: "github", Owner: "team", Repo: "two"},
	}
	a := &nativeApp{monitor: m, service: &Service{monitor: m}, model: &repositoryModel{}, settingsPath: filepath.Join(t.TempDir(), "ui.json")}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.window.Dispose()
	defer a.tray.Dispose()
	a.refreshView()
	user32 := windows.NewLazySystemDLL("user32.dll")
	validate := user32.NewProc("ValidateRect")
	getUpdate := user32.NewProc("GetUpdateRect")
	a.window.Synchronize(func() {
		for name, hwnd := range map[string]win.HWND{"repositories": a.table.Handle(), "sidebar": a.filter.Handle()} {
			var row win.RECT
			if result := win.SendMessage(hwnd, win.LB_GETITEMRECT, 0, uintptr(unsafe.Pointer(&row))); int32(result) == win.LB_ERR {
				t.Errorf("%s: could not get first row", name)
				continue
			}
			_, _, _ = validate.Call(uintptr(hwnd), 0)
			win.InvalidateRect(hwnd, &row, true)
			var probe win.RECT
			if updated, _, _ := getUpdate.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&probe)), 0); updated == 0 {
				t.Errorf("%s: could not observe native invalidation", name)
				continue
			}
			_, _, _ = validate.Call(uintptr(hwnd), 0)
			point := uintptr(uint16(row.Left+5)) | uintptr(uint16(row.Top+5))<<16
			if hit := uint32(win.SendMessage(hwnd, win.LB_ITEMFROMPOINT, 0, point)); win.HIWORD(hit) != 0 {
				t.Errorf("%s: pointer is outside row: %+v", name, row)
				continue
			}
			win.SendMessage(hwnd, win.WM_MOUSEMOVE, 0, point)
			var invalid win.RECT
			if updated, _, _ := getUpdate.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&invalid)), 0); updated != 0 {
				t.Errorf("%s: passive pointer movement invalidated a row: %+v", name, invalid)
			}
			_, _, _ = validate.Call(uintptr(hwnd), 0)
			win.SendMessage(hwnd, win.WM_MOUSEMOVE, win.MK_LBUTTON, point)
			if updated, _, _ := getUpdate.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&invalid)), 0); updated == 0 {
				t.Errorf("%s: button-held pointer movement was not forwarded", name)
			}
		}
		_ = a.table.SetCurrentIndex(0)
		_ = a.table.SetFocus()
		win.SendMessage(a.table.Handle(), win.WM_KEYDOWN, win.VK_DOWN, 0)
		if index := a.table.CurrentIndex(); index != 1 {
			t.Errorf("keyboard selection: got index %d, want 1", index)
		}
		var first win.RECT
		win.SendMessage(a.table.Handle(), win.LB_GETITEMRECT, 0, uintptr(unsafe.Pointer(&first)))
		point := uintptr(uint16(first.Left+10)) | uintptr(uint16(first.Top+10))<<16
		win.SendMessage(a.table.Handle(), win.WM_LBUTTONDOWN, win.MK_LBUTTON, point)
		win.SendMessage(a.table.Handle(), win.WM_LBUTTONUP, 0, point)
		if index := a.table.CurrentIndex(); index != 0 {
			t.Errorf("mouse selection: got index %d, want 0", index)
		}
		columns := repositoryColumns(a.table.ClientBoundsPixels().Width, a.table.DPI())
		x := columns[4] + 5
		point = uintptr(uint16(x)) | uintptr(uint16(10))<<16
		win.SendMessage(a.columnHeader.Handle(), win.WM_LBUTTONDOWN, win.MK_LBUTTON, point)
		win.SendMessage(a.columnHeader.Handle(), win.WM_LBUTTONUP, 0, point)
		if a.sortBox.CurrentIndex() != 3 || a.reverseSort {
			t.Errorf("first Forks header click: sort=%d reverse=%t", a.sortBox.CurrentIndex(), a.reverseSort)
		}
		win.SendMessage(a.columnHeader.Handle(), win.WM_LBUTTONDOWN, win.MK_LBUTTON, point)
		win.SendMessage(a.columnHeader.Handle(), win.WM_LBUTTONUP, 0, point)
		if !a.reverseSort {
			t.Error("second Forks header click did not reverse sorting")
		}
		if got := a.details.Text(); got != description {
			t.Error("selected repository description was not retained")
		}
		time.AfterFunc(300*time.Millisecond, func() {
			a.window.Synchronize(func() {
				assertInspectorFillsClient(t, a, "normal")
				dpi := a.window.DPI()
				win.SetWindowPos(a.window.Handle(), 0, 0, 0, int32(walk.IntFrom96DPI(1050, dpi)), int32(walk.IntFrom96DPI(650, dpi)), win.SWP_NOMOVE|win.SWP_NOZORDER)
				time.AfterFunc(300*time.Millisecond, func() {
					a.window.Synchronize(func() {
						assertInspectorFillsClient(t, a, "compact")
						a.quitting = true
						_ = a.window.Close()
					})
				})
			})
		})
	})
	a.window.Run()
}

func assertInspectorFillsClient(t *testing.T, a *nativeApp, mode string) {
	var client win.RECT
	if !win.GetClientRect(a.window.Handle(), &client) {
		t.Errorf("%s: could not get window client bounds", mode)
		return
	}
	bottom := win.POINT{X: client.Right, Y: client.Bottom}
	win.ClientToScreen(a.window.Handle(), &bottom)
	var inspector, firstButton, lastButton, release, description win.RECT
	win.GetWindowRect(a.inspector.Handle(), &inspector)
	win.GetWindowRect(a.openButton.Handle(), &firstButton)
	win.GetWindowRect(a.manageButton.Handle(), &lastButton)
	win.GetWindowRect(a.detailRelease.Handle(), &release)
	win.GetWindowRect(a.details.Handle(), &description)
	if gap := bottom.Y - inspector.Bottom; gap > 2 || gap < 0 {
		t.Errorf("%s: inspector is %d pixels above client bottom", mode, gap)
	}
	if gap := inspector.Bottom - lastButton.Bottom; gap > int32(walk.IntFrom96DPI(24, a.inspector.DPI())) || gap < 0 {
		t.Errorf("%s: last action is %d pixels above inspector bottom", mode, gap)
	}
	if firstButton.Top < release.Bottom {
		t.Errorf("%s: release card overlaps action buttons", mode)
	}
	if gap := firstButton.Top - release.Bottom; gap > int32(walk.IntFrom96DPI(12, a.inspector.DPI())) {
		t.Errorf("%s: excessive space between release card and actions: %d pixels", mode, gap)
	}
	if mode == "normal" && description.Bottom-description.Top <= int32(walk.IntFrom96DPI(100, a.inspector.DPI())) {
		t.Errorf("%s: description did not grow into available space", mode)
	}
}

func TestRepositoryColumnsAdaptToAvailableWidthAndDPI(t *testing.T) {
	wide := repositoryColumns(900, 96)
	if wide[4] <= wide[3] || wide[5] <= wide[4] || wide[6] != 900 {
		t.Fatalf("wide columns: %v", wide)
	}
	for _, tc := range []struct{ width, dpi int }{{480, 96}, {720, 144}} {
		columns := repositoryColumns(tc.width, tc.dpi)
		if columns[1] <= columns[0] || columns[2] <= columns[1] || columns[3] <= columns[2] || columns[4] != columns[3] || columns[5] != columns[4] || columns[6] <= columns[5] {
			t.Fatalf("compact columns for %+v: %v", tc, columns)
		}
	}
}
