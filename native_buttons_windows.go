//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

// The underlying controls remain Windows BUTTONs, retaining keyboard and UIA
// behavior. The parent subclasses paint only the specific registered buttons.
type buttonPalette struct {
	label    string
	primary  bool
	danger   bool
	caption  bool
	flat     bool
	dropdown bool
	icon     string
}

func styleNativeSortButton(button *walk.PushButton) {
	styleNativeButton(button, false, false)
	if button == nil || highContrastEnabled() {
		return
	}
	p := nativeButtonPalettes[button.Handle()]
	p.dropdown = true
	nativeButtonPalettes[button.Handle()] = p
}

func styleNativeLinkButton(button *walk.PushButton) {
	styleNativeButton(button, false, false)
	if button == nil || highContrastEnabled() {
		return
	}
	p := nativeButtonPalettes[button.Handle()]
	p.flat = true
	nativeButtonPalettes[button.Handle()] = p
}

func styleNativeButtonIcon(button *walk.PushButton, primary bool, icon string) {
	styleNativeButton(button, primary, false)
	if button == nil || highContrastEnabled() {
		return
	}
	p := nativeButtonPalettes[button.Handle()]
	p.icon = icon
	nativeButtonPalettes[button.Handle()] = p
}

var nativeButtonPalettes = map[win.HWND]buttonPalette{}
var nativeButtonParents = map[win.HWND]bool{}
var commonControls = windows.NewLazySystemDLL("comctl32.dll")
var setWindowSubclass = commonControls.NewProc("SetWindowSubclass")
var removeWindowSubclass = commonControls.NewProc("RemoveWindowSubclass")
var defSubclassProc = commonControls.NewProc("DefSubclassProc")
var gdi32 = windows.NewLazySystemDLL("gdi32.dll")
var createSolidBrush = gdi32.NewProc("CreateSolidBrush")
var createPen = gdi32.NewProc("CreatePen")
var buttonSubclass uintptr
var nativeMoveMemory = windows.NewLazySystemDLL("kernel32.dll").NewProc("RtlMoveMemory")

func init() { buttonSubclass = syscall.NewCallback(buttonParentProc) }

func styleNativeButton(button *walk.PushButton, primary, danger bool) {
	if button == nil || highContrastEnabled() {
		return
	}
	parent := win.GetParent(button.Handle())
	if !nativeButtonParents[parent] {
		ok, _, _ := setWindowSubclass.Call(uintptr(parent), buttonSubclass, 1, 0)
		if ok == 0 {
			return
		}
		nativeButtonParents[parent] = true
	}
	nativeButtonPalettes[button.Handle()] = buttonPalette{label: button.Text(), primary: primary, danger: danger}
	win.SendMessage(button.Handle(), win.BM_SETSTYLE, uintptr(win.BS_OWNERDRAW), 1)
}

func styleCaptionButton(button *walk.PushButton, accessibleName, icon string) {
	if button == nil {
		return
	}
	if highContrastEnabled() {
		button.SetVisible(false)
		return
	}
	styleNativeButton(button, false, false)
	p := nativeButtonPalettes[button.Handle()]
	p.caption = true
	p.icon = icon
	p.label = ""
	nativeButtonPalettes[button.Handle()] = p
	_ = button.Accessibility().SetName(accessibleName)
}

func buttonParentProc(hwnd uintptr, msg uint32, wparam, lparam, _, _ uintptr) uintptr {
	if msg == win.WM_DRAWITEM && lparam != 0 {
		var item win.DRAWITEMSTRUCT
		_, _, _ = nativeMoveMemory.Call(uintptr(unsafe.Pointer(&item)), lparam, unsafe.Sizeof(item))
		if palette, ok := nativeButtonPalettes[item.HwndItem]; ok {
			paintNativeButton(&item, palette)
			return 1
		}
	}
	if msg == win.WM_NCDESTROY {
		for handle := range nativeButtonPalettes {
			if win.GetParent(handle) == win.HWND(hwnd) {
				delete(nativeButtonPalettes, handle)
			}
		}
		delete(nativeButtonParents, win.HWND(hwnd))
		_, _, _ = removeWindowSubclass.Call(hwnd, buttonSubclass, 1)
	}
	value, _, _ := defSubclassProc.Call(hwnd, uintptr(msg), wparam, lparam)
	return value
}

