//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

//go:embed assets/repowatch-tray.png
var assets embed.FS

const instanceProtocol = "repowatch-native-inter-v18"

type repositoryModel struct {
	walk.ListModelBase
	items []Repository
}

func (m *repositoryModel) ItemCount() int { return len(m.items) }
func (m *repositoryModel) Value(row int) interface{} {
	r := m.items[row]
	stars := "尚未刷新"
	if r.Snapshot != nil {
		stars = fmt.Sprintf("%d stars", r.Snapshot.Stars)
	}
	forks := "—"
	if r.Snapshot != nil {
		forks = fmt.Sprintf("%d forks", r.Snapshot.Forks)
	}
	return fmt.Sprintf("%s/%s, %s, %s, %s, %s", r.Owner, r.Repo, r.Platform, strings.Join(r.Tags, ", "), stars, forks)
}
func (r Repository) snapshotValue() Snapshot {
	if r.Snapshot == nil {
		return Snapshot{}
	}
	return *r.Snapshot
}

type nativeApp struct {
	window             *walk.MainWindow
	appLogo            *walk.CustomWidget
	searchIcon         *walk.CustomWidget
	searchFrame        *walk.Composite
	searchInner        *walk.Composite
	titleBar           *walk.Composite
	table              *walk.ListBox
	filter             *walk.ListBox
	filterLabels       []string
	search             *walk.LineEdit
	sortButton         *walk.PushButton
	sortIndex          int
	sortDirection      *walk.PushButton
	reverseSort        bool
	columnHeader       *walk.CustomWidget
	centerFrame        *walk.Composite
	centerPanel        *walk.Composite
	bodyPanel          *walk.Composite
	inspector          *walk.Composite
	detailHeader       *walk.Composite
	compactLayout      bool
	compactLayoutSet   bool
	updatingScrollbar  bool
	count              *walk.Label
	detailTitle        *walk.Label
	detailMeta         *walk.Label
	detailLinkButton   *walk.PushButton
	detailMetrics      *walk.CustomWidget
	detailTags         *walk.CustomWidget
	detailIcon         *walk.CustomWidget
	detailTagValues    []string
	detailFacts        *walk.CustomWidget
	detailFactRows     []detailFact
	detailMetricText   [3]string
	detailRelease      *walk.CustomWidget
	detailReleaseText  string
	detailReleaseNotes string
	detailReleaseDate  string
	addButton          *walk.PushButton
	refreshButton      *walk.PushButton
	settingsButton     *walk.PushButton
	openButton         *walk.PushButton
	releaseButton      *walk.PushButton
	releasesAllButton  *walk.PushButton
	terminalButton     *walk.PushButton
	manageButton       *walk.PushButton
	minimizeButton     *walk.PushButton
	maximizeButton     *walk.PushButton
	closeButton        *walk.PushButton
	details            *walk.TextEdit
	status             *walk.Label
	pin                *walk.CheckBox
	tray               *walk.NotifyIcon
	model              *repositoryModel
	monitor            *monitor
	service            *Service
	all                []Repository
	filters            []string
	selectedTag        string
	rebuilding         bool
	quitting           bool
	last               string
	settingsPath       string
	loadingSettings    bool
}

func main() {
	// Use the same per-user socket as the previous version to avoid simultaneous writers.
	listener, already, legacyInstance, err := acquireInstance()
	if err != nil {
		walk.MsgBox(nil, "Repo Watch", "无法确保单实例运行："+err.Error(), walk.MsgBoxIconError)
		return
	}
	if already {
		if legacyInstance {
			walk.MsgBox(nil, "Repo Watch", "旧版 Repo Watch 正在运行，请先从托盘退出旧版，再启动原生版。", walk.MsgBoxIconInformation)
		}
		return
	}
	defer listener.Close()
	if releaseFonts, fontErr := registerBundledFonts(); fontErr == nil {
		nativeUIFontFamily = "Inter"
		defer releaseFonts()
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		walk.MsgBox(nil, "Repo Watch", err.Error(), walk.MsgBoxIconError)
		return
	}
	var app *nativeApp
	m, err := newMonitor(filepath.Join(dir, "RepoWatch", "monitor.json"), fetchWithCredential, func(title, body string) error {
		if app == nil || app.tray == nil {
			return fmtNotificationUnsupported()
		}
		app.window.Synchronize(func() { _ = app.tray.ShowInfo(title, body) })
		return nil
	})
	if err != nil {
		walk.MsgBox(nil, "Repo Watch", "无法读取仓库数据："+err.Error(), walk.MsgBoxIconError)
		return
	}
	if m.NeedsMigration() {
		legacy := filepath.Join(os.Getenv("LOCALAPPDATA"), "RepoWatch", "WebView2")
		if _, err := os.Stat(legacy); err == nil {
			walk.MsgBox(nil, "旧数据提示", "发现旧版 WebView 数据，但尚未迁移到监控文件。请先运行旧版完成迁移，再关闭旧版并启动原生版。原有数据不会被删除。", walk.MsgBoxIconWarning)
			return
		}
	}
	app = &nativeApp{monitor: m, service: &Service{monitor: m}, model: &repositoryModel{}, settingsPath: filepath.Join(dir, "RepoWatch", "ui.json")}
	if err = app.create(); err != nil {
		walk.MsgBox(nil, "Repo Watch", "启动失败："+err.Error(), walk.MsgBoxIconError)
		return
	}
	defer disposeNativeIcons()
	defer app.window.Dispose()
	defer app.tray.Dispose()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(io.Discard, io.LimitReader(conn, 1<<20))
				app.window.Synchronize(app.show)
				_, _ = conn.Write([]byte(instanceProtocol))
			}()
		}
	}()
	app.refreshView()
	m.Run(ctx, 10*time.Minute)
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				app.window.Synchronize(app.refreshView)
			}
		}
	}()
	app.window.Run()
}

