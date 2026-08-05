package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	fynestorage "fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mikrotool/internal/inputcheck"
	"mikrotool/internal/model"
	"mikrotool/internal/router"
	"mikrotool/internal/sshclient"
	"mikrotool/internal/store"
	"mikrotool/internal/transfer"
	"mikrotool/internal/tunnel"
	"mikrotool/internal/winbox"
)

const (
	prefUsername         = "ssh_username"
	prefSSHPort          = "ssh_port"
	prefWinBoxPath       = "winbox_path"
	prefMacAuthExplained = "mac_authorization_explained"
)

type activeConnection struct {
	peer       router.Peer
	tunnel     tunnel.Session
	connection router.Connection
}

type mikrotoolUI struct {
	app         fyne.App
	window      fyne.Window
	dataDir     string
	sitesStore  *store.SiteStore
	recovery    *store.RecoveryStore
	actionLog   *store.ActionLog
	credentials store.CredentialStore
	knownHosts  *sshclient.KnownHosts
	tunnels     *tunnel.Backend

	ipEntry      *widget.Entry
	companyEntry *widget.Entry
	nameEntry    *widget.Entry
	status       *widget.Label
	siteList     *widget.List
	wgButton     *widget.Button
	searchEntry  *widget.Entry
	sortButtons  map[siteSortField]*widget.Button

	sites         []model.Site
	siteRows      []siteListRow
	sortField     siteSortField
	sortAscending bool

	mu            sync.Mutex
	busy          bool
	active        *activeConnection
	closing       bool
	connectCancel context.CancelFunc

	settingsSaveMu sync.Mutex
	settingsLogMu  sync.Mutex
	settingsLog    *widget.Label
	aboutTextGrid  *widget.TextGrid

	logErrMu sync.Mutex
	logErr   error
}

func newMikrotoolUI(application fyne.App, window fyne.Window, dataDir string) *mikrotoolUI {
	ui := &mikrotoolUI{
		app:           application,
		window:        window,
		dataDir:       dataDir,
		sitesStore:    store.NewSiteStore(filepath.Join(dataDir, "sites.csv")),
		recovery:      store.NewRecoveryStore(filepath.Join(dataDir, "active-session.json")),
		actionLog:     store.NewActionLog(filepath.Join(dataDir, "actions.csv")),
		knownHosts:    sshclient.NewKnownHosts(filepath.Join(dataDir, "known_hosts")),
		tunnels:       tunnel.New(dataDir),
		ipEntry:       widget.NewEntry(),
		companyEntry:  widget.NewEntry(),
		nameEntry:     widget.NewEntry(),
		status:        widget.NewLabel("Ready."),
		sortField:     sortBySiteName,
		sortAscending: false,
	}
	ui.status.TextStyle = fyne.TextStyle{Bold: true}
	ui.knownHosts.SetTrustObserver(func(host, fingerprint string) {
		ui.recordAction(fmt.Sprintf("Automatically trusted new SSH host key for %s (%s).", host, fingerprint))
	})
	ui.loadSites()
	ui.createSiteList()
	ui.recordAction("Mikrotool v2.0 started.")
	return ui
}

func (ui *mikrotoolUI) mainPage() fyne.CanvasObject {
	settings := widget.NewButtonWithIcon("", theme.SettingsIcon(), ui.showSettings)
	settings.Importance = widget.LowImportance
	header := container.NewBorder(nil, nil, nil, settings)

	form := widget.NewForm(
		widget.NewFormItem("IP Address", ui.fieldWithCopy(ui.ipEntry)),
		widget.NewFormItem("Company Code", ui.fieldWithCopy(ui.companyEntry)),
		widget.NewFormItem("Site Name", ui.fieldWithCopy(ui.nameEntry)),
	)

	saveButton := widget.NewButtonWithIcon("Save", theme.DocumentSaveIcon(), ui.saveSite)
	ui.wgButton = widget.NewButtonWithIcon("Connect WireGuard", theme.MediaPlayIcon(), ui.toggleWireGuard)
	winboxButton := widget.NewButton("Open WinBox", ui.openWinBox)
	actions := container.NewGridWithColumns(3, saveButton, ui.wgButton, winboxButton)
	searchRow := container.NewGridWithColumns(3, widget.NewLabel(""), widget.NewLabel(""), ui.searchEntry)

	listHeader := ui.siteListHeader()
	listSection := container.NewBorder(container.NewVBox(widget.NewSeparator(), listHeader), nil, nil, nil, ui.siteList)

	top := container.NewVBox(header, form, ui.status, actions, searchRow)
	ui.mu.Lock()
	active := ui.active != nil
	busy := ui.busy
	cancellable := ui.connectCancel != nil
	ui.mu.Unlock()
	if active {
		ui.setWireGuardState("Disconnect WireGuard", !busy)
	} else if busy && cancellable {
		ui.setWireGuardState("Cancel Connection", true)
	} else if busy {
		ui.setWireGuardState("Cancelling…", false)
	}
	return container.NewBorder(top, nil, nil, nil, listSection)
}

func (ui *mikrotoolUI) fieldWithCopy(entry *widget.Entry) fyne.CanvasObject {
	copyButton := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		ui.window.Clipboard().SetContent(entry.Text)
		ui.setStatus("Copied to clipboard.")
	})
	copyButton.Importance = widget.LowImportance
	return container.NewBorder(nil, nil, nil, copyButton, entry)
}

func (ui *mikrotoolUI) loadSites() {
	sites, err := ui.sitesStore.Load()
	if err != nil {
		ui.sites = []model.Site{}
		ui.appendLog("Could not load sites: " + err.Error())
		return
	}
	ui.sites = sites
}

