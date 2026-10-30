package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

//go:embed all:frontend
var frontend embed.FS

//go:embed assets/repowatch-logo.png
var appIcon []byte

//go:embed assets/repowatch-tray.png
var trayIcon []byte

func main() {
	mygo.App.SetName("RepoWatch")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	monitor, err := newMonitor(filepath.Join(dir, "RepoWatch", "monitor.json"), fetchWithCredential, func(title, body string) error {
		if !mygo.NotificationsSupported() {
			return fmtNotificationUnsupported()
		}
		return mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body}).Show()
	})
	if err != nil {
		log.Fatal(err)
	}
	files, err := fs.Sub(frontend, "frontend")
	if err != nil {
		log.Fatal(err)
	}
	mygo.SetFrontend(files)
	service := &Service{monitor: monitor}
	mygo.Bind(service)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var win *mygo.Window
	var tray *mygo.Tray
	quitting := false
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })
	mygo.App.OnQuit(func() { cancel() })
	show := func() {
		if win != nil && !win.IsDestroyed() {
			if win.IsMinimized() {
				win.Restore()
			}
			win.Show()
		}
	}
	mygo.App.OnSecondInstance(func([]string, string) { show() })
	mygo.App.OnNotificationClick(func(string) { show() })
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{Title: "Repo Watch", Width: 400, Height: 640, MinWidth: 350, MinHeight: 440, URL: "/"})
		if err := win.SetIcon(appIcon); err != nil {
			log.Printf("设置窗口图标失败：%v", err)
		}
		service.setPinned = win.SetAlwaysOnTop
		var err error
		tray, err = mygo.NewTray(mygo.TrayOptions{ID: "repo-watch", Icon: trayIcon, ToolTip: "Repo Watch · 后台监控中", Menu: mygo.NewMenu([]*mygo.MenuItem{
			{Label: "打开 Repo Watch", Click: func(*mygo.MenuItem, *mygo.Window) { show() }},
			{Label: "立即刷新", Click: func(*mygo.MenuItem, *mygo.Window) { monitor.Trigger() }},
			mygo.Separator(),
			{Label: "退出 Repo Watch", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		})})
		if err != nil {
			log.Printf("托盘不可用：%v", err)
		} else {
			tray.OnClick(show)
			win.OnClose(func(e *mygo.CloseEvent) {
				if !quitting {
					e.PreventDefault()
					win.Hide()
				}
			})
		}
		monitor.Run(ctx, 10*time.Minute)
	})
	if err = mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