func acquireInstance() (net.Listener, bool, bool, error) {
	path := instanceSocket(os.Getenv("XDG_RUNTIME_DIR"), os.TempDir(), os.Getuid())
	for i := 0; i < 5; i++ {
		conn, err := net.DialTimeout("unix", path, time.Second)
		if err == nil {
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			_, _ = conn.Write([]byte(`{"args":[],"workingDir":""}`))
			if c, ok := conn.(*net.UnixConn); ok {
				_ = c.CloseWrite()
			}
			reply, _ := io.ReadAll(io.LimitReader(conn, 64))
			conn.Close()
			return nil, true, string(reply) != instanceProtocol, nil
		}
		_ = os.Remove(path)
		ln, err := net.Listen("unix", path)
		if err == nil {
			return ln, false, false, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, false, false, fmt.Errorf("无法取得应用锁")
}

func (a *nativeApp) create() error {
	dark := walk.RGB(18, 24, 36)
	muted := walk.RGB(100, 120, 108)
	background := SolidColorBrush{Color: walk.RGB(248, 249, 249)}
	surface := SolidColorBrush{Color: walk.RGB(255, 255, 255)}
	data, err := assets.ReadFile("assets/repowatch-tray.png")
	if err != nil {
		return err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	err = (MainWindow{
		AssignTo: &a.window, Title: "Repo Watch", Size: Size{Width: 1440, Height: 900}, MinSize: Size{Width: 1050, Height: 650},
		Font: Font{Family: nativeCJKFontFamily, PointSize: 11}, Background: background, Layout: Grid{Columns: 1, MarginsZero: true, SpacingZero: true, Alignment: AlignHNearVNear},
		Children: []Widget{
			Composite{Row: 0, AssignTo: &a.titleBar, Background: background, MinSize: Size{Height: 58}, MaxSize: Size{Height: 58}, Layout: HBox{Margins: Margins{Left: 18, Top: 5, Right: 6, Bottom: 5}, Spacing: 10, Alignment: AlignHNearVCenter}, Children: []Widget{
				CustomWidget{AssignTo: &a.appLogo, Background: background, MinSize: Size{Width: 40, Height: 40}, MaxSize: Size{Width: 40, Height: 40}, Accessibility: Accessibility{Name: "Repo Watch", Role: AccRoleGraphic}, PaintPixels: func(c *walk.Canvas, _ walk.Rectangle) error {
					if a.appLogo != nil {
						return paintAppLogo(c, a.appLogo.ClientBoundsPixels())
					}
					return nil
				}},
				Label{Text: "Repo Watch", TextColor: dark, Font: Font{Family: nativeUIFontFamily, PointSize: 17, Bold: true}},
				HSpacer{},
				PushButton{AssignTo: &a.addButton, Text: "添加仓库", MinSize: Size{Width: 125}, OnClicked: a.addRepository},
				PushButton{AssignTo: &a.refreshButton, Text: "刷新", MinSize: Size{Width: 95}, OnClicked: func() { a.monitor.Trigger(); a.status.SetText("已请求后台刷新") }},
				PushButton{AssignTo: &a.settingsButton, Text: "设置", MinSize: Size{Width: 32}, MaxSize: Size{Width: 32}, Accessibility: Accessibility{Name: "设置"}, OnClicked: a.openSettings},
				PushButton{AssignTo: &a.minimizeButton, Text: "—", MinSize: Size{Width: 32}, MaxSize: Size{Width: 32}, OnClicked: func() { win.ShowWindow(a.window.Handle(), win.SW_MINIMIZE) }},
				PushButton{AssignTo: &a.maximizeButton, Text: "□", MinSize: Size{Width: 32}, MaxSize: Size{Width: 32}, OnClicked: func() {
					if win.IsZoomed(a.window.Handle()) {
						win.ShowWindow(a.window.Handle(), win.SW_RESTORE)
					} else {
						win.ShowWindow(a.window.Handle(), win.SW_MAXIMIZE)
					}
				}},
				PushButton{AssignTo: &a.closeButton, Text: "×", MinSize: Size{Width: 32}, MaxSize: Size{Width: 32}, OnClicked: func() { a.window.Close() }},
			}},
			Composite{Row: 1, AssignTo: &a.bodyPanel, Background: background, StretchFactor: 1, Layout: Grid{Columns: 3, MarginsZero: true, Spacing: 1, Alignment: AlignHNearVNear}, Children: []Widget{
				Composite{Column: 0, Background: background, MinSize: Size{Width: 215}, MaxSize: Size{Width: 230}, Layout: VBox{Margins: Margins{Left: 10, Top: 16, Right: 8, Bottom: 8}, Spacing: 8, Alignment: AlignHNearVNear}, Children: []Widget{
					styledListBox(ListBox{AssignTo: &a.filter, Background: background, Model: []string{"全部仓库"}, ItemStyler: &sidebarStyler{app: a}, OnCurrentIndexChanged: func() {
						if a.rebuilding || a.filter == nil {
							return
						}
						i := a.filter.CurrentIndex()
						if i >= 0 && i < len(a.filters) {
							a.selectedTag = a.filters[i]
							a.renderRows()
						}
					}}),
				}},
				Composite{Column: 1, AssignTo: &a.centerFrame, Background: SolidColorBrush{Color: walk.RGB(232, 235, 236)}, MinSize: Size{Width: 420}, StretchFactor: 4, Layout: Grid{Columns: 1, Margins: Margins{Left: 1, Top: 1, Right: 1, Bottom: 1}, SpacingZero: true}, Children: []Widget{
					Composite{AssignTo: &a.centerPanel, Background: surface, MinSize: Size{Width: 418}, StretchFactor: 4, Layout: Grid{Columns: 1, Margins: Margins{Left: 14, Top: 19, Right: 12, Bottom: 8}, Spacing: 8, Alignment: AlignHNearVNear}, Children: []Widget{
						Composite{Row: 0, Background: surface, MinSize: Size{Height: 48}, MaxSize: Size{Height: 48}, Layout: Grid{Columns: 1, MarginsZero: true, SpacingZero: true, Alignment: AlignHNearVNear}, Children: []Widget{
							Composite{Background: surface, MinSize: Size{Height: 40}, MaxSize: Size{Height: 40}, Layout: HBox{MarginsZero: true, Spacing: 8, Alignment: AlignHNearVCenter}, Children: []Widget{
								Composite{AssignTo: &a.searchFrame, Background: SolidColorBrush{Color: walk.RGB(217, 222, 225)}, StretchFactor: 4, Layout: HBox{Margins: Margins{Left: 1, Top: 1, Right: 1, Bottom: 1}, SpacingZero: true, Alignment: AlignHNearVCenter}, Children: []Widget{
									Composite{AssignTo: &a.searchInner, Background: surface, Layout: Grid{Columns: 2, Margins: Margins{Left: 9, Top: 4, Right: 6}, Spacing: 6, Alignment: AlignHNearVCenter}, Children: []Widget{
										CustomWidget{Column: 0, AssignTo: &a.searchIcon, Background: surface, MinSize: Size{Width: 16, Height: 16}, MaxSize: Size{Width: 16, Height: 16}, PaintPixels: func(c *walk.Canvas, _ walk.Rectangle) error {
											if a.searchIcon != nil {
												return paintSearchIcon(c, a.searchIcon.ClientBoundsPixels())
											}
											return nil
										}},
										LineEdit{Column: 1, AssignTo: &a.search, CueBanner: "搜索仓库、平台或标签", StretchFactor: 4, OnTextChanged: func() {
											if a.table != nil {
												a.renderRows()
											}
										}},
									}},
								}},
								PushButton{AssignTo: &a.sortButton, Text: "排序：最近更新", MinSize: Size{Width: 180, Height: 40}, MaxSize: Size{Width: 190, Height: 40}, OnClicked: func() { a.showSortMenu() }},
								PushButton{AssignTo: &a.sortDirection, Text: "⇅", MinSize: Size{Width: 40, Height: 40}, MaxSize: Size{Width: 40, Height: 40}, Accessibility: Accessibility{Name: "切换排序方向"}, OnClicked: func() {
									a.reverseSort = !a.reverseSort
									a.updateSortDirectionLabel()
									a.renderRows()
								}},
							}},
						}},
						CustomWidget{Row: 1, AssignTo: &a.columnHeader, MinSize: Size{Height: 42}, MaxSize: Size{Height: 42}, InvalidatesOnResize: true, Accessibility: Accessibility{Name: "仓库、平台、标签、Stars、Forks、最近更新；点击表头排序", Role: AccRoleColumnHeader}, OnMouseDown: func(x, _ int, button walk.MouseButton) {
							if button == walk.LeftButton && a.table != nil {
								a.sortByHeader(x)
							}
						}, PaintPixels: func(c *walk.Canvas, _ walk.Rectangle) error {
							if a.columnHeader != nil {
								width := 0
								if a.table != nil {
									width = a.table.ClientBoundsPixels().Width
								}
								paintColumnHeader(c, a.columnHeader.ClientBoundsPixels(), a.columnHeader.DPI(), width, a.sortIndex, a.reverseSort)
							}
							return nil
						}},
						styledListBox(ListBox{Row: 2, AssignTo: &a.table, Model: a.model, ItemStyler: &repositoryStyler{app: a}, OnCurrentIndexChanged: func() {
							if a.table != nil {
								a.updateDetails()
							}
						}, OnItemActivated: func() { a.openLink(false) }}),
					}},
				}},
				Composite{Column: 2, RowSpan: 2, AssignTo: &a.inspector, Background: surface, MinSize: Size{Width: 315}, MaxSize: Size{Width: 340}, Layout: VBox{Margins: Margins{Left: 20, Top: 16, Right: 20, Bottom: 10}, Spacing: 8, Alignment: AlignHNearVNear}, Children: []Widget{
					Composite{AssignTo: &a.detailHeader, Background: surface, MinSize: Size{Height: 65}, MaxSize: Size{Height: 65}, Layout: HBox{MarginsZero: true, Spacing: 10, Alignment: AlignHNearVCenter}, Children: []Widget{
						CustomWidget{AssignTo: &a.detailIcon, Background: surface, MinSize: Size{Width: 54, Height: 54}, MaxSize: Size{Width: 54, Height: 54}, Accessibility: Accessibility{Name: "仓库图标", Role: AccRoleGraphic}, PaintPixels: func(c *walk.Canvas, _ walk.Rectangle) error {
							if a.detailIcon != nil {
								paintDetailIcon(c, a.detailIcon.ClientBoundsPixels(), a.detailIcon.DPI(), a.selected())
							}
							return nil
						}},
						Composite{Background: surface, Layout: VBox{MarginsZero: true, Spacing: 3, Alignment: AlignHNearVNear}, Children: []Widget{
							Label{AssignTo: &a.detailTitle, Text: "选择一个仓库", TextColor: dark, Font: Font{Family: nativeUIFontFamily, PointSize: 17, Bold: true}, EllipsisMode: EllipsisEnd},
							Composite{Background: surface, Layout: HBox{MarginsZero: true, Spacing: 3, Alignment: AlignHNearVCenter}, Children: []Widget{
								Label{AssignTo: &a.detailMeta, Text: "查看仓库信息和动态", TextColor: muted, Font: Font{Family: nativeUIFontFamily, PointSize: 10}, EllipsisMode: EllipsisEnd},
								PushButton{AssignTo: &a.detailLinkButton, Text: "↗", MinSize: Size{Width: 20, Height: 20}, MaxSize: Size{Width: 20, Height: 20}, Accessibility: Accessibility{Name: "打开仓库链接"}, OnClicked: func() { a.openLink(false) }},
								HSpacer{},
							}},
						}},
					}},
					TextEdit{AssignTo: &a.details, Background: surface, TextColor: walk.RGB(62, 72, 83), ReadOnly: true, VScroll: true, MinSize: Size{Height: 65}, Text: "选择仓库查看详情。"},
					CustomWidget{AssignTo: &a.detailTags, MinSize: Size{Height: 28}, MaxSize: Size{Height: 28}, Accessibility: Accessibility{Name: "暂无标签", Role: AccRoleStatictext}, PaintPixels: func(c *walk.Canvas, _ walk.Rectangle) error {
						if a.detailTags != nil {
							paintDetailTags(c, a.detailTags.ClientBoundsPixels(), a.detailTags.DPI(), a.detailTagValues)
						}
						return nil
					}},
					CustomWidget{AssignTo: &a.detailMetrics, Background: surface, MinSize: Size{Height: 36}, MaxSize: Size{Height: 36}, Accessibility: Accessibility{Name: "Stars、Forks、关注数", Role: AccRoleStatictext}, PaintPixels: func(c *walk.Canvas, _ walk.Rectangle) error {
						if a.detailMetrics != nil {
							paintDetailMetrics(c, a.detailMetrics.ClientBoundsPixels(), a.detailMetrics.DPI(), a.detailMetricText)
						}
						return nil
					}},
					CustomWidget{MinSize: Size{Height: 1}, MaxSize: Size{Height: 1}, PaintPixels: paintSeparator},
					CustomWidget{AssignTo: &a.detailFacts, Background: surface, MinSize: Size{Height: 145}, MaxSize: Size{Height: 155}, Accessibility: Accessibility{Name: "选择仓库查看详情", Role: AccRoleStatictext}, PaintPixels: func(c *walk.Canvas, _ walk.Rectangle) error {
						if a.detailFacts != nil {
							paintDetailFacts(c, a.detailFacts.ClientBoundsPixels(), a.detailFacts.DPI(), a.detailFactRows)
						}
						return nil
					}},
					CustomWidget{MinSize: Size{Height: 1}, MaxSize: Size{Height: 1}, PaintPixels: paintSeparator},
					Composite{Background: surface, MinSize: Size{Height: 25}, MaxSize: Size{Height: 25}, Layout: HBox{MarginsZero: true, Spacing: 2, Alignment: AlignHNearVCenter}, Children: []Widget{
						Label{Text: "最新发行版", TextColor: walk.RGB(27, 35, 42), Font: Font{Family: nativeCJKFontFamily, PointSize: 11, Bold: true}},
						HSpacer{},
						PushButton{AssignTo: &a.releasesAllButton, Text: "查看全部", MinSize: Size{Width: 70}, MaxSize: Size{Width: 78}, OnClicked: a.openAllReleases},
					}},
					CustomWidget{AssignTo: &a.detailRelease, Style: win.WS_TABSTOP, MinSize: Size{Height: 90}, MaxSize: Size{Height: 90}, Accessibility: Accessibility{Name: "暂无发行版", Role: AccRolePushbutton}, OnMouseDown: func(_, _ int, button walk.MouseButton) {
						if button == walk.LeftButton {
							a.openLink(true)
						}
					}, OnKeyDown: func(key walk.Key) {
						if key == walk.KeyReturn || key == walk.KeySpace {
							a.openLink(true)
						}
					}, PaintPixels: func(c *walk.Canvas, _ walk.Rectangle) error {
						if a.detailRelease != nil {
							paintReleaseCard(c, a.detailRelease.ClientBoundsPixels(), a.detailRelease.DPI(), a.detailReleaseText, a.detailReleaseNotes, a.detailReleaseDate)
						}
						return nil
					}},
					PushButton{AssignTo: &a.openButton, Text: "打开仓库", MinSize: Size{Height: 38}, OnClicked: func() { a.openLink(false) }},
					PushButton{AssignTo: &a.releaseButton, Text: "查看发行版", MinSize: Size{Height: 38}, OnClicked: a.openAllReleases},
					PushButton{AssignTo: &a.terminalButton, Text: "在终端打开", MinSize: Size{Height: 38}, OnClicked: a.openTerminal},
					PushButton{AssignTo: &a.manageButton, Text: "编辑仓库", MinSize: Size{Height: 38}, OnClicked: a.manageRepository},
				}},
				Composite{Row: 1, Column: 0, ColumnSpan: 2, Background: background, MinSize: Size{Height: 34}, MaxSize: Size{Height: 34}, Layout: HBox{Margins: Margins{Left: 18, Top: 3, Right: 18, Bottom: 3}, Spacing: 7, Alignment: AlignHNearVCenter}, Children: []Widget{
					Label{Text: "●", TextColor: walk.RGB(36, 137, 76)}, Label{AssignTo: &a.count, Text: "正在关注 0 个仓库", TextColor: muted}, Label{Text: "│", TextColor: walk.RGB(218, 224, 224)}, Label{AssignTo: &a.status, Text: "尚未刷新", TextColor: muted}, HSpacer{}, CheckBox{AssignTo: &a.pin, Text: "窗口置顶", Visible: false, OnCheckedChanged: func() { a.updatePin() }}, Label{Text: "✓  持续监控中", TextColor: muted},
				}},
			}},
		},
	}).Create()
	if err != nil {
		return err
	}
	// Walk creates its own empty status bar. The application already has a
	// custom footer, so leaving both visible wastes vertical space.
	a.window.StatusBar().SetVisible(false)
	styleNativeFrame(a.window, a.titleBar)
	roundNativePanel(a.centerFrame, 8)
	roundNativePanel(a.searchFrame, 14)
	roundNativePanel(a.searchInner, 12)
	centerNativeSearchEdit(a.search)
	for _, handle := range []win.HWND{a.search.Handle(), a.details.Handle()} {
		style := win.GetWindowLong(handle, win.GWL_EXSTYLE)
		win.SetWindowLong(handle, win.GWL_EXSTYLE, style&^win.WS_EX_CLIENTEDGE)
		win.SetWindowPos(handle, 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
	}
	if !highContrastEnabled() {
		style := win.GetWindowLong(a.search.Handle(), win.GWL_STYLE)
		win.SetWindowLong(a.search.Handle(), win.GWL_STYLE, style&^win.WS_BORDER)
		win.SetWindowPos(a.search.Handle(), 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
	}
	if grid, ok := a.window.Layout().(*walk.GridLayout); ok {
		// Grid keeps a nonzero stretch factor for the fixed-height title row.
		// Give the body the available height instead of leaving a bottom gap.
		_ = grid.SetRowStretchFactor(1, 100000)
	}
	if grid, ok := a.centerPanel.Layout().(*walk.GridLayout); ok {
		_ = grid.SetRowStretchFactor(2, 20)
	}
	a.table.SizeChanged().Attach(func() { _ = a.columnHeader.Invalidate() })
	a.details.SizeChanged().Attach(a.updateDescriptionScrollbar)
	a.bodyPanel.SizeChanged().Attach(a.updateCompactLayout)
	a.updateCompactLayout()
	if grid, ok := a.bodyPanel.Layout().(*walk.GridLayout); ok {
		_ = grid.SetColumnStretchFactor(1, 20)
		_ = grid.SetRowStretchFactor(0, 20)
	}
	for _, list := range []*walk.ListBox{a.table, a.filter} {
		style := win.GetWindowLong(list.Handle(), win.GWL_STYLE)
		win.SetWindowLong(list.Handle(), win.GWL_STYLE, style&^win.WS_BORDER)
		win.SetWindowPos(list.Handle(), 0, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)
	}
	styleNativeButtonIcon(a.addButton, true, "add")
	styleNativeButtonIcon(a.refreshButton, false, "refresh")
	styleNativeButtonIcon(a.settingsButton, false, "settings")
	if p, ok := nativeButtonPalettes[a.settingsButton.Handle()]; ok {
		p.caption = true
		p.label = ""
		nativeButtonPalettes[a.settingsButton.Handle()] = p
	}
	styleNativeButtonIcon(a.openButton, true, "open")
	styleNativeButtonIcon(a.detailLinkButton, false, "open")
	if p, ok := nativeButtonPalettes[a.detailLinkButton.Handle()]; ok {
		p.flat = true
		p.label = ""
		nativeButtonPalettes[a.detailLinkButton.Handle()] = p
	}
	styleNativeButtonIcon(a.releaseButton, false, "release")
	styleNativeLinkButton(a.releasesAllButton)
	styleNativeButtonIcon(a.terminalButton, false, "terminal")
	styleNativeButtonIcon(a.manageButton, false, "edit")
	styleNativeButtonIcon(a.sortDirection, false, "sort")
	styleNativeSortButton(a.sortButton)
	if p, ok := nativeButtonPalettes[a.sortDirection.Handle()]; ok {
		p.label = ""
		nativeButtonPalettes[a.sortDirection.Handle()] = p
	}
	styleCaptionButton(a.minimizeButton, "最小化", "minimize")
	styleCaptionButton(a.maximizeButton, "最大化或还原", "maximize")
	styleCaptionButton(a.closeButton, "关闭窗口", "close")
	a.loadingSettings = true
	a.pin.SetChecked(loadPinSetting(a.settingsPath))
	a.loadingSettings = false
	icon, err := walk.NewIconFromImage(img)
	if err != nil {
		return err
	}
	if err = a.window.SetIcon(icon); err != nil {
		return err
	}
	tray, err := walk.NewNotifyIcon(a.window)
	if err != nil {
		return err
	}
	a.tray = tray
	if err = tray.SetIcon(icon); err != nil {
		return err
	}
	_ = tray.SetToolTip("Repo Watch · 后台监控中")
	tray.MouseDown().Attach(func(_, _ int, b walk.MouseButton) {
		if b == walk.LeftButton {
			a.show()
		}
	})
	for _, item := range []struct {
		text string
		fn   func()
	}{{"打开 Repo Watch", a.show}, {"立即刷新", func() { a.monitor.Trigger() }}, {"退出 Repo Watch", func() { a.quitting = true; a.window.Close() }}} {
		action := walk.NewAction()
		_ = action.SetText(item.text)
		action.Triggered().Attach(item.fn)
		if err = tray.ContextMenu().Actions().Add(action); err != nil {
			return err
		}
	}
	if err = tray.SetVisible(true); err != nil {
		return err
	}
	a.window.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		if !a.quitting {
			*canceled = true
			a.window.Hide()
		}
	})
	return nil
}
func (a *nativeApp) updateCompactLayout() {
	if a.bodyPanel == nil || a.inspector == nil || a.detailHeader == nil || a.detailIcon == nil || a.details == nil || a.detailTags == nil || a.detailMetrics == nil || a.detailFacts == nil || a.detailRelease == nil {
		return
	}
	height := a.bodyPanel.ClientBounds().Height
	if height <= 0 {
		return
	}
	compact := height < 690
	if a.compactLayoutSet && compact == a.compactLayout {
		return
	}
	a.compactLayout, a.compactLayoutSet = compact, true
	header, descMin, tags, metrics, factsMin, factsMax, release, button, spacing := 65, 65, 28, 36, 145, 155, 90, 38, 8
	iconSize := 54
	if compact {
		header, descMin, tags, metrics, factsMin, factsMax, release, button, spacing = 50, 50, 24, 26, 100, 110, 56, 30, 4
		iconSize = 42
	}
	_ = a.detailHeader.SetMinMaxSize(walk.Size{Height: header}, walk.Size{Height: header})
	_ = a.detailIcon.SetMinMaxSize(walk.Size{Width: iconSize, Height: iconSize}, walk.Size{Width: iconSize, Height: iconSize})
	_ = a.details.SetMinMaxSize(walk.Size{Height: descMin}, walk.Size{})
	_ = a.detailTags.SetMinMaxSize(walk.Size{Height: tags}, walk.Size{Height: tags})
	_ = a.detailMetrics.SetMinMaxSize(walk.Size{Height: metrics}, walk.Size{Height: metrics})
	_ = a.detailFacts.SetMinMaxSize(walk.Size{Height: factsMin}, walk.Size{Height: factsMax})
	_ = a.detailRelease.SetMinMaxSize(walk.Size{Height: release}, walk.Size{Height: release})
	for _, b := range []*walk.PushButton{a.openButton, a.releaseButton, a.terminalButton, a.manageButton} {
		_ = b.SetMinMaxSize(walk.Size{Height: button}, walk.Size{})
	}
	if layout, ok := a.inspector.Layout().(*walk.BoxLayout); ok {
		_ = layout.SetSpacing(spacing)
		// Give remaining height to the description instead of spreading it
		// between the release card and the action buttons.
		_ = layout.SetStretchFactor(a.details, 100000)
	}
}
func (a *nativeApp) show() {
	a.window.Show()
	win.ShowWindow(a.window.Handle(), win.SW_RESTORE)
	win.SetForegroundWindow(a.window.Handle())
}
func (a *nativeApp) openSettings() {
	var dialog *walk.Dialog
	var pin *walk.CheckBox
	var done *walk.PushButton
	view := Dialog{Font: Font{Family: nativeCJKFontFamily, PointSize: 10}, AssignTo: &dialog, Title: "设置", Size: Size{Width: 360, Height: 210}, MinSize: Size{Width: 330, Height: 190}, Layout: VBox{Margins: Margins{Left: 20, Top: 16, Right: 20, Bottom: 16}, Spacing: 10}, CancelButton: &done, Children: []Widget{
		Label{Text: "窗口设置"},
		CheckBox{AssignTo: &pin, Text: "窗口置顶", OnCheckedChanged: func() {
			if pin != nil {
				a.pin.SetChecked(pin.Checked())
			}
		}},
		PushButton{Text: "字体与图标许可", OnClicked: func() { a.showIconLicenses(dialog) }},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &done, Text: "完成", OnClicked: func() { dialog.Accept() }}}},
	}}
	if err := view.Create(a.window); err != nil {
		walk.MsgBox(a.window, "设置", err.Error(), walk.MsgBoxIconError)
		return
	}
	pin.SetChecked(a.pin.Checked())
	dialog.Run()
}
func (a *nativeApp) showIconLicenses(owner walk.Form) {
	var sections []string
	if content, err := bundledFonts.ReadFile("assets/fonts/inter/LICENSE.txt"); err == nil {
		sections = append(sections, "Inter (SIL Open Font License 1.1)\r\n\r\n"+strings.ReplaceAll(string(content), "\n", "\r\n"))
	}
	for _, resource := range []struct{ title, path string }{
		{"Lucide Icons 0.468.0", "assets/icons/lucide/LICENSE"},
		{"Feather Icons", "assets/icons/lucide/FEATHER-LICENSE"},
		{"Simple Icons 13.21.0", "assets/icons/simple-icons/LICENSE.md"},
		{"Simple Icons disclaimer", "assets/icons/simple-icons/DISCLAIMER.md"},
	} {
		if content, err := iconAssets.ReadFile(resource.path); err == nil {
			sections = append(sections, resource.title+"\r\n\r\n"+strings.ReplaceAll(string(content), "\n", "\r\n"))
		}
	}
	var dialog *walk.Dialog
	var closeButton *walk.PushButton
	view := Dialog{Font: Font{Family: nativeCJKFontFamily, PointSize: 10}, AssignTo: &dialog, Title: "字体与图标许可", Size: Size{Width: 650, Height: 500}, MinSize: Size{Width: 480, Height: 320}, Layout: VBox{}, CancelButton: &closeButton, Children: []Widget{
		TextEdit{Text: strings.Join(sections, "\r\n\r\n"), ReadOnly: true, VScroll: true},
		Composite{Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &closeButton, Text: "关闭", OnClicked: func() { dialog.Cancel() }}}},
	}}
	if err := view.Create(owner); err != nil {
		walk.MsgBox(owner, "图标许可", err.Error(), walk.MsgBoxIconError)
		return
	}
	dialog.Run()
}
func (a *nativeApp) updatePin() {
	after := win.HWND_NOTOPMOST
	if a.pin.Checked() {
		after = win.HWND_TOPMOST
	}
	win.SetWindowPos(a.window.Handle(), after, 0, 0, 0, 0, win.SWP_NOMOVE|win.SWP_NOSIZE)
	if !a.loadingSettings {
		if err := savePinSetting(a.settingsPath, a.pin.Checked()); err != nil {
			walk.MsgBox(a.window, "设置保存失败", err.Error(), walk.MsgBoxIconWarning)
		}
	}
}
func (a *nativeApp) refreshView() {
	all := a.monitor.List()
	data, _ := json.Marshal(all)
	if string(data) == a.last {
		return
	}
	a.last = string(data)
	a.all = all
	tags := map[string]int{}
	for _, r := range all {
		for _, tag := range r.Tags {
			tags[tag]++
		}
	}
	var list []string
	for tag := range tags {
		list = append(list, tag)
	}
	sort.Strings(list)
	a.filters = append([]string{""}, list...)
	labels := []string{fmt.Sprintf("全部仓库   %d", len(all))}
	for _, tag := range list {
		labels = append(labels, fmt.Sprintf("%s   %d", tag, tags[tag]))
	}
	a.filterLabels = labels
	idx := 0
	for i, t := range a.filters {
		if t == a.selectedTag {
			idx = i
		}
	}
	a.selectedTag = a.filters[idx]
	a.rebuilding = true
	_ = a.filter.SetModel(labels)
	_ = a.filter.SetCurrentIndex(idx)
	a.rebuilding = false
	a.renderRows()
	_ = a.count.SetText(fmt.Sprintf("正在关注 %d 个仓库", len(all)))
	latest := time.Time{}
	for _, r := range all {
		if r.Snapshot != nil {
			checked, err := time.Parse(time.RFC3339, r.Snapshot.CheckedAt)
			if err == nil && checked.After(latest) {
				latest = checked
			}
		}
	}
	status := "尚未刷新"
	if !latest.IsZero() {
		status = "最近刷新：" + latest.Local().Format("2006-01-02 15:04")
	}
	_ = a.status.SetText(status)
}
func (a *nativeApp) renderRows() {
	id := ""
	if r := a.selected(); r != nil {
		id = r.ID
	}
	query := ""
	if a.search != nil {
		query = a.search.Text()
	}
	items := searchRepositories(filterRepositories(a.all, a.selectedTag), query)
	order := "updated"
	{
		switch a.sortIndex {
		case 1:
			order = "name"
		case 2:
			order = "stars"
		case 3:
			order = "forks"
		}
	}
	a.model.items = sortRepositories(items, order)
	if a.reverseSort {
		for i, j := 0, len(a.model.items)-1; i < j; i, j = i+1, j-1 {
			a.model.items[i], a.model.items[j] = a.model.items[j], a.model.items[i]
		}
	}
	a.model.PublishItemsReset()
	_ = a.columnHeader.Invalidate()
	index := -1
	for i, r := range a.model.items {
		if r.ID == id {
			index = i
			break
		}
	}
	if index < 0 && len(a.model.items) > 0 {
		index = 0
	}
	_ = a.table.SetCurrentIndex(index)
	a.updateDetails()
}