func (ui *mikrotoolUI) createSiteList() {
	ui.siteRows = buildSiteListRows(ui.sites, ui.sortField, ui.sortAscending)
	ui.searchEntry = widget.NewEntry()
	ui.searchEntry.SetPlaceHolder("Search")
	ui.searchEntry.OnChanged = ui.searchSites
	ui.siteList = widget.NewList(
		func() int { return len(ui.siteRows) },
		func() fyne.CanvasObject {
			return container.NewGridWithColumns(3, newEditableCell(), newEditableCell(), newEditableCell())
		},
		func(id widget.ListItemID, object fyne.CanvasObject) {
			if id < 0 || id >= len(ui.siteRows) {
				return
			}
			cells := object.(*fyne.Container)
			row := ui.siteRows[id]
			if row.divider != "" {
				cells.Objects[0].(*editableCell).SetDivider(row.divider)
				cells.Objects[1].(*editableCell).SetDivider("")
				cells.Objects[2].(*editableCell).SetDivider("")
				return
			}
			if row.siteIndex < 0 || row.siteIndex >= len(ui.sites) {
				return
			}
			site := ui.sites[row.siteIndex]
			values := []string{site.IP, site.CompanyCode, site.Name}
			for column := range 3 {
				cell := cells.Objects[column].(*editableCell)
				columnCopy := column
				cell.Set(values[column],
					func() { ui.siteList.Select(id) },
					func() { ui.editSiteValue(row.siteIndex, columnCopy) },
					func(event *fyne.PointEvent) { ui.showSiteMenu(row.siteIndex, cell, event) },
				)
			}
		},
	)
	ui.siteList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(ui.siteRows) {
			return
		}
		row := ui.siteRows[id]
		if row.divider != "" || row.siteIndex < 0 {
			ui.siteList.Unselect(id)
			return
		}
		ui.selectSite(row.siteIndex)
	}
}

func (ui *mikrotoolUI) siteListHeader() fyne.CanvasObject {
	ui.sortButtons = make(map[siteSortField]*widget.Button, 3)
	labels := map[siteSortField]string{
		sortByIP:          "IP Address",
		sortByCompanyCode: "Company Code",
		sortBySiteName:    "Site Name",
	}
	objects := make([]fyne.CanvasObject, 0, 3)
	for _, field := range []siteSortField{sortByIP, sortByCompanyCode, sortBySiteName} {
		fieldCopy := field
		button := widget.NewButton("", func() { ui.setSiteSort(fieldCopy) })
		button.Importance = widget.LowImportance
		ui.sortButtons[field] = button
		objects = append(objects, button)
	}
	ui.refreshSortButtonLabels(labels)
	return container.NewGridWithColumns(3, objects...)
}

func (ui *mikrotoolUI) refreshSortButtonLabels(labels map[siteSortField]string) {
	for field, button := range ui.sortButtons {
		label := labels[field]
		if field == ui.sortField {
			if ui.sortAscending {
				label += " ↑"
			} else {
				label += " ↓"
			}
		}
		button.SetText(label)
	}
}

func (ui *mikrotoolUI) setSiteSort(field siteSortField) {
	if ui.sortField == field {
		ui.sortAscending = !ui.sortAscending
	} else {
		ui.sortField = field
		ui.sortAscending = true
	}
	ui.refreshSortButtonLabels(map[siteSortField]string{
		sortByIP:          "IP Address",
		sortByCompanyCode: "Company Code",
		sortBySiteName:    "Site Name",
	})
	ui.refreshSiteList("")
}

func (ui *mikrotoolUI) refreshSiteList(selectCompanyCode string) {
	ui.siteRows = buildSiteListRows(ui.sites, ui.sortField, ui.sortAscending)
	ui.siteList.Refresh()
	ui.siteList.UnselectAll()
	if selectCompanyCode == "" {
		return
	}
	for rowID, row := range ui.siteRows {
		if row.siteIndex >= 0 && model.SameCompanyCode(ui.sites[row.siteIndex].CompanyCode, selectCompanyCode) {
			ui.siteList.Select(rowID)
			ui.siteList.ScrollTo(rowID)
			return
		}
	}
}

func (ui *mikrotoolUI) searchSites(query string) {
	siteIndex := closestSiteIndex(ui.sites, query)
	if siteIndex < 0 {
		ui.siteList.UnselectAll()
		return
	}
	for rowID, row := range ui.siteRows {
		if row.siteIndex == siteIndex {
			ui.siteList.Select(rowID)
			ui.siteList.ScrollTo(rowID)
			return
		}
	}
}

func (ui *mikrotoolUI) showSiteMenu(siteIndex int, cell fyne.CanvasObject, event *fyne.PointEvent) {
	if siteIndex < 0 || siteIndex >= len(ui.sites) {
		return
	}
	companyCode := ui.sites[siteIndex].CompanyCode
	menu := fyne.NewMenu("", fyne.NewMenuItem("Delete", func() {
		ui.confirmDeleteSite(companyCode)
	}))
	widget.ShowPopUpMenuAtRelativePosition(menu, ui.window.Canvas(), event.Position, cell)
}

func (ui *mikrotoolUI) confirmDeleteSite(companyCode string) {
	for _, site := range ui.sites {
		if !model.SameCompanyCode(site.CompanyCode, companyCode) {
			continue
		}
		dialog.ShowConfirm("Delete Site", "Delete "+site.Name+"?", func(confirmed bool) {
			if confirmed {
				ui.deleteSite(companyCode)
			}
		}, ui.window)
		return
	}
}

