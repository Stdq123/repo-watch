//go:build windows

package main

import (
	"image/color"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

type nativeIconKey struct {
	name string
	size int
	tint walk.Color
}

// These bitmaps are created and used only by the Windows UI thread. They are
// released after the window closes, including variants for prior DPI values.
var nativeIconBitmaps = map[nativeIconKey]win.HBITMAP{}

func drawNativeIcon(hdc win.HDC, bounds walk.Rectangle, name string, tint walk.Color) {
	if hdc == 0 || bounds.Width <= 0 || bounds.Height <= 0 {
		return
	}
	size := bounds.Width
	if bounds.Height < size {
		size = bounds.Height
	}
	key := nativeIconKey{name: name, size: size, tint: tint}
	bitmap := nativeIconBitmaps[key]
	if bitmap == 0 {
		img, err := renderIcon(name, size, color.RGBA{R: uint8(tint), G: uint8(tint >> 8), B: uint8(tint >> 16), A: 255})
		if err != nil {
			return
		}
		header := win.BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(win.BITMAPINFOHEADER{})), BiWidth: int32(size), BiHeight: -int32(size), BiPlanes: 1, BiBitCount: 32, BiCompression: win.BI_RGB}
		var pixels unsafe.Pointer
		bitmap = win.CreateDIBSection(hdc, &header, win.DIB_RGB_COLORS, &pixels, 0, 0)
		if bitmap == 0 || pixels == nil {
			return
		}
		buffer := unsafe.Slice((*byte)(pixels), size*size*4)
		for i := 0; i < len(buffer); i += 4 {
			buffer[i], buffer[i+1], buffer[i+2], buffer[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3]
		}
		nativeIconBitmaps[key] = bitmap
	}
	memory := win.CreateCompatibleDC(hdc)
	if memory == 0 {
		return
	}
	defer win.DeleteDC(memory)
	previous := win.SelectObject(memory, win.HGDIOBJ(bitmap))
	x, y := bounds.X+(bounds.Width-size)/2, bounds.Y+(bounds.Height-size)/2
	win.AlphaBlend(hdc, int32(x), int32(y), int32(size), int32(size), memory, 0, 0, int32(size), int32(size), win.BLENDFUNCTION{SourceConstantAlpha: 255, AlphaFormat: win.AC_SRC_ALPHA})
	win.SelectObject(memory, previous)
}

func disposeNativeIcons() {
	for key, bitmap := range nativeIconBitmaps {
		win.DeleteObject(win.HGDIOBJ(bitmap))
		delete(nativeIconBitmaps, key)
	}
}