func (a *nativeApp) sortByHeader(x int) {
	columns := repositoryColumns(a.table.ClientBoundsPixels().Width, a.table.DPI())
	column := -1
	for i := 0; i < len(columns)-1; i++ {
		if x >= columns[i] && x < columns[i+1] {
			column = i
			break
		}
	}
	order, ok := map[int]int{0: 1, 3: 2, 4: 3, 5: 0}[column]
	if !ok {
		return
	}
	if a.sortIndex == order {
		a.reverseSort = !a.reverseSort
		a.updateSortDirectionLabel()
		a.renderRows()
	} else {
		a.reverseSort = false
		a.updateSortDirectionLabel()
		a.setSortIndex(order)
	}
}

var nativeSortLabels = []string{"排序：最近更新", "排序：仓库名称", "排序：Stars", "排序：Forks"}
var appendNativeMenu = windows.NewLazySystemDLL("user32.dll").NewProc("AppendMenuW")

// The button and popup are native Windows controls; only the button face is
// styled so its border matches the search field.
func (a *nativeApp) showSortMenu() {
	menu := win.CreatePopupMenu()
	if menu == 0 {
		return
	}
	defer win.DestroyMenu(menu)
	for i, label := range nativeSortLabels {
		text, err := windows.UTF16PtrFromString(label)
		if err != nil {
			return
		}
		flags := uintptr(0) // MF_STRING
		if i == a.sortIndex {
			flags = 0x8 // MF_CHECKED
		}
		ok, _, _ := appendNativeMenu.Call(uintptr(menu), flags, uintptr(i+1), uintptr(unsafe.Pointer(text)))
		if ok == 0 {
			return
		}
	}
	var rect win.RECT
	if !win.GetWindowRect(a.sortButton.Handle(), &rect) {
		return
	}
	choice := win.TrackPopupMenu(menu, win.TPM_LEFTALIGN|win.TPM_TOPALIGN|win.TPM_RETURNCMD, rect.Left, rect.Bottom, 0, a.window.Handle(), nil)
	if choice > 0 && int(choice) <= len(nativeSortLabels) {
		a.setSortIndex(int(choice) - 1)
	}
}