func (ui *mikrotoolUI) deleteSite(companyCode string) {
	next := make([]model.Site, 0, len(ui.sites))
	deletedName := ""
	for _, site := range ui.sites {
		if deletedName == "" && model.SameCompanyCode(site.CompanyCode, companyCode) {
			deletedName = site.Name
			continue
		}
		next = append(next, site)
	}
	if deletedName == "" {
		return
	}
	if err := ui.sitesStore.Save(next); err != nil {
		ui.recordAction("Site deletion failed: " + err.Error())
		dialog.ShowError(err, ui.window)
		return
	}
	ui.sites = next
	ui.refreshSiteList("")
	ui.setStatus("Deleted " + deletedName + ".")
	ui.recordAction("Deleted site " + deletedName + ".")
}

func (ui *mikrotoolUI) selectSite(id int) {
	if id < 0 || id >= len(ui.sites) {
		return
	}
	site := ui.sites[id]
	ui.ipEntry.SetText(site.IP)
	ui.companyEntry.SetText(site.CompanyCode)
	ui.nameEntry.SetText(site.Name)
}

func (ui *mikrotoolUI) saveSite() {
	site := model.Site{IP: ui.ipEntry.Text, CompanyCode: ui.companyEntry.Text, Name: ui.nameEntry.Text}
	next, updated, err := ui.sitesStore.Upsert(ui.sites, site)
	if err != nil {
		ui.recordAction("Site save failed: " + err.Error())
		dialog.ShowError(err, ui.window)
		return
	}
	ui.sites = next
	ui.refreshSiteList(site.Normalized().CompanyCode)
	if updated {
		ui.setStatus("Updated company " + site.Normalized().CompanyCode + ".")
	} else {
		ui.setStatus("Saved new company " + site.Normalized().CompanyCode + ".")
	}
}

func (ui *mikrotoolUI) editSiteValue(id, column int) {
	if id < 0 || id >= len(ui.sites) || column < 0 || column > 2 {
		return
	}
	site := ui.sites[id]
	labels := []string{"IP Address", "Company Code", "Site Name"}
	values := []string{site.IP, site.CompanyCode, site.Name}
	ui.recordAction("Opened " + labels[column] + " editor for company " + site.CompanyCode + ".")
	entry := widget.NewEntry()
	entry.SetText(values[column])
	dialog.ShowForm("Edit "+labels[column], "Save", "Cancel", []*widget.FormItem{
		widget.NewFormItem(labels[column], entry),
	}, func(ok bool) {
		if !ok {
			ui.recordAction("Site edit cancelled.")
			return
		}
		edited := site
		switch column {
		case 0:
			edited.IP = entry.Text
		case 1:
			edited.CompanyCode = entry.Text
		case 2:
			edited.Name = entry.Text
		}
		if err := edited.Validate(); err != nil {
			ui.recordAction("Site edit validation failed: " + err.Error())
			dialog.ShowError(err, ui.window)
			return
		}
		for other := range ui.sites {
			if other != id && model.SameCompanyCode(ui.sites[other].CompanyCode, edited.CompanyCode) {
				err := fmt.Errorf("company code %q already exists", edited.CompanyCode)
				ui.recordAction("Site edit failed: " + err.Error())
				dialog.ShowError(err, ui.window)
				return
			}
		}
		next := append([]model.Site(nil), ui.sites...)
		next[id] = edited.Normalized()
		if err := ui.sitesStore.Save(next); err != nil {
			ui.recordAction("Site edit could not be saved: " + err.Error())
			dialog.ShowError(err, ui.window)
			return
		}
		ui.sites = next
		ui.refreshSiteList(edited.Normalized().CompanyCode)
		ui.setStatus("Updated " + labels[column] + ".")
	}, ui.window)
}

func (ui *mikrotoolUI) showImport() {
	ui.recordAction("Opened the CSV/WBX import picker.")
	fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			ui.recordAction("Import picker failed: " + err.Error())
			dialog.ShowError(err, ui.window)
			return
		}
		if reader == nil {
			ui.recordAction("Import cancelled.")
			return
		}
		fileName, validationErr := inputcheck.DisplayText(reader.URI().Name(), "import file name", 255)
		if validationErr != nil {
			_ = reader.Close()
			ui.recordAction("Import rejected: " + validationErr.Error())
			dialog.ShowError(validationErr, ui.window)
			return
		}
		extension := strings.ToLower(reader.URI().Extension())
		existing := append([]model.Site(nil), ui.sites...)
		ui.setStatus("Importing " + fileName + "…")
		go func() {
			defer reader.Close()
			var parsed transfer.Parsed
			var parseErr error
			switch extension {
			case ".csv":
				parsed, parseErr = transfer.ParseCSV(reader)
			case ".wbx":
				parsed, parseErr = transfer.ParseWBX(reader)
			default:
				parseErr = fmt.Errorf("choose a .csv or WinBox 3 .wbx file")
			}
			if parseErr != nil {
				fyne.Do(func() {
					ui.setStatus("Import failed.")
					ui.appendLog("Import failed: " + parseErr.Error())
					dialog.ShowError(parseErr, ui.window)
				})
				return
			}
			merged := transfer.Merge(existing, parsed)
			if err := ui.sitesStore.Save(merged.Sites); err != nil {
				fyne.Do(func() {
					ui.setStatus("Import failed while saving.")
					ui.appendLog("Imported sites could not be saved: " + err.Error())
					dialog.ShowError(err, ui.window)
				})
				return
			}
			fyne.Do(func() {
				ui.sites = merged.Sites
				ui.refreshSiteList("")
				summary := fmt.Sprintf("Created %d, updated %d, skipped %d.", merged.Created, merged.Updated, merged.Skipped)
				if extension == ".wbx" {
					summary += " WinBox usernames and passwords were ignored."
				}
				ui.setStatus("Import complete. " + summary)
				ui.appendLog("Import complete: " + summary)
				for index, warning := range merged.Warnings {
					if index == 5 {
						ui.appendLog(fmt.Sprintf("%d additional import warnings omitted.", len(merged.Warnings)-index))
						break
					}
					ui.appendLog("Import warning: " + warning)
				}
				dialog.ShowInformation("Import Complete", summary, ui.window)
			})
		}()
	}, ui.window)
	fileDialog.SetFilter(fynestorage.NewExtensionFileFilter([]string{".csv", ".wbx"}))
	fileDialog.Show()
}

