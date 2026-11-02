//go:build windows

package main

import (
	"embed"
	"fmt"
	"runtime"
	"unicode"
	"unsafe"

	"github.com/lxn/win"
)

//go:embed assets/fonts/inter/*.ttf assets/fonts/inter/LICENSE.txt
var bundledFonts embed.FS

// Segoe UI is a safe fallback if Windows rejects the bundled font resources.
var nativeUIFontFamily = "Segoe UI"

// Inter has no CJK glyphs. Select the Windows UI face explicitly
// for CJK text instead of letting GDI fall back to a serif typeface.
const nativeCJKFontFamily = "Microsoft YaHei UI"

func fontFamilyForText(text string) string {
	for _, r := range text {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Bopomofo) {
			return nativeCJKFontFamily
		}
	}
	return nativeUIFontFamily
}

// Fonts are private to this process; the user's Windows font library is not
// changed. The returned function must run after all windows are disposed.
func registerBundledFonts() (func(), error) {
	var handles []win.HANDLE
	cleanup := func() {
		for _, handle := range handles {
			win.RemoveFontMemResourceEx(handle)
		}
		handles = nil
	}
	for _, name := range []string{"Inter-Regular.ttf", "Inter-Bold.ttf"} {
		data, err := bundledFonts.ReadFile("assets/fonts/inter/" + name)
		if err != nil || len(data) == 0 {
			cleanup()
			return nil, fmt.Errorf("font resource %s unavailable", name)
		}
		var count uint32
		handle := win.AddFontMemResourceEx(uintptr(unsafe.Pointer(&data[0])), uint32(len(data)), nil, &count)
		runtime.KeepAlive(data)
		if handle == 0 || count == 0 {
			if handle != 0 {
				win.RemoveFontMemResourceEx(handle)
			}
			cleanup()
			return nil, fmt.Errorf("Windows could not load font %s", name)
		}
		handles = append(handles, handle)
	}
	return cleanup, nil
}