func (a *nativeApp) setSortIndex(index int) {
	if index < 0 || index >= len(nativeSortLabels) {
		return
	}
	a.sortIndex = index
	_ = a.sortButton.SetText(nativeSortLabels[index])
	if p, ok := nativeButtonPalettes[a.sortButton.Handle()]; ok {
		p.label = nativeSortLabels[index]
		nativeButtonPalettes[a.sortButton.Handle()] = p
		_ = a.sortButton.Invalidate()
	}
	if a.table != nil {
		a.renderRows()
	}
}

func (a *nativeApp) updateSortDirectionLabel() {
	label := "切换排序方向，当前为默认"
	if a.reverseSort {
		label = "切换排序方向，当前为反向"
	}
	_ = a.sortDirection.Accessibility().SetName(label)
}
func (a *nativeApp) selected() *Repository {
	if a.table == nil {
		return nil
	}
	i := a.table.CurrentIndex()
	if i < 0 || i >= len(a.model.items) {
		return nil
	}
	return &a.model.items[i]
}
func (a *nativeApp) updateDetails() {
	if a.details == nil || a.detailTitle == nil || a.detailMeta == nil || a.detailMetrics == nil || a.detailTags == nil || a.detailFacts == nil || a.detailRelease == nil {
		return
	}
	r := a.selected()
	for _, button := range []*walk.PushButton{a.openButton, a.detailLinkButton, a.releaseButton, a.releasesAllButton, a.terminalButton, a.manageButton} {
		if button != nil {
			button.SetEnabled(r != nil)
		}
	}
	if r == nil {
		a.detailRelease.SetEnabled(false)
		a.setOpenButtonPlatform("")
		setContentFont(a.detailTitle, "选择一个仓库", 17, walk.FontBold)
		setContentFont(a.detailMeta, "查看仓库信息和动态", 10, 0)
		_ = a.detailTitle.SetText("选择一个仓库")
		_ = a.detailMeta.SetText("查看仓库信息和动态")
		fitDetailMeta(a.detailMeta, "查看仓库信息和动态")
		a.setDetailMetrics([3]string{"—", "—", "—"})
		a.detailTagValues = nil
		_ = a.detailTags.Accessibility().SetName("暂无标签")
		_ = a.detailTags.Invalidate()
		_ = a.detailIcon.Invalidate()
		a.setDetailFacts(nil, "选择仓库查看监控信息")
		a.setReleaseText("暂无发行版", "", "")
		message := "选择仓库查看详情。"
		if len(a.all) == 0 {
			message = "还没有仓库，点击右上角添加。"
		} else if len(a.model.items) == 0 {
			message = "没有匹配的仓库，请调整搜索或标签筛选。"
		}
		setContentFont(a.details, message, 11, 0)
		_ = a.details.SetText(message)
		a.updateDescriptionScrollbar()
		_ = a.details.Invalidate()
		return
	}
	s := r.snapshotValue()
	a.detailRelease.SetEnabled(s.ReleaseURL != "")
	a.setOpenButtonPlatform(r.Platform)
	if a.releaseButton != nil {
		a.releaseButton.SetEnabled(repositoryReleasesURL(s.URL) != "")
	}
	if a.releasesAllButton != nil {
		a.releasesAllButton.SetEnabled(repositoryReleasesURL(s.URL) != "")
	}
	setContentFont(a.detailTitle, r.Repo, 17, walk.FontBold)
	setContentFont(a.detailMeta, r.Owner+" / "+r.Repo, 10, 0)
	_ = a.detailTitle.SetText(r.Repo)
	_ = a.detailMeta.SetText(r.Owner + " / " + r.Repo)
	fitDetailMeta(a.detailMeta, r.Owner+" / "+r.Repo)
	metrics := [3]string{"—", "—", "—"}
	if r.Snapshot != nil {
		metrics[0], metrics[1] = compactCount(s.Stars), compactCount(s.Forks)
		if s.Watchers != nil {
			metrics[2] = compactCount(*s.Watchers)
		}
	}
	a.setDetailMetrics(metrics)
	description := s.Description
	if description == "" {
		description = "暂无项目描述"
	}
	tags := strings.Join(r.Tags, ", ")
	if tags == "" {
		tags = "暂无标签"
	}
	release := s.Release
	if release == "" {
		release = "暂无发行版"
	}
	updated := relativeUpdate(s.PushedAt, time.Now())
	checked := formatTime(s.CheckedAt)
	state := r.Change
	if r.Error != "" {
		state = r.Error
	}
	if state == "" {
		state = "等待首次刷新"
	}
	setContentFont(a.details, description, 11, 0)
	_ = a.details.SetText(description)
	a.updateDescriptionScrollbar()
	_ = a.details.Invalidate()
	a.detailTagValues = append([]string(nil), r.Tags...)
	_ = a.detailTags.Accessibility().SetName("标签：" + tags)
	_ = a.detailTags.Invalidate()
	_ = a.detailIcon.Invalidate()
	value := func(s string) string {
		if strings.TrimSpace(s) == "" {
			return "—"
		}
		return s
	}
	created := "—"
	if s.CreatedAt != "" {
		created = formatDate(s.CreatedAt)
	}
	platform := map[string]string{"github": "GitHub", "gitea": "Gitea", "codeberg": "Codeberg"}[r.Platform]
	if platform == "" {
		platform = r.Platform
	}
	a.setDetailFacts([]detailFact{{"平台", value(platform)}, {"语言", value(s.Language)}, {"许可证", value(s.License)}, {"创建时间", created}, {"最近更新", updated}}, fmt.Sprintf("最近检查：%s\n监控动态：%s", checked, state))
	a.setReleaseText(release, s.ReleaseNotes, relativeUpdate(s.ReleasePublishedAt, time.Now()))
}

