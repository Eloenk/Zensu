package main

import (
	"context"
	"embed"
	"os"
	"os/signal"
	"syscall"
	"zensu/internal/config"
	"zensu/internal/logger"
	"zensu/internal/tray"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := logger.Init(); err != nil {
		println("Error initializing logger:", err.Error())
	}
	defer logger.Close()

	app := NewApp()

	// Handle signals for graceful shutdown on Ctrl+C / terminal close
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Infof("SIGNAL_RECEIVED", "Received termination signal, cleaning up...")
		tray.Quit()
		app.shutdown(context.Background())
		os.Exit(0)
	}()

	err := wails.Run(&options.App{
		Title:         "Zensu",
		Width:         850,
		Height:        550,
		DisableResize: true,
		Fullscreen:    false,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 20, G: 20, B: 30, A: 255},
		OnStartup:        app.startup,
		OnShutdown: func(ctx context.Context) {
			tray.Quit()
			app.shutdown(ctx)
		},
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			cfg, err := config.Load()
			if err == nil && cfg.MinimizeToTray {
				logger.Infof("APP_MINIMIZE_TRAY", "Hiding window to background tray...")
				wailsRuntime.WindowHide(ctx)
				return true
			}
			return false
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "zensu-anime-downloader-lock",
			OnSecondInstanceLaunch: func(secondInstanceData options.SecondInstanceData) {
				logger.Infof("SINGLE_INSTANCE", "Second instance launch detected, bringing main window to front...")
				if app.ctx != nil {
					wailsRuntime.WindowShow(app.ctx)
					wailsRuntime.WindowUnminimise(app.ctx)
				}
			},
		},
		Logger: &logger.WailsLogger{},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		logger.Errorf("APP_RUN", "Wails run failed: %v", err)
	}
}
