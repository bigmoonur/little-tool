package main

import (
	"embed"
	"os"
	"path/filepath"

	"videopicker/internal/picker"

	"github.com/bigmoonur/little-tool/shared/license"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

const appID = "VideoRandomPicker"

// dataDir 返回数据目录。程序旁有 data 文件夹时使用便携模式。
func dataDir() (string, error) {
	if dir := os.Getenv("VIDEO_PICKER_DATA"); dir != "" {
		return dir, nil
	}
	if exe, err := os.Executable(); err == nil {
		portable := filepath.Join(filepath.Dir(exe), "data")
		if fi, err := os.Stat(portable); err == nil && fi.IsDir() {
			return portable, nil
		}
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appID), nil
}

func main() {
	var store *picker.Store
	dir, err := dataDir()
	if err == nil {
		store, err = picker.OpenStore(dir)
	}
	var lic licenseManager
	if dir != "" {
		lic = license.New(dir, AppVersion)
	}
	app := NewApp(store, lic, err)

	width, height, maximised := 1120, 740, false
	settings := picker.Settings{Theme: "system"}
	if store != nil {
		settings = store.Settings()
		if w := store.Window(); w != nil {
			if w.Width >= 880 && w.Height >= 580 && w.Width <= 4000 && w.Height <= 3000 {
				width, height = w.Width, w.Height
			}
			maximised = w.Maximised
		}
	}
	// 开发时使用单独的单实例标识，不会和正在运行的正式版互相干扰。
	instanceID := "5f2d8c1a-0b7e-4c69-9d3a-video-random-picker"
	if os.Getenv("VIDEO_PICKER_DATA") != "" {
		instanceID += "-dev"
	}
	startState := options.Normal
	if maximised {
		startState = options.Maximised
	}
	bg := &options.RGBA{R: 236, G: 238, B: 242, A: 255}
	theme := windows.SystemDefault
	switch settings.Theme {
	case "dark":
		bg = &options.RGBA{R: 20, G: 24, B: 38, A: 255}
		theme = windows.Dark
	case "light":
		theme = windows.Light
	}

	err = wails.Run(&options.App{
		Title:            "视频随机提取工具",
		Width:            width,
		Height:           height,
		MinWidth:         880,
		MinHeight:        580,
		WindowStartState: startState,
		AlwaysOnTop:      settings.AlwaysOnTop,
		// 仅供开发时在浏览器里调试界面，不弹出原生窗口。
		StartHidden:      os.Getenv("VIDEO_PICKER_HIDDEN") == "1",
		BackgroundColour: bg,
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               instanceID,
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.onSecondInstance() },
		},
		Bind: []interface{}{app},
		Windows: &windows.Options{
			Theme:                theme,
			DisablePinchZoom:     true,
			IsZoomControlEnabled: false,
		},
		Mac: &mac.Options{
			About: &mac.AboutInfo{Title: "视频随机提取工具", Message: "版本 " + AppVersion},
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