func setContentFont(w walk.Window, text string, size int, style walk.FontStyle) {
	font, err := walk.NewFont(fontFamilyForText(text), size, style)
	if err == nil {
		w.SetFont(font)
	}
}

func fitDetailMeta(label *walk.Label, text string) {
	if label == nil {
		return
	}
	hdc := win.GetDC(label.Handle())
	if hdc == 0 {
		return
	}
	defer win.ReleaseDC(label.Handle(), hdc)
	font := win.SendMessage(label.Handle(), win.WM_GETFONT, 0, 0)
	old := win.SelectObject(hdc, win.HGDIOBJ(font))
	defer win.SelectObject(hdc, old)
	units := windows.StringToUTF16(text)
	var size win.SIZE
	if len(units) > 1 && win.GetTextExtentPoint32(hdc, &units[0], int32(len(units)-1), &size) {
		width := walk.IntTo96DPI(int(size.CX)+12, label.DPI())
		if width > 210 {
			width = 210
		}
		_ = label.SetMinMaxSize(walk.Size{Width: width}, walk.Size{Width: width})
		label.Parent().RequestLayout()
	}
}

func (a *nativeApp) setOpenButtonPlatform(platform string) {
	label, icon := "打开仓库", "open"
	switch platform {
	case "github":
		label, icon = "在 GitHub 打开", "github"
	case "gitea":
		label, icon = "在 Gitea 打开", "gitea"
	case "codeberg":
		label, icon = "在 Codeberg 打开", "codeberg"
	}
	_ = a.openButton.SetText(label)
	if palette, ok := nativeButtonPalettes[a.openButton.Handle()]; ok {
		palette.label, palette.icon = label, icon
		nativeButtonPalettes[a.openButton.Handle()] = palette
		_ = a.openButton.Invalidate()
	}
}