func (ui *mikrotoolUI) showExport() {
	sites := append([]model.Site(nil), ui.sites...)
	ui.recordAction("Opened the CSV export picker.")
	fileDialog := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			ui.recordAction("Export picker failed: " + err.Error())
			dialog.ShowError(err, ui.window)
			return
		}
		if writer == nil {
			ui.recordAction("Export cancelled.")
			return
		}
		fileName, validationErr := inputcheck.DisplayText(writer.URI().Name(), "export file name", 255)
		if validationErr == nil && strings.ToLower(writer.URI().Extension()) != ".csv" {
			validationErr = fmt.Errorf("export file must use the .csv extension")
		}
		if validationErr != nil {
			_ = writer.Close()
			ui.recordAction("Export rejected: " + validationErr.Error())
			dialog.ShowError(validationErr, ui.window)
			return
		}
		ui.setStatus("Exporting sites…")
		go func() {
			exportErr := transfer.ExportCSV(writer, sites)
			exportErr = errors.Join(exportErr, writer.Close())
			fyne.Do(func() {
				if exportErr != nil {
					ui.setStatus("CSV export failed.")
					ui.appendLog("CSV export failed: " + exportErr.Error())
					dialog.ShowError(exportErr, ui.window)
					return
				}
				message := fmt.Sprintf("Exported %d site(s) to %s.", len(sites), fileName)
				ui.setStatus(message)
				ui.appendLog(message)
			})
		}()
	}, ui.window)
	fileDialog.SetFilter(fynestorage.NewExtensionFileFilter([]string{".csv"}))
	fileDialog.SetFileName("mikrotool-sites.csv")
	fileDialog.Show()
}

type settingsSnapshot struct {
	username   string
	password   string
	port       string
	winboxPath string
}

func (ui *mikrotoolUI) saveSettings(values settingsSnapshot) error {
	ui.settingsSaveMu.Lock()
	defer ui.settingsSaveMu.Unlock()

	username, err := inputcheck.Username(values.username)
	if err != nil {
		return err
	}
	if err := inputcheck.Password(values.password); err != nil {
		return err
	}
	portNumber, err := strconv.ParseUint(strings.TrimSpace(values.port), 10, 16)
	if err != nil || portNumber == 0 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if runtime.GOOS == "windows" {
		if err := winbox.ValidateExecutablePath(values.winboxPath); err != nil {
			return err
		}
	}
	if err := ui.credentials.SetPassword(values.password); err != nil {
		return err
	}
	preferences := ui.app.Preferences()
	preferences.SetString(prefUsername, username)
	preferences.SetString(prefSSHPort, strconv.FormatUint(portNumber, 10))
	if runtime.GOOS == "windows" {
		preferences.SetString(prefWinBoxPath, strings.TrimSpace(values.winboxPath))
	}
	return nil
}