func paintNativeButton(item *win.DRAWITEMSTRUCT, p buttonPalette) {
	// Values below are the documented DRAWITEM state bits for Win32 BUTTONs.
	disabled := item.ItemState&0x0004 != 0
	pressed := item.ItemState&0x0001 != 0
	bg := walk.RGB(255, 255, 255)
	border := walk.RGB(217, 222, 225)
	text := walk.RGB(34, 42, 54)
	if p.flat {
		border = bg
		text = walk.RGB(23, 91, 54)
	}
	if p.caption {
		bg = walk.RGB(248, 249, 249)
		border = bg
		text = walk.RGB(27, 35, 42)
	}
	if p.primary {
		bg = walk.RGB(26, 91, 55)
		border = bg
		text = walk.RGB(255, 255, 255)
	}
	if p.danger {
		text = walk.RGB(160, 68, 61)
	}
	if pressed {
		if p.primary {
			bg = walk.RGB(19, 69, 43)
		} else {
			bg = walk.RGB(235, 243, 234)
		}
	}
	if disabled {
		bg = walk.RGB(247, 249, 246)
		border = walk.RGB(233, 238, 232)
		text = walk.RGB(157, 169, 158)
	}
	brush, _, _ := createSolidBrush.Call(uintptr(bg))
	pen, _, _ := createPen.Call(0, 1, uintptr(border))
	if brush == 0 || pen == 0 {
		if brush != 0 {
			win.DeleteObject(win.HGDIOBJ(brush))
		}
		if pen != 0 {
			win.DeleteObject(win.HGDIOBJ(pen))
		}
		return
	}
	oldBrush := win.SelectObject(item.HDC, win.HGDIOBJ(brush))
	oldPen := win.SelectObject(item.HDC, win.HGDIOBJ(pen))
	r := item.RcItem
	radius := int32(9)
	if p.dropdown {
		radius = (r.Bottom - r.Top) * 14 / 40
	}
	win.RoundRect(item.HDC, r.Left, r.Top, r.Right, r.Bottom, radius, radius)
	win.SelectObject(item.HDC, oldBrush)
	win.SelectObject(item.HDC, oldPen)
	win.DeleteObject(win.HGDIOBJ(brush))
	win.DeleteObject(win.HGDIOBJ(pen))
	oldMode := win.SetBkMode(item.HDC, win.TRANSPARENT)
	oldColor := win.SetTextColor(item.HDC, win.COLORREF(text))
	font := win.SendMessage(item.HwndItem, win.WM_GETFONT, 0, 0)
	var oldFont win.HGDIOBJ
	if font != 0 {
		oldFont = win.SelectObject(item.HDC, win.HGDIOBJ(font))
	}
	ptr, err := windows.UTF16PtrFromString(p.label)
	if err == nil {
		if p.dropdown {
			padding := (r.Bottom - r.Top) * 12 / 40
			size := (r.Bottom - r.Top) * 16 / 40
			drawButtonIcon(item.HDC, "chevron-down", r.Right-padding-size, r.Top+(r.Bottom-r.Top-size)/2, size, text)
			textRect := r
			textRect.Left += padding
			textRect.Right -= padding + size + padding/2
			win.DrawTextEx(item.HDC, ptr, -1, &textRect, win.DT_LEFT|win.DT_VCENTER|win.DT_SINGLELINE|win.DT_END_ELLIPSIS, nil)
		} else if p.icon != "" {
			var measured win.SIZE
			units := windows.StringToUTF16(p.label)
			win.GetTextExtentPoint32(item.HDC, ptr, int32(len(units)-1), &measured)
			size := (r.Bottom - r.Top) * 45 / 100
			gap := size / 2
			if p.label == "" {
				gap = 0
			}
			x := r.Left + (r.Right-r.Left-size-gap-measured.CX)/2
			y := r.Top + (r.Bottom-r.Top-size)/2
			drawButtonIcon(item.HDC, p.icon, x, y, size, text)
			textRect := r
			textRect.Left = x + size + gap
			win.DrawTextEx(item.HDC, ptr, -1, &textRect, win.DT_LEFT|win.DT_VCENTER|win.DT_SINGLELINE, nil)
		} else {
			win.DrawTextEx(item.HDC, ptr, -1, &r, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE, nil)
		}
	}
	if font != 0 {
		win.SelectObject(item.HDC, oldFont)
	}
	win.SetTextColor(item.HDC, oldColor)
	win.SetBkMode(item.HDC, oldMode)
	if item.ItemState&0x0010 != 0 {
		r.Left += 4
		r.Top += 4
		r.Right -= 4
		r.Bottom -= 4
		win.DrawFocusRect(item.HDC, &r)
	}
}

func drawButtonIcon(hdc win.HDC, icon string, x, y, size int32, color walk.Color) {
	assets := map[string]string{
		"add": "lucide/plus", "refresh": "lucide/refresh-cw",
		"open": "lucide/external-link", "release": "lucide/tag",
		"copy": "lucide/copy", "edit": "lucide/pencil",
		"terminal": "lucide/terminal",
		"github":   "simple-icons/github", "gitea": "simple-icons/gitea", "codeberg": "simple-icons/codeberg",
		"sort": "lucide/list-filter", "settings": "lucide/settings",
		"chevron-down": "lucide/chevron-down",
		"minimize":     "lucide/minus", "maximize": "lucide/square", "close": "lucide/x",
	}
	if name, ok := assets[icon]; ok {
		drawNativeIcon(hdc, walk.Rectangle{X: int(x), Y: int(y), Width: int(size), Height: int(size)}, name, color)
	}
}