var showNativeScrollbar = windows.NewLazySystemDLL("user32.dll").NewProc("ShowScrollBar")

func (a *nativeApp) updateDescriptionScrollbar() {
	if a.details == nil || a.updatingScrollbar || highContrastEnabled() {
		return
	}
	a.updatingScrollbar = true
	defer func() { a.updatingScrollbar = false }()
	// Restoring the bar first lets the edit control recompute its range after
	// changing from a short description to a longer one.
	_, _, _ = showNativeScrollbar.Call(uintptr(a.details.Handle()), uintptr(win.SB_VERT), 1)
	info := win.SCROLLINFO{CbSize: uint32(unsafe.Sizeof(win.SCROLLINFO{})), FMask: win.SIF_RANGE | win.SIF_PAGE}
	if !win.GetScrollInfo(a.details.Handle(), win.SB_VERT, &info) {
		return
	}
	show := uintptr(0)
	if int64(info.NMax)-int64(info.NMin)+1 > int64(info.NPage) && info.NPage > 0 {
		show = 1
	}
	_, _, _ = showNativeScrollbar.Call(uintptr(a.details.Handle()), uintptr(win.SB_VERT), show)
}
func (a *nativeApp) setDetailMetrics(values [3]string) {
	a.detailMetricText = values
	_ = a.detailMetrics.Accessibility().SetName(fmt.Sprintf("Stars %s，Forks %s，关注 %s", values[0], values[1], values[2]))
	_ = a.detailMetrics.Invalidate()
}
func (a *nativeApp) setDetailFacts(rows []detailFact, extra string) {
	a.detailFactRows = rows
	var text []string
	for _, row := range rows {
		text = append(text, row.label+"："+row.value)
	}
	text = append(text, extra)
	_ = a.detailFacts.Accessibility().SetName(strings.Join(text, "，"))
	_ = a.detailFacts.SetToolTipText(strings.Join(text, "\n"))
	_ = a.detailFacts.Invalidate()
}
func (a *nativeApp) setReleaseText(text, notes, date string) {
	notes = releaseSummary(notes)
	a.detailReleaseText = text
	a.detailReleaseNotes = notes
	a.detailReleaseDate = date
	_ = a.detailRelease.Accessibility().SetName("最新发行版：" + text + "，" + date + "，" + notes)
	_ = a.detailRelease.SetToolTipText(strings.TrimSpace(text + "\n" + date + "\n" + notes))
	_ = a.detailRelease.Invalidate()
}
func formatDate(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.Local().Format("2006-01-02")
}
func formatTime(value string) string {
	if value == "" {
		return "暂无记录"
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.Local().Format("2006-01-02 15:04")
}
func (a *nativeApp) openLink(release bool) {
	r := a.selected()
	if r == nil {
		return
	}
	s := r.snapshotValue()
	link := s.URL
	if release {
		link = s.ReleaseURL
	}
	u, err := url.Parse(link)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		walk.MsgBox(a.window, "Repo Watch", "暂无可打开的链接，请先刷新仓库。", walk.MsgBoxIconInformation)
		return
	}
	win.ShellExecute(a.window.Handle(), windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(link), nil, nil, win.SW_SHOWNORMAL)
}
func (a *nativeApp) openAllReleases() {
	r := a.selected()
	if r == nil {
		return
	}
	link := repositoryReleasesURL(r.snapshotValue().URL)
	if link == "" {
		return
	}
	win.ShellExecute(a.window.Handle(), windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(link), nil, nil, win.SW_SHOWNORMAL)
}
func (a *nativeApp) copyRepositoryURL() {
	r := a.selected()
	if r == nil {
		return
	}
	address := r.snapshotValue().URL
	if address == "" {
		walk.MsgBox(a.window, "Repo Watch", "暂无仓库地址，请先刷新。", walk.MsgBoxIconInformation)
		return
	}
	if err := walk.Clipboard().SetText(address); err != nil {
		walk.MsgBox(a.window, "复制失败", err.Error(), walk.MsgBoxIconError)
		return
	}
	_ = a.status.SetText("仓库地址已复制")
}

