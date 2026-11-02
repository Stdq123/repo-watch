package main

import (
	"image/color"
	"strings"
	"testing"
)

func TestIconResourcesRenderWithRequestedColorAndTransparency(t *testing.T) {
	for _, name := range []string{"lucide/star", "lucide/binoculars", "lucide/search", "simple-icons/github", "simple-icons/gitea", "simple-icons/codeberg"} {
		t.Run(name, func(t *testing.T) {
			icon, err := renderIcon(name, 48, color.RGBA{R: 30, G: 140, B: 70, A: 255})
			if err != nil {
				t.Fatal(err)
			}
			if icon.Bounds().Dx() != 48 || icon.Bounds().Dy() != 48 {
				t.Fatalf("wrong dimensions: %v", icon.Bounds())
			}
			visible, transparent, colored := 0, 0, 0
			for y := 0; y < 48; y++ {
				for x := 0; x < 48; x++ {
					r, g, b, a := icon.At(x, y).RGBA()
					if a == 0 {
						transparent++
					} else {
						visible++
						if g > r && g > b {
							colored++
						}
					}
				}
			}
			if visible == 0 || transparent == 0 || colored == 0 {
				t.Fatalf("visible=%d transparent=%d colored=%d", visible, transparent, colored)
			}
		})
	}
}

func TestAllBundledIconsRender(t *testing.T) {
	for _, family := range []string{"lucide", "simple-icons"} {
		entries, err := iconAssets.ReadDir("assets/icons/" + family)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".svg") {
				continue
			}
			name := family + "/" + strings.TrimSuffix(entry.Name(), ".svg")
			icon, err := renderIcon(name, 48, color.RGBA{R: 25, G: 80, B: 40, A: 255})
			if err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			visible := false
			for i := 3; i < len(icon.Pix); i += 4 {
				if icon.Pix[i] != 0 {
					visible = true
					break
				}
			}
			if !visible {
				t.Errorf("%s rendered empty", name)
			}
		}
	}
}

func TestIconRendererRejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{{"missing/icon", 24}, {"lucide/star", 0}, {"lucide/star", 513}, {"../lucide/star", 24}} {
		if _, err := renderIcon(tc.name, tc.size, color.RGBA{A: 255}); err == nil {
			t.Errorf("accepted %+v", tc)
		}
	}
}

func TestEyeIconRendersOutlineAsWellAsPupil(t *testing.T) {
	icon, err := renderIcon("lucide/eye", 48, color.RGBA{R: 20, G: 60, B: 40, A: 255})
	if err != nil {
		t.Fatal(err)
	}
	minX, maxX, minY, maxY := 48, -1, 48, -1
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			if icon.RGBAAt(x, y).A != 0 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX-minX < 32 || maxY-minY < 18 {
		t.Fatalf("eye outline did not render: (%d,%d)-(%d,%d)", minX, minY, maxX, maxY)
	}
}

func TestCodebergMarkKeepsItsTransparentMountain(t *testing.T) {
	icon, err := renderIcon("simple-icons/codeberg", 240, color.RGBA{R: 33, G: 133, B: 208, A: 255})
	if err != nil {
		t.Fatal(err)
	}
	if icon.RGBAAt(120, 20).A < 200 || icon.RGBAAt(100, 150).A != 0 {
		t.Fatal("Codeberg mark lost its foreground or transparent mountain")
	}
}
