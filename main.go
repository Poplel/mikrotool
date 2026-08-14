package main

import (
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"

	"mikrotool/internal/deeplink"
)

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	dataDir := filepath.Join(configDir, "Mikrotool")

	// A mikrotool: link opens a second process on Windows. Hand the link to the
	// copy that already owns the data directory and stop, so one instance keeps
	// managing the site list and any live tunnel.
	launchLink := deeplink.LinkArgument(os.Args[1:])
	releaseInstance, primary := deeplink.AcquirePrimary(dataDir)
	if !primary && launchLink != "" {
		if deliverErr := deeplink.Deliver(dataDir, launchLink); deliverErr == nil {
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
	window.SetCloseIntercept(ui.requestClose)
	window.Show()
	ui.recoverInterruptedSession()
	if primary {
		stopListening := deeplink.Listen(dataDir, ui.applyLinkAsync, ui.reportLinkError)
		defer stopListening()
	}
	if launchLink != "" {
		ui.openLink(launchLink)
	}
	application.Run()
}