func (ui *mikrotoolUI) showSettings() {
	ui.recordAction("Opened Settings.")
	username := widget.NewEntry()
	username.SetText(ui.app.Preferences().String(prefUsername))
	password := widget.NewPasswordEntry()
	if saved, err := ui.credentials.Password(); err == nil {
		password.SetText(saved)
	} else if !errors.Is(err, store.ErrPasswordNotSet) {
		ui.recordAction("Stored password could not be loaded: " + err.Error())
	}
	port := widget.NewEntry()
	port.SetText(ui.app.Preferences().StringWithFallback(prefSSHPort, "22"))
	winboxPath := widget.NewEntry()
	winboxPath.SetText(ui.app.Preferences().String(prefWinBoxPath))

	items := []*widget.FormItem{
		widget.NewFormItem("Username", username),
		widget.NewFormItem("Password", password),
		widget.NewFormItem("Port", port),
	}
	if runtime.GOOS == "windows" {
		browseWinBox := widget.NewButtonWithIcon("Browse…", theme.FolderOpenIcon(), func() {
			ui.recordAction("Opened the WinBox executable picker.")
			fileDialog := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
				if err != nil {
					ui.recordAction("WinBox executable picker failed: " + err.Error())
					dialog.ShowError(err, ui.window)
					return
				}
				if reader == nil {
					ui.recordAction("WinBox executable selection cancelled.")
					return
				}
				selectedURI := reader.URI()
				_ = reader.Close()
				if !strings.EqualFold(selectedURI.Scheme(), "file") {
					err := fmt.Errorf("WinBox executable must be selected from a local drive")
					ui.recordAction("WinBox executable selection rejected: " + err.Error())
					dialog.ShowError(err, ui.window)
					return
				}
				selectedPath := selectedURI.Path()
				if err := winbox.ValidateExecutablePath(selectedPath); err != nil {
					ui.recordAction("WinBox executable selection rejected: " + err.Error())
					dialog.ShowError(err, ui.window)
					return
				}
				winboxPath.SetText(selectedPath)
				ui.recordAction("Selected a WinBox executable.")
			}, ui.window)
			fileDialog.SetFilter(fynestorage.NewExtensionFileFilter([]string{".exe"}))
			fileDialog.Show()
		})
		pathWithPicker := container.NewBorder(nil, nil, nil, browseWinBox, winboxPath)
		items = append(items, widget.NewFormItem("WinBox executable", pathWithPicker))
	}
	form := widget.NewForm(items...)
	autoSaveStatus := widget.NewLabel("Changes save automatically.")
	autoSaveStatus.Wrapping = fyne.TextWrapWord

	snapshot := func() settingsSnapshot {
		return settingsSnapshot{
			username: username.Text, password: password.Text,
			port: port.Text, winboxPath: winboxPath.Text,
		}
	}
	var timerMu sync.Mutex
	var timer *time.Timer
	var generation uint64
	cancelPendingSave := func() {
		timerMu.Lock()
		generation++
		if timer != nil {
			timer.Stop()
			timer = nil
		}
		timerMu.Unlock()
	}
	scheduleSave := func() {
		values := snapshot()
		timerMu.Lock()
		generation++
		currentGeneration := generation
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(500*time.Millisecond, func() {
			timerMu.Lock()
			if currentGeneration != generation {
				timerMu.Unlock()
				return
			}
			timerMu.Unlock()
			err := ui.saveSettings(values)
			fyne.Do(func() {
				if err != nil {
					autoSaveStatus.SetText("Not saved: " + err.Error())
					return
				}
				autoSaveStatus.SetText("Saved automatically.")
				ui.recordAction("Settings saved automatically.")
			})
		})
		timerMu.Unlock()
	}
	username.OnChanged = func(string) { scheduleSave() }
	password.OnChanged = func(string) { scheduleSave() }
	port.OnChanged = func(string) { scheduleSave() }
	winboxPath.OnChanged = func(string) { scheduleSave() }

	leaveSettings := func(next func()) {
		cancelPendingSave()
		if err := ui.saveSettings(snapshot()); err != nil {
			ui.recordAction("Settings contain unsaved values: " + err.Error())
		} else {
			ui.recordAction("Settings saved automatically.")
		}
		ui.setSettingsLog(nil)
		next()
	}
	back := widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), func() {
		leaveSettings(func() {
			ui.recordAction("Closed Settings.")
			ui.window.SetContent(ui.mainPage())
		})
	})
	info := widget.NewButtonWithIcon("", theme.InfoIcon(), func() {
		leaveSettings(ui.showAbout)
	})
	info.Importance = widget.LowImportance
	header := container.NewBorder(nil, nil, back, info,
		widget.NewLabelWithStyle("Settings", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))

	importButton := widget.NewButtonWithIcon("Import CSV / WinBox 3 WBX…", theme.UploadIcon(), ui.showImport)
	exportButton := widget.NewButtonWithIcon("Export CSV…", theme.DownloadIcon(), ui.showExport)
	dataActions := container.NewGridWithColumns(2, importButton, exportButton)

	actionLabel := newCopyableTextLabel("")
	ui.setSettingsLog(actionLabel)
	actionLabel.SetText(ui.settingsActionLogText())
	logHeader := widget.NewLabelWithStyle("Log", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	logScroll := container.NewVScroll(actionLabel)
	logScroll.SetMinSize(fyne.NewSize(100, 240))
	settingsBody := container.NewBorder(
		container.NewVBox(form, autoSaveStatus, dataActions, widget.NewSeparator(), logHeader),
		nil, nil, nil, logScroll,
	)
	ui.window.SetContent(container.NewBorder(header, nil, nil, nil, settingsBody))
}

func (ui *mikrotoolUI) connectionSettings(host string) (router.Connection, error) {
	if net.ParseIP(strings.TrimSpace(host)) == nil {
		return router.Connection{}, fmt.Errorf("select or enter a valid router IP address")
	}
	username, err := inputcheck.Username(ui.app.Preferences().String(prefUsername))
	if err != nil {
		return router.Connection{}, fmt.Errorf("open Settings and enter a valid SSH username: %w", err)
	}
	portNumber, err := strconv.ParseUint(ui.app.Preferences().StringWithFallback(prefSSHPort, "22"), 10, 16)
	if err != nil || portNumber == 0 {
		return router.Connection{}, fmt.Errorf("set a valid SSH port in Settings")
	}
	password, err := ui.credentials.Password()
	if err != nil {
		return router.Connection{}, fmt.Errorf("open Settings and save the SSH password: %w", err)
	}
	return router.Connection{
		Host: strings.TrimSpace(host), Port: uint16(portNumber), Username: username,
		Password: password, KnownHosts: ui.knownHosts,
	}, nil
}

func (ui *mikrotoolUI) openWinBox() {
	connection, err := ui.connectionSettings(ui.ipEntry.Text)
	if err != nil {
		ui.recordAction("WinBox could not be opened: " + err.Error())
		dialog.ShowError(err, ui.window)
		return
	}
	launcher := winbox.Launcher{Executable: ui.app.Preferences().String(prefWinBoxPath)}
	ui.setStatus("Opening WinBox…")
	go func() {
		err := launcher.Open(connection.Host, connection.Username, connection.Password)
		fyne.Do(func() {
			if err != nil {
				ui.setStatus("WinBox could not be opened.")
				ui.appendLog("WinBox launch failed: " + err.Error())
				dialog.ShowError(err, ui.window)
				return
			}
			ui.setStatus("WinBox opened for " + connection.Host + ".")
		})
	}()
}

