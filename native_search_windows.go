//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

var searchEditSubclass uintptr

func init() { searchEditSubclass = syscall.NewCallback(searchEditProc) }

// Walk sizes an EDIT as if it still had its standard border. Once the border
// is removed, Windows draws the text at the top of that oversized client area.
// Keep the actual EDIT just tall enough for its font and center it in the
// space assigned by the layout. It remains a normal editable native control.
func centerNativeSearchEdit(edit *walk.LineEdit) {
	if edit == nil || highContrastEnabled() {
		return
	}
	_, _, _ = setWindowSubclass.Call(uintptr(edit.Handle()), searchEditSubclass, 4, 0)
}

func searchEditProc(hwnd uintptr, msg uint32, wparam, lparam, _, _ uintptr) uintptr {
	if msg == win.WM_WINDOWPOSCHANGING && lparam != 0 {
		// LPARAM points to a Windows-owned WINDOWPOS valid for this message.
		// Copy it into Go memory rather than converting the integer to a Go pointer.
		var pos win.WINDOWPOS
		_, _, _ = nativeMoveMemory.Call(uintptr(unsafe.Pointer(&pos)), lparam, unsafe.Sizeof(pos))
		if pos.Flags&win.SWP_NOSIZE == 0 && pos.Flags&win.SWP_NOMOVE == 0 && pos.Cy > 0 {
			dc := win.GetDC(win.HWND(hwnd))
			if dc != 0 {
				font := win.SendMessage(win.HWND(hwnd), win.WM_GETFONT, 0, 0)
				old := win.SelectObject(dc, win.HGDIOBJ(font))
				var metrics win.TEXTMETRIC
				if win.GetTextMetrics(dc, &metrics) && metrics.TmHeight > 0 {
					height := metrics.TmHeight + 2
					if height < pos.Cy {
						pos.Y += (pos.Cy - height) / 2
						pos.Cy = height
						_, _, _ = nativeMoveMemory.Call(lparam, uintptr(unsafe.Pointer(&pos)), unsafe.Sizeof(pos))
					}
				}
				win.SelectObject(dc, old)
				win.ReleaseDC(win.HWND(hwnd), dc)
			}
		}
	}
	if msg == win.WM_NCDESTROY {
		_, _, _ = removeWindowSubclass.Call(hwnd, searchEditSubclass, 4)
	}
	result, _, _ := defSubclassProc.Call(hwnd, uintptr(msg), wparam, lparam)
	return result
}
