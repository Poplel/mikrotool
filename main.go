package main

import (
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"mikrotool/internal/deeplink"
	"mikrotool/internal/dock"
)

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	dataDir := filepath.Join(configDir, "Mikrotool")

	// A mikrotool: link opens a second process on Windows. Hand the link to the
	// copy that already owns the data directory and stop, so one instance keeps
	// managing the site list and any live tunnel. A plain second launch asks
	// that copy to reopen its window, which may be closed to the tray.
	launchLink := deeplink.LinkArgument(os.Args[1:])
	releaseInstance, primary := deeplink.AcquirePrimary(dataDir)
	if !primary {
		if launchLink != "" {
			if deliverErr := deeplink.Deliver(dataDir, launchLink); deliverErr == nil {
				return
			}
		} else if showErr := deeplink.RequestShow(dataDir); showErr == nil {
			return
		}
	}
	defer releaseInstance()

	application := app.NewWithID(appID)
	application.SetIcon(appIcon)
	window := application.NewWindow(fmt.Sprintf("%s v%s", appName, appVersion))

	migrationErrors := migrateLegacyData(configDir, dataDir)
	if err := migrateLegacyPreferences(application); err != nil {
		migrationErrors = append(migrationErrors, err)
	}
	ui := newMikrotoolUI(application, window, dataDir)
	for _, migrationErr := range migrationErrors {
		ui.appendLog("Migration warning: " + migrationErr.Error())
	}
	if registerErr := deeplink.Register(); registerErr != nil {
		ui.appendLog("Link handler registration warning: " + registerErr.Error())
	}
	window.SetContent(ui.mainPage())
	window.Resize(fyne.NewSize(820, 760))
	window.SetCloseIntercept(ui.hideWindow)
	window.Show()
	ui.installTray()
	// AppKit installs its own Apple Event handlers while launching, so take
	// over the reopen event only once the app has started.
	application.Lifecycle().SetOnStarted(func() { dock.OnReopen(ui.showWindow) })
	ui.recoverInterruptedSession()
	if primary {
		stopListening := deeplink.Listen(dataDir, ui.applyLinkAsync, ui.reportLinkError, func() { fyne.Do(ui.showWindow) })
		defer stopListening()
	}
	if launchLink != "" {
		ui.openLink(launchLink)
	}
	application.Run()
}
