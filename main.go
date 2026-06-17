package main

import (
	"context"
	"embed"
	"os"
	"os/signal"
	"syscall"
	"zensu/internal/logger"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
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
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		logger.Errorf("APP_RUN", "Wails run failed: %v", err)
	}
}
