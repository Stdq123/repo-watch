//go:build windows

package main

import (
	"fmt"
	"syscall"

	"github.com/lxn/walk"
	d "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

var passiveListMotionSubclass uintptr

func init() { passiveListMotionSubclass = syscall.NewCallback(passiveListMotionProc) }

// Walk invalidates the row on every WM_MOUSEMOVE, even when the styler does
// not render a hover state. Keep button-held motion for native drag selection.
func passiveListMotionProc(hwnd uintptr, msg uint32, wparam, lparam, _, _ uintptr) uintptr {
	const buttons = win.MK_LBUTTON | win.MK_RBUTTON | win.MK_MBUTTON | win.MK_XBUTTON1 | win.MK_XBUTTON2
	if msg == win.WM_MOUSEMOVE && wparam&buttons == 0 {
		return 0
	}
	if msg == win.WM_NCDESTROY {
		_, _, _ = removeWindowSubclass.Call(hwnd, passiveListMotionSubclass, 4)
	}
	result, _, _ := defSubclassProc.Call(hwnd, uintptr(msg), wparam, lparam)
	return result
}

// Walk's declarative owner-drawn list omits LBS_HASSTRINGS. Set it during
// creation so Windows accessibility APIs and keyboard search get real text.
type accessibleListBox struct{ d.ListBox }

func styledListBox(spec d.ListBox) d.Widget { return accessibleListBox{ListBox: spec} }
func (spec accessibleListBox) Create(builder *d.Builder) error {
	list, err := walk.NewListBoxWithStyle(builder.Parent(), win.LBS_OWNERDRAWVARIABLE|win.LBS_HASSTRINGS)
	if err != nil {
		return err
	}
	if spec.AssignTo != nil {
		*spec.AssignTo = list
	}
	return builder.InitWidget(spec.ListBox, list, func() error {
		if ok, _, err := setWindowSubclass.Call(uintptr(list.Handle()), passiveListMotionSubclass, 4, 0); ok == 0 {
			return fmt.Errorf("设置原生列表重绘行为失败: %w", err)
		}
		list.SetItemStyler(spec.ItemStyler)
		if err := list.SetModel(spec.Model); err != nil {
			return err
		}
		if spec.OnCurrentIndexChanged != nil {
			list.CurrentIndexChanged().Attach(spec.OnCurrentIndexChanged)
		}
		if spec.OnItemActivated != nil {
			list.ItemActivated().Attach(spec.OnItemActivated)
		}
		return nil
	})
}
