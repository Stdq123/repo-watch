//go:build windows

package main

import (
	"strings"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

func highContrastEnabled() bool {
	var settings win.HIGHCONTRAST
	settings.CbSize = uint32(unsafe.Sizeof(settings))
	return win.SystemParametersInfo(win.SPI_GETHIGHCONTRAST, settings.CbSize, unsafe.Pointer(&settings), 0) && settings.DwFlags&win.HCF_HIGHCONTRASTON != 0
}

// repositoryStyler draws the rows while keeping a real Windows list box,
// including its selection, scrolling, keyboard navigation and accessible text.
type repositoryStyler struct{ app *nativeApp }

func (s *repositoryStyler) ItemHeightDependsOnWidth() bool { return false }
func (s *repositoryStyler) DefaultItemHeight() int {
	if s.app.table == nil {
		return 105
	}
	return walk.IntFrom96DPI(105, s.app.table.DPI())
}
func (s *repositoryStyler) ItemHeight(_, _ int) int { return s.DefaultItemHeight() }

// The header and rows share the same proportional columns so resizing never
// causes values to drift away from their labels.
func repositoryColumns(width, dpi int) [7]int {
	if width < 0 {
		width = 0
	}
	if width < walk.IntFrom96DPI(600, dpi) {
		return [7]int{0, width * 45 / 100, width * 64 / 100, width * 81 / 100, width * 81 / 100, width * 81 / 100, width}
	}
	return [7]int{0, width * 30 / 100, width * 44 / 100, width * 61 / 100, width * 72 / 100, width * 82 / 100, width}
}
func paintColumnHeader(c *walk.Canvas, bounds walk.Rectangle, dpi, listWidth, sortIndex int, reverse bool) {
	if c == nil || bounds.Width <= 0 {
		return
	}
	px := func(v int) int { return walk.IntFrom96DPI(v, dpi) }
	brush, err := walk.NewSolidColorBrush(walk.RGB(245, 247, 247))
	if err != nil {
		return
	}
	defer brush.Dispose()
	_ = c.FillRoundedRectanglePixels(brush, bounds, walk.Size{Width: px(8), Height: px(8)})
	// Walk shares fonts by family/size; disposing one here can invalidate a
	// font still in use by another native control.
	width := bounds.Width
	if listWidth > 0 && listWidth < width {
		width = listWidth
	}
	col := repositoryColumns(width, dpi)
	titles := []string{"仓库", "平台", "标签", "Stars", "Forks", "最近更新"}
	for i, text := range titles {
		font, err := walk.NewFont(fontFamilyForText(text), 11, walk.FontBold)
		if err != nil {
			continue
		}
		x := bounds.X + col[i] + px(10)
		w := col[i+1] - col[i] - px(12)
		order, sortable := map[int]int{0: 1, 3: 2, 4: 3, 5: 0}[i]
		if sortable {
			w -= px(17)
		}
		if w > 0 {
			_ = c.DrawTextPixels(text, font, walk.RGB(53, 61, 70), walk.Rectangle{X: x, Y: bounds.Y, Width: w, Height: bounds.Height}, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)
			if sortable {
				measured, _, err := c.MeasureTextPixels(text, font, walk.Rectangle{Width: w, Height: bounds.Height}, walk.TextSingleLine|walk.TextCalcRect)
				if err == nil && measured.Width+px(15) <= col[i+1]-col[i]-px(12) {
					icon, tint := "lucide/arrow-down", walk.RGB(137, 145, 150)
					if order == sortIndex {
						tint = walk.RGB(36, 55, 45)
						if reverse {
							icon = "lucide/arrow-up"
						}
					}
					drawNativeIcon(c.HDC(), walk.Rectangle{X: x + measured.Width + px(4), Y: bounds.Y + (bounds.Height-px(12))/2, Width: px(12), Height: px(12)}, icon, tint)
				}
			}
		}
	}
}
func tagColors(tag string) (walk.Color, walk.Color) {
	var hash uint32 = 2166136261
	for _, r := range tag {
		hash ^= uint32(r)
		hash *= 16777619
	}
	backgrounds := []walk.Color{walk.RGB(219, 239, 220), walk.RGB(218, 234, 251), walk.RGB(237, 228, 250), walk.RGB(255, 244, 205)}
	foregrounds := []walk.Color{walk.RGB(38, 105, 54), walk.RGB(36, 79, 133), walk.RGB(89, 61, 136), walk.RGB(116, 88, 24)}
	i := hash % uint32(len(backgrounds))
	return backgrounds[i], foregrounds[i]
}
func (s *repositoryStyler) StyleItem(item *walk.ListItemStyle) {
	if s.app.table == nil || item.Index() < 0 || item.Index() >= len(s.app.model.items) {
		return
	}
	if highContrastEnabled() {
		_ = item.DrawBackground()
		_ = item.DrawText(s.app.model.Value(item.Index()).(string), item.BoundsPixels(), walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)
		return
	}
	c := item.Canvas()
	if c == nil {
		return
	}
	b := item.BoundsPixels()
	dpi := s.app.table.DPI()
	px := func(v int) int { return walk.IntFrom96DPI(v, dpi) }
	fill := func(color walk.Color, r walk.Rectangle, radius int) {
		if r.Width <= 0 || r.Height <= 0 {
			return
		}
		brush, err := walk.NewSolidColorBrush(color)
		if err != nil {
			return
		}
		defer brush.Dispose()
		if radius > 0 {
			_ = c.FillRoundedRectanglePixels(brush, r, walk.Size{Width: px(radius), Height: px(radius)})
		} else {
			_ = c.FillRectanglePixels(brush, r)
		}
	}
	fill(walk.RGB(255, 255, 255), b, 0)
	active := s.app.table.CurrentIndex() == item.Index()
	if active {
		fill(walk.RGB(234, 245, 237), walk.Rectangle{X: b.X + px(2), Y: b.Y + px(2), Width: b.Width - px(4), Height: b.Height - px(4)}, 8)
		fill(walk.RGB(19, 78, 48), walk.Rectangle{X: b.X + px(2), Y: b.Y + px(6), Width: px(4), Height: b.Height - px(12)}, 3)
	}
	r := s.app.model.items[item.Index()]
	bold, err := walk.NewFont(fontFamilyForText(r.Repo), 12, walk.FontBold)
	if err != nil {
		return
	}
	small, err := walk.NewFont(nativeUIFontFamily, 11, 0)
	if err != nil {
		return
	}
	snap := r.snapshotValue()
	cols := repositoryColumns(b.Width, dpi)
	color := walk.RGB(24, 35, 43)
	muted := walk.RGB(99, 110, 121)
	text := func(str string, col int, y, h int, font *walk.Font, color walk.Color) {
		x := b.X + cols[col] + px(10)
		width := cols[col+1] - cols[col] - px(14)
		if width > 0 {
			_ = c.DrawTextPixels(str, font, color, walk.Rectangle{X: x, Y: b.Y + px(y), Width: width, Height: px(h)}, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)
		}
	}
	iconx := b.X + px(15)
	icony := b.Y + (b.Height-px(42))/2
	icon := px(42)
	paintRepositoryIcon(c, walk.Rectangle{X: iconx, Y: icony, Width: icon, Height: icon}, r.Platform+"/"+r.Owner+"/"+r.Repo)
	left := iconx + icon + px(10)
	nameWidth := b.X + cols[1] - left - px(5)
	if nameWidth > 0 {
		_ = c.DrawTextPixels(r.Repo, bold, color, walk.Rectangle{X: left, Y: b.Y + px(24), Width: nameWidth, Height: px(23)}, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)
		desc := snap.Description
		if desc == "" {
			desc = "暂无项目描述"
		}
		if descFont, fontErr := walk.NewFont(fontFamilyForText(desc), 11, 0); fontErr == nil {
			_ = c.DrawTextPixels(desc, descFont, muted, walk.Rectangle{X: left, Y: b.Y + px(49), Width: nameWidth, Height: px(48)}, walk.TextWordbreak|walk.TextEndEllipsis)
		}
	}
	platform := map[string]string{"github": "GitHub", "codeberg": "Codeberg", "gitea": "Gitea"}[r.Platform]
	if platform == "" {
		platform = r.Platform
	}
	platformX := b.X + cols[1] + px(10)
	platformY := b.Y + (b.Height-px(18))/2
	paintPlatformIcon(c, walk.Rectangle{X: platformX, Y: platformY, Width: px(18), Height: px(18)}, r.Platform)
	platformWidth := cols[2] - cols[1] - px(33)
	if platformWidth > 0 {
		_ = c.DrawTextPixels(platform, small, color, walk.Rectangle{X: platformX + px(21), Y: b.Y + (b.Height-px(24))/2, Width: platformWidth, Height: px(24)}, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)
	}
	tagX := b.X + cols[2] + px(8)
	tagMax := cols[3] - cols[2] - px(16)
	tagTop := (b.Height - px(52)) / 2
	if len(r.Tags) == 1 {
		tagTop = (b.Height - px(23)) / 2
	}
	for i, tag := range r.Tags {
		if i >= 2 || tagMax <= px(14) {
			break
		}
		tagFont, err := walk.NewFont(fontFamilyForText(tag), 11, 0)
		if err != nil {
			break
		}
		m, _, err := c.MeasureTextPixels(tag, tagFont, walk.Rectangle{Width: px(1000), Height: px(22)}, walk.TextSingleLine|walk.TextCalcRect)
		if err != nil {
			break
		}
		width := m.Width + px(18)
		if width > tagMax {
			width = tagMax
		}
		bg, fg := tagColors(tag)
		rect := walk.Rectangle{X: tagX, Y: b.Y + tagTop + px(i*29), Width: width, Height: px(23)}
		fill(bg, rect, 7)
		_ = c.DrawTextPixels(tag, tagFont, fg, walk.Rectangle{X: rect.X + px(8), Y: rect.Y, Width: rect.Width - px(16), Height: rect.Height}, walk.TextSingleLine|walk.TextEndEllipsis|walk.TextVCenter)
	}
	stars, forks := "—", "—"
	updated := "暂无记录"
	if r.Snapshot != nil {
		stars = compactCount(snap.Stars)
		forks = compactCount(snap.Forks)
		updated = relativeUpdate(snap.PushedAt, time.Now())
	}
	for _, metric := range []struct {
		column, kind int
		value        string
	}{{3, 0, stars}, {4, 1, forks}} {
		width := cols[metric.column+1] - cols[metric.column]
		if width <= px(32) {
			continue
		}
		x := b.X + cols[metric.column] + px(10)
		paintMetricIcon(c, walk.Rectangle{X: x, Y: b.Y + (b.Height-px(17))/2, Width: px(17), Height: px(17)}, metric.kind)
		_ = c.DrawTextPixels(metric.value, small, color, walk.Rectangle{X: x + px(23), Y: b.Y + (b.Height-px(24))/2, Width: width - px(34), Height: px(24)}, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)
	}
	if updatedFont, fontErr := walk.NewFont(fontFamilyForText(updated), 11, 0); fontErr == nil {
		text(updated, 5, 40, 24, updatedFont, muted)
	}
	if !active {
		fill(walk.RGB(232, 235, 236), walk.Rectangle{X: b.X + px(8), Y: b.Y + b.Height - px(1), Width: b.Width - px(16), Height: px(1)}, 0)
	}
}

type sidebarStyler struct{ app *nativeApp }

func (s *sidebarStyler) ItemHeightDependsOnWidth() bool { return false }
func (s *sidebarStyler) DefaultItemHeight() int {
	if s.app.filter == nil {
		return 36
	}
	return walk.IntFrom96DPI(45, s.app.filter.DPI())
}
func (s *sidebarStyler) ItemHeight(_, _ int) int { return s.DefaultItemHeight() }
func (s *sidebarStyler) StyleItem(item *walk.ListItemStyle) {
	if s.app.filter == nil || item.Index() < 0 || item.Index() >= len(s.app.filterLabels) {
		return
	}
	if highContrastEnabled() {
		_ = item.DrawBackground()
		_ = item.DrawText(s.app.filterLabels[item.Index()], item.BoundsPixels(), walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)
		return
	}
	c := item.Canvas()
	if c == nil {
		return
	}
	b := item.BoundsPixels()
	dpi := s.app.filter.DPI()
	px := func(v int) int { return walk.IntFrom96DPI(v, dpi) }
	fill := func(color walk.Color, r walk.Rectangle, radius int) {
		brush, err := walk.NewSolidColorBrush(color)
		if err != nil {
			return
		}
		defer brush.Dispose()
		if radius > 0 {
			_ = c.FillRoundedRectanglePixels(brush, r, walk.Size{Width: px(radius), Height: px(radius)})
		} else {
			_ = c.FillRectanglePixels(brush, r)
		}
	}
	fill(walk.RGB(248, 249, 249), b, 0)
	selected := s.app.filter.CurrentIndex() == item.Index()
	if selected {
		fill(walk.RGB(229, 242, 231), walk.Rectangle{X: b.X + px(2), Y: b.Y + px(2), Width: b.Width - px(4), Height: b.Height - px(4)}, 8)
		fill(walk.RGB(29, 107, 62), walk.Rectangle{X: b.X + px(2), Y: b.Y + px(8), Width: px(3), Height: b.Height - px(16)}, 2)
	}
	color := walk.RGB(42, 50, 60)
	nameColor := walk.RGB(42, 50, 60)
	if selected {
		color = walk.RGB(34, 89, 55)
		nameColor = walk.RGB(30, 71, 45)
	}
	icon := "lucide/tag"
	if item.Index() == 0 {
		icon = "lucide/layout-grid"
	}
	drawNativeIcon(c.HDC(), walk.Rectangle{X: b.X + px(12), Y: b.Y + (b.Height-px(18))/2, Width: px(18), Height: px(18)}, icon, color)
	text := s.app.filterLabels[item.Index()]
	name, count := text, ""
	if n := strings.LastIndex(text, "   "); n >= 0 {
		name, count = text[:n], text[n+3:]
	}
	if nameFont, err := walk.NewFont(fontFamilyForText(name), 11, 0); err == nil {
		_ = c.DrawTextPixels(name, nameFont, nameColor, walk.Rectangle{X: b.X + px(34), Y: b.Y, Width: b.Width - px(70), Height: b.Height}, walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
	}
	if countFont, err := walk.NewFont(nativeUIFontFamily, 11, 0); err == nil {
		_ = c.DrawTextPixels(count, countFont, walk.RGB(92, 109, 98), walk.Rectangle{X: b.X + b.Width - px(30), Y: b.Y, Width: px(20), Height: b.Height}, walk.TextRight|walk.TextVCenter|walk.TextSingleLine)
	}
}

// Repositories use a stable generic Lucide icon until they expose an avatar.
func paintRepositoryIcon(c *walk.Canvas, b walk.Rectangle, name string) {
	var hash uint32 = 2166136261
	for _, r := range strings.ToLower(name) {
		hash = (hash ^ uint32(r)) * 16777619
	}
	icons := []string{"lucide/leaf", "lucide/box", "lucide/waves", "lucide/book-open"}
	if c != nil {
		drawNativeIcon(c.HDC(), b, icons[hash%uint32(len(icons))], walk.RGB(15, 68, 49))
	}
}

func paintPlatformIcon(c *walk.Canvas, b walk.Rectangle, platform string) {
	if c == nil {
		return
	}
	name, color := "simple-icons/github", walk.RGB(24, 23, 23)
	switch platform {
	case "codeberg":
		name, color = "simple-icons/codeberg", walk.RGB(33, 133, 208)
	case "gitea":
		name, color = "simple-icons/gitea", walk.RGB(96, 153, 38)
	case "github":
	default:
		name, color = "lucide/box", walk.RGB(55, 68, 65)
	}
	drawNativeIcon(c.HDC(), b, name, color)
}

func paintAppLogo(c *walk.Canvas, b walk.Rectangle) error {
	if c == nil || b.Width <= 0 || b.Height <= 0 {
		return nil
	}
	green, err := walk.NewSolidColorBrush(walk.RGB(14, 83, 49))
	if err != nil {
		return err
	}
	defer green.Dispose()
	_ = c.FillRoundedRectanglePixels(green, b, walk.Size{Width: b.Width / 4, Height: b.Height / 4})
	margin := b.Width / 5
	drawNativeIcon(c.HDC(), walk.Rectangle{X: b.X + margin, Y: b.Y + margin, Width: b.Width - 2*margin, Height: b.Height - 2*margin}, "lucide/binoculars", walk.RGB(255, 255, 255))
	return nil
}

func paintDetailIcon(c *walk.Canvas, b walk.Rectangle, dpi int, r *Repository) {
	if c != nil && r != nil {
		paintRepositoryIcon(c, b, r.Platform+"/"+r.Owner+"/"+r.Repo)
	}
}
func paintDetailTags(c *walk.Canvas, b walk.Rectangle, dpi int, tags []string) {
	if c == nil {
		return
	}
	px := func(v int) int { return walk.IntFrom96DPI(v, dpi) }
	white, err := walk.NewSolidColorBrush(walk.RGB(255, 255, 255))
	if err != nil {
		return
	}
	_ = c.FillRectanglePixels(white, b)
	white.Dispose()
	if len(tags) == 0 {
		if font, err := walk.NewFont(nativeCJKFontFamily, 10, 0); err == nil {
			_ = c.DrawTextPixels("暂无标签", font, walk.RGB(120, 130, 137), b, walk.TextSingleLine|walk.TextVCenter)
		}
		return
	}
	x := b.X
	for _, tag := range tags {
		font, err := walk.NewFont(fontFamilyForText(tag), 10, 0)
		if err != nil {
			return
		}
		m, _, err := c.MeasureTextPixels(tag, font, walk.Rectangle{Width: px(1000), Height: px(22)}, walk.TextSingleLine|walk.TextCalcRect)
		if err != nil {
			return
		}
		width := m.Width + px(16)
		if width > px(120) {
			width = px(120)
		}
		if x+width > b.X+b.Width {
			break
		}
		bg, fg := tagColors(tag)
		brush, err := walk.NewSolidColorBrush(bg)
		if err != nil {
			return
		}
		chip := walk.Rectangle{X: x, Y: b.Y + px(2), Width: width, Height: px(23)}
		_ = c.FillRoundedRectanglePixels(brush, chip, walk.Size{Width: px(7), Height: px(7)})
		brush.Dispose()
		_ = c.DrawTextPixels(tag, font, fg, walk.Rectangle{X: x + px(8), Y: chip.Y, Width: width - px(16), Height: chip.Height}, walk.TextSingleLine|walk.TextEndEllipsis|walk.TextVCenter)
		x += width + px(5)
	}
}

func paintSeparator(c *walk.Canvas, b walk.Rectangle) error {
	brush, err := walk.NewSolidColorBrush(walk.RGB(231, 234, 234))
	if err != nil {
		return err
	}
	defer brush.Dispose()
	return c.FillRectanglePixels(brush, b)
}

func paintSearchIcon(c *walk.Canvas, b walk.Rectangle) error {
	if c != nil {
		drawNativeIcon(c.HDC(), b, "lucide/search", walk.RGB(91, 102, 115))
	}
	return nil
}

type detailFact struct{ label, value string }

func paintDetailFacts(c *walk.Canvas, b walk.Rectangle, dpi int, rows []detailFact) {
	if c == nil || b.Width <= 0 || b.Height <= 0 {
		return
	}
	labelFont, err := walk.NewFont(nativeCJKFontFamily, 11, 0)
	if err != nil {
		return
	}
	rowHeight := b.Height / 5
	leftWidth := b.Width * 38 / 100
	px := func(v int) int { return walk.IntFrom96DPI(v, dpi) }
	for i, row := range rows {
		if i >= 5 {
			break
		}
		y := b.Y + i*rowHeight
		_ = c.DrawTextPixels(row.label, labelFont, walk.RGB(111, 121, 136), walk.Rectangle{X: b.X, Y: y, Width: leftWidth, Height: rowHeight}, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)
		valueX := b.X + leftWidth
		if row.label == "平台" && row.value != "—" {
			paintPlatformIcon(c, walk.Rectangle{X: valueX, Y: y + (rowHeight-px(18))/2, Width: px(18), Height: px(18)}, strings.ToLower(row.value))
			valueX += px(24)
		} else if row.label == "语言" && row.value != "—" {
			color := walk.RGB(47, 166, 73)
			switch strings.ToLower(row.value) {
			case "go":
				color = walk.RGB(0, 173, 216)
			case "typescript":
				color = walk.RGB(49, 120, 198)
			case "javascript":
				color = walk.RGB(230, 195, 35)
			case "rust":
				color = walk.RGB(194, 96, 40)
			}
			if brush, err := walk.NewSolidColorBrush(color); err == nil {
				_ = c.FillEllipsePixels(brush, walk.Rectangle{X: valueX + px(3), Y: y + (rowHeight-px(9))/2, Width: px(9), Height: px(9)})
				brush.Dispose()
			}
			valueX += px(24)
		}
		if valueFont, err := walk.NewFont(fontFamilyForText(row.value), 11, 0); err == nil {
			_ = c.DrawTextPixels(row.value, valueFont, walk.RGB(44, 54, 68), walk.Rectangle{X: valueX, Y: y, Width: b.X + b.Width - valueX, Height: rowHeight}, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)
		}
	}
}

func paintDetailMetrics(c *walk.Canvas, b walk.Rectangle, dpi int, values [3]string) {
	if c == nil || b.Width <= 0 || b.Height <= 0 {
		return
	}
	px := func(v int) int { return walk.IntFrom96DPI(v, dpi) }
	font, err := walk.NewFont(nativeUIFontFamily, 10, 0)
	if err != nil {
		return
	}
	col := b.Width / 3
	for i := 0; i < 3; i++ {
		x := b.X + i*col
		icon := walk.Rectangle{X: x, Y: b.Y + (b.Height-px(18))/2, Width: px(18), Height: px(18)}
		paintMetricIcon(c, icon, i)
		_ = c.DrawTextPixels(values[i], font, walk.RGB(43, 53, 66), walk.Rectangle{X: x + px(25), Y: b.Y, Width: col - px(26), Height: b.Height}, walk.TextSingleLine|walk.TextVCenter|walk.TextEndEllipsis)
	}
}

func paintMetricIcon(c *walk.Canvas, b walk.Rectangle, kind int) {
	icons := []string{"lucide/star", "lucide/git-fork", "lucide/eye"}
	if c != nil && kind >= 0 && kind < len(icons) {
		drawNativeIcon(c.HDC(), b, icons[kind], walk.RGB(37, 47, 61))
	}
}

func paintReleaseCard(c *walk.Canvas, b walk.Rectangle, dpi int, release, notes, date string) {
	if c == nil || b.Width <= 0 || b.Height <= 0 {
		return
	}
	px := func(v int) int { return walk.IntFrom96DPI(v, dpi) }
	background, err := walk.NewSolidColorBrush(walk.RGB(255, 255, 255))
	if err != nil {
		return
	}
	defer background.Dispose()
	_ = c.FillRectanglePixels(background, b)
	border, err := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(220, 225, 226))
	if err != nil {
		return
	}
	defer border.Dispose()
	box := walk.Rectangle{X: b.X, Y: b.Y, Width: b.Width - px(1), Height: b.Height - px(1)}
	_ = c.DrawRoundedRectanglePixels(border, box, walk.Size{Width: px(6), Height: px(6)})
	if release == "" {
		release = "暂无发行版"
	}
	font, err := walk.NewFont(fontFamilyForText(release), 11, walk.FontBold)
	if err != nil {
		return
	}
	titleWidth := b.Width - px(28)
	if release != "暂无发行版" {
		titleWidth = b.Width - px(150)
	}
	if titleWidth < px(40) {
		titleWidth = px(40)
	}
	_ = c.DrawTextPixels(release, font, walk.RGB(35, 44, 51), walk.Rectangle{X: b.X + px(14), Y: b.Y + px(8), Width: titleWidth, Height: px(22)}, walk.TextSingleLine|walk.TextEndEllipsis|walk.TextVCenter)
	caption, err := walk.NewFont(nativeCJKFontFamily, 10, 0)
	if err != nil {
		return
	}
	if release != "暂无发行版" {
		measured, _, measureErr := c.MeasureTextPixels(release, font, walk.Rectangle{Width: titleWidth, Height: px(22)}, walk.TextSingleLine|walk.TextCalcRect)
		badgeX := b.X + px(14) + measured.Width + px(8)
		if measureErr == nil && badgeX+px(38) < b.X+b.Width-px(80) {
			if pen, penErr := walk.NewCosmeticPen(walk.PenSolid, walk.RGB(88, 174, 108)); penErr == nil {
				badge := walk.Rectangle{X: badgeX, Y: b.Y + px(10), Width: px(36), Height: px(19)}
				_ = c.DrawRoundedRectanglePixels(pen, badge, walk.Size{Width: px(18), Height: px(18)})
				_ = c.DrawTextPixels("最新", caption, walk.RGB(32, 111, 54), badge, walk.TextCenter|walk.TextVCenter|walk.TextSingleLine)
				pen.Dispose()
			}
		}
		drawNativeIcon(c.HDC(), walk.Rectangle{X: b.X + b.Width - px(22), Y: b.Y + (b.Height-px(14))/2, Width: px(14), Height: px(14)}, "lucide/chevron-right", walk.RGB(55, 65, 75))
	}
	if release != "暂无发行版" && date != "" && date != "暂无记录" {
		_ = c.DrawTextPixels(date, caption, walk.RGB(110, 121, 130), walk.Rectangle{X: b.X + b.Width - px(92), Y: b.Y + px(8), Width: px(64), Height: px(22)}, walk.TextSingleLine|walk.TextRight|walk.TextEndEllipsis|walk.TextVCenter)
	}
	message := strings.TrimSpace(notes)
	if message == "" {
		message = "暂无发行说明"
		if release == "暂无发行版" {
			message = "暂无可用的发行版信息"
		}
	}
	if messageFont, err := walk.NewFont(fontFamilyForText(message), 10, 0); err == nil {
		_ = c.DrawTextPixels(message, messageFont, walk.RGB(110, 121, 130), walk.Rectangle{X: b.X + px(14), Y: b.Y + px(33), Width: b.Width - px(28), Height: b.Height - px(39)}, walk.TextWordbreak|walk.TextEndEllipsis)
	}
}
