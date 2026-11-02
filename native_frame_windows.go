//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

var titleBarSubclass uintptr
var frameSizeSubclass uintptr
var createRoundRectRegion = windows.NewLazySystemDLL("gdi32.dll").NewProc("CreateRoundRectRgn")
var setWindowRegion = windows.NewLazySystemDLL("user32.dll").NewProc("SetWindowRgn")

func init() {
	titleBarSubclass = syscall.NewCallback(titleBarProc)
	frameSizeSubclass = syscall.NewCallback(frameSizeProc)
}

func roundNativePanel(panel *walk.Composite, radius int) {
	if panel == nil || highContrastEnabled() {
		return
	}
	update := func() {
		if panel.IsDisposed() {
			return
		}
		size := panel.SizePixels()
		if size.Width <= 0 || size.Height <= 0 {
			return
		}
		r := walk.IntFrom96DPI(radius, panel.DPI())
		region, _, _ := createRoundRectRegion.Call(0, 0, uintptr(size.Width+1), uintptr(size.Height+1), uintptr(r), uintptr(r))
		if region == 0 {
			return
		}
		ok, _, _ := setWindowRegion.Call(uintptr(panel.Handle()), region, 1)
		if ok == 0 {
			win.DeleteObject(win.HGDIOBJ(region))
		}
	}
	panel.SizeChanged().Attach(update)
	update()
}

// Keep the standard resizable Win32 frame and system menu, while moving the
// caption into the app's native toolbar to match the reference layout.
func styleNativeFrame(window *walk.MainWindow, titleBar *walk.Composite) {
	if window == nil || titleBar == nil {
		return
	}
	_, _, _ = setWindowSubclass.Call(uintptr(window.Handle()), frameSizeSubclass, 3, 0)
	if highContrastEnabled() {
		return
	}
	ok, _, _ := setWindowSubclass.Call(uintptr(titleBar.Handle()), titleBarSubclass, 2, uintptr(window.Handle()))
	if ok == 0 {
		return
	}
	style := win.GetWindowLong(window.Handle(), win.GWL_STYLE)
	win.SetWindowLong(window.Handle(), win.GWL_STYLE, style&^win.WS_CAPTION)
	win.SetWindowPos(window.Handle(), 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
}

// Walk normally prevents resizing below the expanded panel's minimum. The
// inspector adapts when the height shrinks, so use the actual compact limit.
func frameSizeProc(hwnd uintptr, msg uint32, wparam, lparam, _, _ uintptr) uintptr {
	if msg == win.WM_NCDESTROY {
		_, _, _ = removeWindowSubclass.Call(hwnd, frameSizeSubclass, 3)
	}
	result, _, _ := defSubclassProc.Call(hwnd, uintptr(msg), wparam, lparam)
	if msg == win.WM_GETMINMAXINFO && lparam != 0 {
		var info win.MINMAXINFO
		_, _, _ = nativeMoveMemory.Call(uintptr(unsafe.Pointer(&info)), lparam, unsafe.Sizeof(info))
		dpi := int(win.GetDpiForWindow(win.HWND(hwnd)))
		if dpi <= 0 {
			dpi = 96
		}
		info.PtMinTrackSize.X = int32(walk.IntFrom96DPI(1050, dpi))
		info.PtMinTrackSize.Y = int32(walk.IntFrom96DPI(650, dpi))
		_, _, _ = nativeMoveMemory.Call(lparam, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	}
	return result
}

func titleBarProc(hwnd uintptr, msg uint32, wparam, lparam, _, window uintptr) uintptr {
	switch msg {
	case win.WM_LBUTTONDOWN:
		win.ReleaseCapture()
		win.SendMessage(win.HWND(window), win.WM_NCLBUTTONDOWN, win.HTCAPTION, 0)
		return 0
	case win.WM_LBUTTONDBLCLK:
		if win.IsZoomed(win.HWND(window)) {
			win.ShowWindow(win.HWND(window), win.SW_RESTORE)
		} else {
			win.ShowWindow(win.HWND(window), win.SW_MAXIMIZE)
		}
		return 0
	case win.WM_NCDESTROY:
		_, _, _ = removeWindowSubclass.Call(hwnd, titleBarSubclass, 2)
	}
	value, _, _ := defSubclassProc.Call(hwnd, uintptr(msg), wparam, lparam)
	return value
}