func (ui *mikrotoolUI) toggleWireGuard() {
	ui.mu.Lock()
	if ui.busy {
		cancel := ui.connectCancel
		if ui.active == nil && cancel != nil {
			ui.connectCancel = nil
			ui.mu.Unlock()
			cancel()
			ui.setWireGuardState("Cancelling…", false)
			ui.setStatus("Cancelling the WireGuard connection…")
			return
		}
		ui.mu.Unlock()
		return
	}
	active := ui.active
	if active != nil {
		ui.busy = true
	}
	ui.mu.Unlock()
	if active != nil {
		ui.setWireGuardState("Disconnecting…", false)
		go ui.disconnect(active, true, true, "Disconnecting WireGuard…")
		return
	}
	site := model.Site{IP: ui.ipEntry.Text, CompanyCode: ui.companyEntry.Text, Name: ui.nameEntry.Text}.Normalized()
	if runtime.GOOS == "darwin" && !ui.app.Preferences().Bool(prefMacAuthExplained) {
		ui.recordAction("Displayed the macOS authorization explanation.")
		message := "Mikrotool uses macOS's built-in osascript utility to request temporary administrator rights for the network changes required by WireGuard. macOS—not Mikrotool—collects your password. Normal use should require one authorization while connecting and one while disconnecting."
		dialog.ShowConfirm("macOS Authorization", message+"\n\nContinue?", func(ok bool) {
			if !ok {
				ui.setStatus("WireGuard connection cancelled.")
				return
			}
			ui.app.Preferences().SetBool(prefMacAuthExplained, true)
			ui.startWireGuard(site)
		}, ui.window)
		return
	}
	ui.startWireGuard(site)
}

func (ui *mikrotoolUI) startWireGuard(site model.Site) {
	ctx, cancel := context.WithCancel(context.Background())
	ui.mu.Lock()
	if ui.busy || ui.active != nil {
		ui.mu.Unlock()
		cancel()
		return
	}
	ui.busy = true
	ui.connectCancel = cancel
	ui.mu.Unlock()
	ui.setWireGuardState("Cancel Connection", true)
	go ui.beginWireGuard(ctx, site)
}

func (ui *mikrotoolUI) beginWireGuard(parent context.Context, site model.Site) {
	if err := site.Validate(); err != nil {
		ui.failOperation(err)
		return
	}
	connection, err := ui.connectionSettings(site.IP)
	if err != nil {
		ui.failOperation(err)
		return
	}
	ui.setStatusAsync("Connecting by SSH and reading router information…")
	ctx, cancel := context.WithTimeout(parent, 35*time.Second)
	defer cancel()
	client, err := router.Connect(ctx, connection)
	if err != nil {
		ui.handleSSHError(err)
		return
	}
	info, err := client.Inspect(ctx)
	_ = client.Close()
	if err != nil {
		ui.failOperation(err)
		return
	}
	if err := parent.Err(); err != nil {
		ui.failOperation(err)
		return
	}
	ui.appendLogAsync(fmt.Sprintf("Connected to %s (RouterOS %s); found %d WireGuard interface(s).", info.Identity, info.Version, len(info.Interfaces)))
	if len(info.Interfaces) == 1 {
		ui.provision(parent, site, connection, info, info.Interfaces[0])
		return
	}
	fyne.Do(func() {
		selectWidget := widget.NewSelect(info.Interfaces, nil)
		selectWidget.SetSelected(info.Interfaces[0])
		dialog.ShowCustomConfirm("Choose WireGuard Interface", "Connect", "Cancel", container.NewVBox(
			widget.NewLabel("Choose the router interface for this transient connection."), selectWidget,
		), func(ok bool) {
			if !ok {
				ui.cancelConnection()
				ui.completeOperation(nil)
				return
			}
			go ui.provision(parent, site, connection, info, selectWidget.Selected)
		}, ui.window)
	})
}

func (ui *mikrotoolUI) provision(parent context.Context, site model.Site, connection router.Connection, info router.RouterInfo, interfaceName string) {
	ui.setStatusAsync("Finding an unused address and creating a random transient peer…")
	ctx, cancel := context.WithTimeout(parent, 40*time.Second)
	defer cancel()
	client, err := router.Connect(ctx, connection)
	if err != nil {
		ui.handleSSHError(err)
		return
	}
	peer, err := client.CreateTransientPeer(ctx, info, interfaceName)
	_ = client.Close()
	if err != nil {
		var cleanupErr error
		if peer.Name != "" && peer.PublicKey != "" {
			peer.PrivateKey = ""
			cleanupErr = ui.removeRemotePeer(connection, peer.Name, peer.PublicKey)
		}
		ui.failOperation(errors.Join(err, cleanupErr))
		return
	}
	if err := parent.Err(); err != nil {
		peer.PrivateKey = ""
		cleanupErr := ui.removeRemotePeer(connection, peer.Name, peer.PublicKey)
		ui.failOperation(errors.Join(err, cleanupErr))
		return
	}
	tunnelName := localTunnelName(peer.Name)
	recovery := store.Recovery{
		Host: peer.Host, SSHPort: peer.SSHPort, Username: peer.Username,
		PeerName: peer.Name, PublicKey: peer.PublicKey, TunnelName: tunnelName,
	}
	if err := ui.recovery.Save(recovery); err != nil {
		cleanupErr := ui.removeRemotePeer(connection, peer.Name, peer.PublicKey)
		ui.failOperation(errors.Join(fmt.Errorf("save crash-recovery state: %w", err), cleanupErr))
		return
	}
	ui.setStatusAsync("Peer created at " + peer.Address + "; starting the local WireGuard tunnel…")
	tunnelSession, err := ui.tunnels.Start(parent, tunnelName, peer.Config(), peer.PublicKey)
	peer.PrivateKey = ""
	if err != nil {
		cleanupErr := ui.removeRemotePeer(connection, peer.Name, peer.PublicKey)
		if cleanupErr == nil {
			_ = ui.recovery.Clear()
		}
		ui.failOperation(errors.Join(err, cleanupErr))
		return
	}
	recovery.ConfigPath = tunnelSession.ConfigPath
	recovery.Interface = tunnelSession.Interface
	if err := ui.recovery.Save(recovery); err != nil {
		stopErr := ui.tunnels.Stop(tunnelSession)
		cleanupErr := ui.removeRemotePeer(connection, peer.Name, peer.PublicKey)
		ui.failOperation(errors.Join(fmt.Errorf("update crash-recovery state: %w", err), stopErr, cleanupErr))
		return
	}
	active := &activeConnection{peer: peer, tunnel: tunnelSession, connection: connection}
	ui.mu.Lock()
	cancelled := ui.connectCancel == nil || parent.Err() != nil
	if cancelled {
		ui.mu.Unlock()
		stopErr := ui.tunnels.Stop(tunnelSession)
		cleanupErr := ui.removeRemotePeer(connection, peer.Name, peer.PublicKey)
		if stopErr == nil && cleanupErr == nil {
			_ = ui.recovery.Clear()
		}
		ui.failOperation(errors.Join(context.Canceled, stopErr, cleanupErr))
		return
	}
	ui.active = active
	ui.busy = false
	setupCancel := ui.connectCancel
	ui.connectCancel = nil
	ui.mu.Unlock()
	if setupCancel != nil {
		setupCancel()
	}
	fyne.Do(func() {
		ui.setWireGuardState("Disconnect WireGuard", true)
		ui.setStatus(fmt.Sprintf("WireGuard connected to %s as %s.", site.Name, peer.Address))
		ui.appendLog(fmt.Sprintf("Transient peer %s is active on %s. Press Disconnect WireGuard to remove it.", peer.Name, peer.Interface))
	})
	go ui.monitor(active)
}