func (a *nativeApp) chooseLocalDirectory() bool {
	r := a.selected()
	if r == nil {
		return false
	}
	id := r.ID
	dialog := walk.FileDialog{Title: "选择仓库的本地目录"}
	accepted, err := dialog.ShowBrowseFolder(a.window)
	if err != nil {
		walk.MsgBox(a.window, "选择目录失败", err.Error(), walk.MsgBoxIconError)
		return false
	}
	if !accepted {
		return false
	}
	if err = a.monitor.UpdateLocalPath(id, dialog.FilePath); err != nil {
		walk.MsgBox(a.window, "保存目录失败", err.Error(), walk.MsgBoxIconError)
		return false
	}
	a.refreshView()
	return true
}

func (a *nativeApp) openTerminal() {
	r := a.selected()
	if r == nil {
		return
	}
	if r.LocalPath == "" {
		if !a.chooseLocalDirectory() {
			return
		}
		r = a.selected()
		if r == nil {
			return
		}
	}
	info, err := os.Stat(r.LocalPath)
	if err != nil || !info.IsDir() {
		walk.MsgBox(a.window, "本地目录不可用", "保存的目录不存在或无法访问，请重新关联本地目录。", walk.MsgBoxIconWarning)
		return
	}
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	// ShellExecute creates an interactive console instead of passing the GUI
	// process's null standard input to PowerShell. The directory is supplied as
	// a separate Windows argument, never interpolated into a command line.
	result, _, callErr := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW").Call(
		uintptr(a.window.Handle()), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("open"))),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(powershell))), 0,
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(r.LocalPath))), uintptr(win.SW_SHOWNORMAL))
	if result <= 32 {
		walk.MsgBox(a.window, "打开终端失败", fmt.Sprintf("无法启动 PowerShell（代码 %d）：%v", result, callErr), walk.MsgBoxIconError)
	}
}
func (a *nativeApp) manageRepository() {
	r := a.selected()
	if r == nil {
		return
	}
	var dialog *walk.Dialog
	var cancel *walk.PushButton
	d := Dialog{Font: Font{Family: nativeCJKFontFamily, PointSize: 10}, AssignTo: &dialog, Title: "管理仓库 · " + r.Owner + "/" + r.Repo, Size: Size{Width: 380, Height: 350}, MinSize: Size{Width: 340, Height: 330}, Layout: VBox{Margins: Margins{Left: 20, Top: 16, Right: 20, Bottom: 16}, Spacing: 8}, CancelButton: &cancel, Children: []Widget{
		Label{Text: "选择要管理的内容"},
		PushButton{Text: "编辑标签", OnClicked: func() { dialog.Accept(); a.editTags() }},
		PushButton{Text: "管理访问令牌", OnClicked: func() { dialog.Accept(); a.editCredential() }},
		PushButton{Text: "关联本地目录", OnClicked: func() { dialog.Accept(); a.chooseLocalDirectory() }},
		PushButton{Text: "解除本地目录", Enabled: r.LocalPath != "", OnClicked: func() {
			id := r.ID
			dialog.Accept()
			if err := a.monitor.UpdateLocalPath(id, ""); err != nil {
				walk.MsgBox(a.window, "保存目录失败", err.Error(), walk.MsgBoxIconError)
				return
			}
			a.refreshView()
		}},
		PushButton{Text: "复制仓库地址", OnClicked: func() { dialog.Accept(); a.copyRepositoryURL() }},
		PushButton{Text: "移除仓库", OnClicked: func() { dialog.Accept(); a.removeRepository() }},
		PushButton{AssignTo: &cancel, Text: "取消", OnClicked: func() { dialog.Cancel() }},
	}}
	if err := d.Create(a.window); err != nil {
		walk.MsgBox(a.window, "管理仓库", err.Error(), walk.MsgBoxIconError)
		return
	}
	dialog.Run()
}
func (a *nativeApp) removeRepository() {
	r := a.selected()
	if r == nil {
		return
	}
	if walk.MsgBox(a.window, "移除仓库", "确认移除 "+r.Owner+"/"+r.Repo+" 及其本机令牌？", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
		return
	}
	if err := a.service.RemoveRepository(*r); err != nil {
		walk.MsgBox(a.window, "移除失败", err.Error(), walk.MsgBoxIconError)
	}
	a.refreshView()
}

