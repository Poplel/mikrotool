package main

import (
	"fmt"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	application := app.NewWithID(appID)
	application.SetIcon(appIcon)
	window := application.NewWindow(fmt.Sprintf("%s v%s", appName, appVersion))

	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	dataDir := filepath.Join(configDir, "Mikrotool")
	migrationErrors := migrateLegacyData(configDir, dataDir)
	if err := migrateLegacyPreferences(application); err != nil {
		migrationErrors = append(migrationErrors, err)
	}
	ui := newMikrotoolUI(application, window, dataDir)
	for _, migrationErr := range migrationErrors {
		ui.appendLog("Migration warning: " + migrationErr.Error())
	}
	window.SetContent(ui.mainPage())
	window.Resize(fyne.NewSize(820, 760))
	window.SetCloseIntercept(ui.requestClose)
	window.Show()
	ui.recoverInterruptedSession()
	application.Run()
}