func (ui *mikrotoolUI) disconnect(active *activeConnection, stopLocal, showPopup bool, statusText string) {
	ui.setStatusAsync(statusText)
	var tunnelErr error
	if stopLocal {
		if runtime.GOOS == "darwin" && active.tunnel.ConfigPath != "" {
			if _, err := os.Stat(active.tunnel.ConfigPath); errors.Is(err, os.ErrNotExist) {
				ui.appendLogAsync("The prior private WireGuard config was already removed; continuing router peer cleanup.")
				tunnelErr = ui.tunnels.RemovePrivateConfig(active.tunnel)
			} else {
				tunnelErr = ui.tunnels.Stop(active.tunnel)
			}
		} else {
			tunnelErr = ui.tunnels.Stop(active.tunnel)
		}
	} else {
		tunnelErr = ui.tunnels.RemovePrivateConfig(active.tunnel)
	}
	ui.setStatusAsync("Removing the transient peer from the router…")
	remoteErr := ui.removeRemotePeer(active.connection, active.peer.Name, active.peer.PublicKey)
	err := errors.Join(tunnelErr, remoteErr)
	if err == nil {
		_ = ui.recovery.Clear()
	}
	ui.mu.Lock()
	if ui.active == active {
		ui.active = nil
	}
	ui.busy = false
	closing := ui.closing
	ui.mu.Unlock()
	fyne.Do(func() {
		ui.setWireGuardState("Connect WireGuard", true)
		if err != nil {
			ui.setStatus("WireGuard stopped with cleanup warnings.")
			ui.appendLog("Cleanup warning: " + err.Error())
			if showPopup {
				dialog.ShowError(err, ui.window)
			}
		} else {
			ui.setStatus("WireGuard disconnected; transient peer removed.")
			ui.appendLog("Local tunnel stopped and transient router peer removed.")
		}
		if closing {
			ui.app.Quit()
		}
	})
}

func (ui *mikrotoolUI) removeRemotePeer(connection router.Connection, peerName, publicKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := router.Connect(ctx, connection)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.RemoveTransientPeer(ctx, peerName, publicKey)
}

func (ui *mikrotoolUI) monitor(active *activeConnection) {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ui.mu.Lock()
		if ui.active != active || ui.busy {
			ui.mu.Unlock()
			return
		}
		ui.mu.Unlock()
		running, err := ui.tunnels.Active(active.tunnel)
		if err != nil {
			ui.appendLogAsync("Could not verify local tunnel state: " + err.Error())
			continue
		}
		if running {
			continue
		}
		ui.mu.Lock()
		if ui.active != active || ui.busy {
			ui.mu.Unlock()
			return
		}
		ui.busy = true
		ui.mu.Unlock()
		ui.appendLogAsync("The local WireGuard session stopped outside Mikrotool; cleaning up its router peer.")
		ui.disconnect(active, false, false, "Local tunnel stopped; removing the transient router peer…")
		return
	}
}

func (ui *mikrotoolUI) recoverInterruptedSession() {
	value, err := ui.recovery.Load()
	if err != nil {
		ui.appendLog("Could not read interrupted-session state: " + err.Error())
		return
	}
	if value == nil {
		return
	}
	password, err := ui.credentials.Password()
	if err != nil {
		ui.appendLog("A prior transient peer may remain. Save the SSH password in Settings, then restart Mikrotool to clean it up.")
		return
	}
	connection := router.Connection{
		Host: value.Host, Port: value.SSHPort, Username: value.Username,
		Password: password, KnownHosts: ui.knownHosts,
	}
	active := &activeConnection{
		peer:       router.Peer{Host: value.Host, SSHPort: value.SSHPort, Username: value.Username, Name: value.PeerName, PublicKey: value.PublicKey},
		tunnel:     tunnel.Session{Name: value.TunnelName, Interface: value.Interface, ConfigPath: value.ConfigPath, PublicKey: value.PublicKey},
		connection: connection,
	}
	ui.mu.Lock()
	ui.active = active
	ui.busy = true
	ui.mu.Unlock()
	ui.setWireGuardState("Recovering…", false)
	go ui.disconnect(active, true, false, "Recovering and removing an interrupted transient session…")
}

