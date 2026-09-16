package tray

import (
	"context"
	_ "embed"
	"os"
	"zensu/internal/logger"

	"github.com/energye/systray"
)

//go:embed icon.ico
var iconData []byte

type TrayManager struct {
	ctx           context.Context
	onShow        func()
	onQuit        func()
	onCheckUpdate func()
}

var globalManager *TrayManager

func Init(ctx context.Context, onShow func(), onCheckUpdate func(), onQuit func()) {
	globalManager = &TrayManager{
		ctx:           ctx,
		onShow:        onShow,
		onQuit:        onQuit,
		onCheckUpdate: onCheckUpdate,
	}

	go systray.Run(onReady, onExit)
}

func Quit() {
	systray.Quit()
}

func onReady() {
	if len(iconData) > 0 {
		systray.SetIcon(iconData)
	}
	systray.SetTitle("Zensu")
	systray.SetTooltip("Zensu Anime Downloader")

	mShow := systray.AddMenuItem("Show Zensu", "Bring Zensu window to front")
	systray.AddSeparator()
	mCheckUpdate := systray.AddMenuItem("Check for Updates", "Check for app updates")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit Zensu", "Exit application completely")

	systray.SetOnClick(func(menu systray.IMenu) {
		if globalManager != nil && globalManager.onShow != nil {
			globalManager.onShow()
		}
	})

	systray.SetOnDClick(func(menu systray.IMenu) {
		if globalManager != nil && globalManager.onShow != nil {
			globalManager.onShow()
		}
	})

	mShow.Click(func() {
		if globalManager != nil && globalManager.onShow != nil {
			globalManager.onShow()
		}
	})

	mCheckUpdate.Click(func() {
		if globalManager != nil && globalManager.onCheckUpdate != nil {
			globalManager.onCheckUpdate()
		}
	})

	mQuit.Click(func() {
		logger.Infof("TRAY_QUIT", "Quit clicked from tray menu")
		systray.Quit()
		if globalManager != nil && globalManager.onQuit != nil {
			globalManager.onQuit()
		} else {
			os.Exit(0)
		}
	})
}

func onExit() {
}
