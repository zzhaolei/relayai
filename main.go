package main

import (
	"embed"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	app := NewApp()

	a := application.New(application.Options{
		Name:        "RelayAI",
		Description: "AI 模型反代管理工具",
		Icon:        appIcon,
		Services: []application.Service{
			application.NewService(app),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
	})

	win := a.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "RelayAI",
		Width:     900,
		Height:    640,
		MinWidth:  800,
		MinHeight: 600,
		BackgroundColour: application.NewRGB(245, 247, 250),
		URL:       "/",
	})

	app.setWindow(win)

	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		win.Hide()
		e.Cancel()
	})

	a.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(event *application.ApplicationEvent) {
		if !win.IsVisible() {
			win.Show()
		}
		win.Focus()
	})

	tray := a.SystemTray.New()
	tray.SetIcon(appIcon)
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(appIcon)
	}
	tray.SetTooltip("RelayAI")

	var statusItem, toggleItem, tokenItem *application.MenuItem

	rebuildMenu := func() {
		menu := a.NewMenu()
		statusItem = menu.Add("代理: --")
		menu.AddSeparator()

		running := app.ProxyStatus().Running
		toggleItem = menu.AddCheckbox("启用代理", running)
		toggleItem.OnClick(func(ctx *application.Context) {
			if app.ProxyStatus().Running {
				app.proxy.Stop()
			} else {
				app.proxy.Start()
			}
		})
		menu.AddSeparator()

		tokenItem = menu.Add("Token: --")
		menu.AddSeparator()

		menu.Add("打开").OnClick(func(ctx *application.Context) {
			if win.IsVisible() {
				win.Focus()
			} else {
				win.Show()
			}
		})
		menu.Add("退出").OnClick(func(ctx *application.Context) {
			a.Quit()
		})
		tray.SetMenu(menu)
	}
	rebuildMenu()

	tray.OnClick(func() {
		tray.OpenMenu()
	})

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			status := app.ProxyStatus()
			if status.Running {
				statusItem.SetLabel("代理: 运行中 :" + status.Addr)
			} else {
				statusItem.SetLabel("代理: 已停止")
			}
			toggleItem.SetChecked(status.Running)
			in, out, total := app.proxy.GetTotalTokenUsage()
			tokenItem.SetLabel(fmt.Sprintf("入 %s  出 %s  共 %s",
				formatCount(in), formatCount(out), formatCount(total)))
		}
	}()

	if err := app.initProxy(); err != nil {
		log.Printf("failed to start proxy: %v", err)
	}

	err := a.Run()
	if err != nil {
		log.Fatal(err)
	}
}

func formatCount(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1_000_000_000)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