func (ui *mikrotoolUI) handleSSHError(err error) {
	var changed *sshclient.ChangedHostKeyError
	if !errors.As(err, &changed) {
		ui.failOperation(err)
		return
	}
	fyne.Do(func() {
		ui.completeOperation(changed)
		expected := "the previously trusted key"
		if len(changed.Expected) > 0 {
			expected = strings.Join(changed.Expected, "\n")
		}
		message := fmt.Sprintf("The SSH host key for %s does not match.\n\nExpected:\n%s\n\nReceived:\n%s\n\nThe connection was blocked. Verify the router before changing its saved key.", changed.Host, expected, changed.Fingerprint)
		dialog.ShowError(errors.New(message), ui.window)
	})
}

func (ui *mikrotoolUI) requestClose() {
	ui.mu.Lock()
	if ui.closing {
		ui.mu.Unlock()
		return
	}
	ui.closing = true
	active := ui.active
	busy := ui.busy
	cancel := ui.connectCancel
	if busy && active == nil && cancel != nil {
		ui.connectCancel = nil
	}
	if active != nil && !busy {
		ui.busy = true
	}
	ui.mu.Unlock()
	if busy && active == nil && cancel != nil {
		cancel()
		ui.setWireGuardState("Cancelling…", false)
		ui.setStatus("Cancelling the connection before closing…")
		return
	}
	if busy {
		ui.recordAction("Close requested while Mikrotool was finishing an operation.")
		return
	}
	if active == nil {
		ui.recordAction("Mikrotool closed.")
		ui.app.Quit()
		return
	}
	ui.setWireGuardState("Disconnecting…", false)
	go ui.disconnect(active, true, false, "Stopping WireGuard before closing…")
}

func (ui *mikrotoolUI) failOperation(err error) {
	fyne.Do(func() {
		ui.completeOperation(err)
		if !errors.Is(err, context.Canceled) {
			dialog.ShowError(err, ui.window)
		}
	})
}

func (ui *mikrotoolUI) completeOperation(err error) {
	ui.mu.Lock()
	ui.busy = false
	cancel := ui.connectCancel
	ui.connectCancel = nil
	closing := ui.closing
	ui.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	ui.setWireGuardState("Connect WireGuard", true)
	if errors.Is(err, context.Canceled) {
		ui.setStatus("WireGuard connection cancelled.")
		if err != context.Canceled {
			ui.appendLog("Connection cancellation cleanup: " + err.Error())
		}
	} else if err != nil {
		ui.setStatus("WireGuard connection failed.")
		ui.appendLog("Connection failed: " + err.Error())
	} else {
		ui.setStatus("WireGuard connection cancelled.")
	}
	if closing {
		ui.recordAction("Mikrotool closed after cancelling setup.")
		ui.app.Quit()
	}
}

func (ui *mikrotoolUI) cancelConnection() {
	ui.mu.Lock()
	cancel := ui.connectCancel
	ui.connectCancel = nil
	ui.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (ui *mikrotoolUI) setWireGuardState(text string, enabled bool) {
	if ui.wgButton == nil {
		return
	}
	ui.wgButton.SetText(text)
	if strings.Contains(text, "Disconnect") || strings.Contains(text, "Cancel") || strings.Contains(text, "Stopping") {
		ui.wgButton.SetIcon(theme.MediaStopIcon())
	} else {
		ui.wgButton.SetIcon(theme.MediaPlayIcon())
	}
	if enabled {
		ui.wgButton.Enable()
	} else {
		ui.wgButton.Disable()
	}
}

func (ui *mikrotoolUI) setStatus(text string) {
	if ui.status.Text == text {
		return
	}
	ui.status.SetText(text)
	ui.recordAction(text)
}
func (ui *mikrotoolUI) setStatusAsync(text string) { fyne.Do(func() { ui.setStatus(text) }) }

func (ui *mikrotoolUI) appendLog(text string) {
	ui.recordAction(text)
}

func (ui *mikrotoolUI) appendLogAsync(text string) { fyne.Do(func() { ui.appendLog(text) }) }

func (ui *mikrotoolUI) recordAction(text string) {
	if err := ui.actionLog.Append(text); err != nil {
		ui.logErrMu.Lock()
		ui.logErr = err
		ui.logErrMu.Unlock()
	}
	ui.refreshSettingsLog()
}

func (ui *mikrotoolUI) setSettingsLog(label *widget.Label) {
	ui.settingsLogMu.Lock()
	ui.settingsLog = label
	ui.settingsLogMu.Unlock()
}

func (ui *mikrotoolUI) refreshSettingsLog() {
	ui.settingsLogMu.Lock()
	label := ui.settingsLog
	ui.settingsLogMu.Unlock()
	if label == nil {
		return
	}
	text := ui.settingsActionLogText()
	fyne.Do(func() {
		ui.settingsLogMu.Lock()
		defer ui.settingsLogMu.Unlock()
		if ui.settingsLog == label {
			label.SetText(text)
		}
	})
}

func (ui *mikrotoolUI) settingsActionLogText() string {
	entries, err := ui.actionLog.Entries()
	ui.logErrMu.Lock()
	deferredErr := ui.logErr
	ui.logErrMu.Unlock()
	if err != nil {
		return "The action log could not be read: " + err.Error()
	}
	lines := make([]string, 0, len(entries)+1)
	if deferredErr != nil {
		lines = append(lines, "Logging warning: "+deferredErr.Error())
	}
	for _, entry := range entries {
		lines = append(lines, entry.Time.In(time.Local).Format("2006-01-02 15:04:05")+"  "+entry.Message)
	}
	if len(lines) == 0 {
		return "No actions have been logged in the last 24 hours."
	}
	return strings.Join(lines, "\n")
}

func localTunnelName(peerName string) string {
	const prefix = "mt-"
	parts := strings.Split(peerName, "-")
	suffix := parts[len(parts)-1]
	if len(suffix) > 12 {
		suffix = suffix[len(suffix)-12:]
	}
	return prefix + suffix
}