func (a *nativeApp) addRepository() {
	var d *walk.Dialog
	var address, owner, repo, base, token, tags *walk.LineEdit
	var platform *walk.ComboBox
	var add, cancel *walk.PushButton
	dialog := Dialog{Font: Font{Family: nativeCJKFontFamily, PointSize: 10}, AssignTo: &d, Title: "添加仓库", MinSize: Size{Width: 460, Height: 410}, Size: Size{Width: 500, Height: 430}, Layout: VBox{}, DefaultButton: &add, CancelButton: &cancel, Children: []Widget{
		Label{Text: "仓库链接（可选，填写后自动识别平台与仓库）"}, LineEdit{AssignTo: &address},
		Composite{Layout: Grid{Columns: 2}, Children: []Widget{Label{Text: "平台"}, ComboBox{AssignTo: &platform, Model: []string{"GitHub", "Codeberg", "Gitea（自建）"}}, Label{Text: "Owner"}, LineEdit{AssignTo: &owner}, Label{Text: "Repository"}, LineEdit{AssignTo: &repo}, Label{Text: "Gitea 实例地址"}, LineEdit{AssignTo: &base}, Label{Text: "标签（逗号分隔）"}, LineEdit{AssignTo: &tags}, Label{Text: "访问令牌（可选）"}, LineEdit{AssignTo: &token, PasswordMode: true}}},
		Label{Text: "令牌保存在 Windows 凭据管理器中；私有仓库需使用 HTTPS。"},
		Composite{Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &cancel, Text: "取消", OnClicked: func() { d.Cancel() }}, PushButton{AssignTo: &add, Text: "添加", OnClicked: func() {
			var req RepoRequest
			var err error
			if strings.TrimSpace(address.Text()) != "" {
				req, err = parseRepositoryURL(address.Text())
			} else {
				names := []string{"github", "codeberg", "gitea"}
				i := platform.CurrentIndex()
				if i < 0 || i >= len(names) {
					walk.MsgBox(d, "添加失败", "请选择平台", walk.MsgBoxIconError)
					return
				}
				req = RepoRequest{Platform: names[i], Owner: strings.TrimSpace(owner.Text()), Repo: strings.TrimSpace(repo.Text())}
				if req.Platform == "gitea" {
					req.BaseURL = strings.TrimRight(strings.TrimSpace(base.Text()), "/")
				}
				err = validateRequest(req)
			}
			if err != nil {
				walk.MsgBox(d, "添加失败", err.Error(), walk.MsgBoxIconError)
				return
			}
			var id [16]byte
			if _, err = rand.Read(id[:]); err != nil {
				walk.MsgBox(d, "添加失败", err.Error(), walk.MsgBoxIconError)
				return
			}
			r := Repository{ID: hex.EncodeToString(id[:]), Platform: req.Platform, BaseURL: req.BaseURL, Owner: req.Owner, Repo: req.Repo, Tags: splitTags(tags.Text())}
			if err = a.service.AddRepository(r, token.Text()); err != nil {
				walk.MsgBox(d, "添加失败", err.Error(), walk.MsgBoxIconError)
				return
			}
			_ = token.SetText("")
			d.Accept()
			a.refreshView()
		}}}},
	}}
	if err := dialog.Create(a.window); err != nil {
		walk.MsgBox(a.window, "添加失败", err.Error(), walk.MsgBoxIconError)
		return
	}
	_ = platform.SetCurrentIndex(0)
	d.Run()
}
func splitTags(value string) []string {
	return normalizeTags(strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' }))
}
func (a *nativeApp) editTags() {
	r := a.selected()
	if r == nil {
		return
	}
	id := r.ID
	var d *walk.Dialog
	var field *walk.LineEdit
	var save, cancel *walk.PushButton
	dialog := Dialog{Font: Font{Family: nativeCJKFontFamily, PointSize: 10}, AssignTo: &d, Title: "编辑标签 · " + r.Owner + "/" + r.Repo, Size: Size{Width: 450, Height: 160}, Layout: VBox{}, DefaultButton: &save, CancelButton: &cancel, Children: []Widget{Label{Text: "多个标签用逗号分隔，留空可清除全部标签。"}, LineEdit{AssignTo: &field, Text: strings.Join(r.Tags, ", ")}, Composite{Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &cancel, Text: "取消", OnClicked: func() { d.Cancel() }}, PushButton{AssignTo: &save, Text: "保存", OnClicked: func() {
		if err := a.service.UpdateRepositoryTags(id, splitTags(field.Text())); err != nil {
			walk.MsgBox(d, "保存失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		d.Accept()
		a.refreshView()
	}}}}}}
	if err := dialog.Create(a.window); err != nil {
		walk.MsgBox(a.window, "编辑标签", err.Error(), walk.MsgBoxIconError)
		return
	}
	d.Run()
}
func (a *nativeApp) editCredential() {
	r := a.selected()
	if r == nil {
		return
	}
	req := r.request()
	var d *walk.Dialog
	var token *walk.LineEdit
	var save, cancel *walk.PushButton
	dialog := Dialog{Font: Font{Family: nativeCJKFontFamily, PointSize: 10}, AssignTo: &d, Title: "访问令牌 · " + r.Owner + "/" + r.Repo, Size: Size{Width: 480, Height: 190}, Layout: VBox{}, DefaultButton: &save, CancelButton: &cancel, Children: []Widget{Label{Text: "输入新令牌以设置或替换；已保存的令牌不会显示。"}, LineEdit{AssignTo: &token, PasswordMode: true}, Label{Text: "令牌保存在 Windows 凭据管理器，只通过 HTTPS 发送。"}, Composite{Layout: HBox{}, Children: []Widget{PushButton{Text: "清除令牌", OnClicked: func() {
		if walk.MsgBox(d, "清除令牌", "确定清除该仓库的本机令牌？", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
			return
		}
		if err := a.service.ClearCredential(req); err != nil {
			walk.MsgBox(d, "清除失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		d.Accept()
		a.monitor.Trigger()
	}}, HSpacer{}, PushButton{AssignTo: &cancel, Text: "取消", OnClicked: func() { d.Cancel() }}, PushButton{AssignTo: &save, Text: "保存", OnClicked: func() {
		if err := a.service.SaveCredential(req, token.Text()); err != nil {
			walk.MsgBox(d, "保存失败", err.Error(), walk.MsgBoxIconError)
			return
		}
		_ = token.SetText("")
		d.Accept()
		a.monitor.Trigger()
	}}}}}}
	if err := dialog.Create(a.window); err != nil {
		walk.MsgBox(a.window, "访问令牌", err.Error(), walk.MsgBoxIconError)
		return
	}
	d.Run()
}
