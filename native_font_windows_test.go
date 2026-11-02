//go:build windows

package main

import (
	"testing"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/sys/windows"
)

func TestFontSelectionUsesChineseFontForCJK(t *testing.T) {
	previous := nativeUIFontFamily
	nativeUIFontFamily = "Inter"
	defer func() { nativeUIFontFamily = previous }()
	for _, tc := range []struct{ text, want string }{
		{"Repo Watch 123", "Inter"}, {"Go · 51.2k", "Inter"},
		{"仓库", "Microsoft YaHei UI"}, {"修复 API 问题", "Microsoft YaHei UI"}, {"日本語", "Microsoft YaHei UI"},
	} {
		if got := fontFamilyForText(tc.text); got != tc.want {
			t.Errorf("%q: got %q want %q", tc.text, got, tc.want)
		}
	}
}

func TestBundledUIFontHasLatinGlyphsAndRequiresChineseFallback(t *testing.T) {
	for _, name := range []string{"Inter-Regular.ttf", "Inter-Bold.ttf"} {
		data, err := bundledFonts.ReadFile("assets/fonts/inter/" + name)
		if err != nil {
			t.Fatal(err)
		}
		font, err := sfnt.Parse(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var buffer sfnt.Buffer
		family, err := font.Name(&buffer, sfnt.NameIDFamily)
		if err != nil || family != "Inter" {
			t.Fatalf("%s family=%q err=%v", name, family, err)
		}
		for _, char := range []rune("Repo Watch0123456789") {
			index, err := font.GlyphIndex(&buffer, char)
			if err != nil || index == 0 {
				t.Errorf("%s lacks %q: index=%v err=%v", name, char, index, err)
			}
		}
		if index, err := font.GlyphIndex(&buffer, '中'); err != nil || index != 0 {
			t.Fatalf("unexpected Chinese glyph index %v, err=%v", index, err)
		}
	}
}

func TestWindowsCanSelectBundledUIFont(t *testing.T) {
	cleanup, err := registerBundledFonts()
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	var logical win.LOGFONT
	logical.LfHeight = -16
	copy(logical.LfFaceName[:], windows.StringToUTF16("Inter"))
	hfont := win.CreateFontIndirect(&logical)
	if hfont == 0 {
		t.Fatal("CreateFontIndirect failed")
	}
	defer win.DeleteObject(win.HGDIOBJ(hfont))
	hdc := win.GetDC(0)
	if hdc == 0 {
		t.Fatal("GetDC failed")
	}
	defer win.ReleaseDC(0, hdc)
	old := win.SelectObject(hdc, win.HGDIOBJ(hfont))
	defer win.SelectObject(hdc, old)
	var actual [64]uint16
	proc := windows.NewLazySystemDLL("gdi32.dll").NewProc("GetTextFaceW")
	n, _, _ := proc.Call(uintptr(hdc), uintptr(len(actual)), uintptr(unsafe.Pointer(&actual[0])))
	if n == 0 || windows.UTF16ToString(actual[:]) != "Inter" {
		t.Fatalf("Windows selected %q", windows.UTF16ToString(actual[:]))
	}
	logical.LfWeight = win.FW_BOLD
	bold := win.CreateFontIndirect(&logical)
	if bold == 0 {
		t.Fatal("creating bold font failed")
	}
	defer win.DeleteObject(win.HGDIOBJ(bold))
	previous := win.SelectObject(hdc, win.HGDIOBJ(bold))
	var metrics win.TEXTMETRIC
	ok := win.GetTextMetrics(hdc, &metrics)
	win.SelectObject(hdc, previous)
	if !ok || metrics.TmWeight < win.FW_BOLD {
		t.Fatalf("bold font weight = %d", metrics.TmWeight)
	}
}
