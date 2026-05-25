package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
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

	// 初始化代理服务
	if err := app.initProxy(); err != nil {
		log.Printf("failed to start proxy: %v", err)
	}

	err := a.Run()
	if err != nil {
		log.Fatal(err)
	}
}
