package main

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"regexp"

	"golang.org/x/image/draw"
)

//go:embed assets/icons/lucide/*.svg assets/icons/lucide/*.png assets/icons/lucide/LICENSE assets/icons/lucide/FEATHER-LICENSE assets/icons/simple-icons/*.svg assets/icons/simple-icons/*.png assets/icons/simple-icons/LICENSE.md assets/icons/simple-icons/DISCLAIMER.md
var iconAssets embed.FS

var iconNamePattern = regexp.MustCompile(`^(lucide|simple-icons)/[a-z0-9-]+$`)

// renderIcon scales a pre-rendered SVG alpha mask to physical pixels and tints
// it for the current component state. No external files or network are used.
func renderIcon(name string, size int, tint color.RGBA) (*image.RGBA, error) {
	if !iconNamePattern.MatchString(name) || size < 1 || size > 512 {
		return nil, fmt.Errorf("invalid icon request")
	}
	data, err := iconAssets.ReadFile("assets/icons/" + name + ".png")
	if err != nil {
		return nil, err
	}
	icon, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode icon %s: %w", name, err)
	}
	imageRGBA := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(imageRGBA, imageRGBA.Bounds(), icon, icon.Bounds(), draw.Src, nil)
	for i := 0; i < len(imageRGBA.Pix); i += 4 {
		alpha := uint16(imageRGBA.Pix[i+3]) * uint16(tint.A) / 255
		imageRGBA.Pix[i] = uint8(uint16(tint.R) * alpha / 255)
		imageRGBA.Pix[i+1] = uint8(uint16(tint.G) * alpha / 255)
		imageRGBA.Pix[i+2] = uint8(uint16(tint.B) * alpha / 255)
		imageRGBA.Pix[i+3] = uint8(alpha)
	}
	return imageRGBA, nil
}
