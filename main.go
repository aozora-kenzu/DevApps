package main

import (
	"context"
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/aozora-kenzu/UniDevUI/internal/autostart"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIconICO []byte

//go:embed build/appicon.png
var trayIconPNG []byte

func main() {
	startHidden := autostart.HasTrayArg(os.Args[1:])
	app := NewApp(startHidden, trayIconICO, trayIconPNG)

	err := wails.Run(&options.App{
		Title:            "UniDevUI — Portable Runtimes & Windows Apps",
		Width:            1200,
		Height:           800,
		MinWidth:         1000,
		MinHeight:        640,
		Frameless:        true,
		StartHidden:      startHidden,
		CSSDragProperty:  "--wails-draggable",
		CSSDragValue:     "drag",
		BackgroundColour: &options.RGBA{R: 30, G: 32, B: 38, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Windows: &windows.Options{
			Theme: windows.Dark,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "UniDevUI-SingleInstance-AozoraKZ-2026",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				if !autostart.HasTrayArg(data.Args) {
					app.ShowWindow()
				}
			},
		},
		OnStartup:     app.startup,
		OnShutdown:    app.shutdown,
		OnBeforeClose: func(ctx context.Context) bool { return app.handleBeforeClose(ctx) },
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
