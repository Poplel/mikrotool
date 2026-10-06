package main

import (
	_ "embed"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"

	"mikrotool/internal/model"
)

// TrayIcon.png is the app icon's silhouette in black for the macOS menu bar,
// where a template image is tinted to match light and dark menu bars.
//
//go:embed TrayIcon.png
var trayTemplateData []byte

// systrayMonitorTitle is the hidden window Fyne creates alongside the tray. It
// receives the close request when macOS asks the app to quit (Cmd-Q, logout).
const systrayMonitorTitle = "SystrayMonitor"

// trayHost is the part of desktop.App the tray needs, so tests can record the
// menu without a real menu bar.
type trayHost interface {
	SetSystemTrayMenu(menu *fyne.Menu)
	SetSystemTrayIcon(icon fyne.Resource)
}

// installTray puts Mikrotool in the macOS menu bar or the Windows notification
// area. From then on closing the window leaves Mikrotool running there, and
// quitting goes through the same cleanup as before, so a live tunnel's router
// peer is still removed.
func (ui *mikrotoolUI) installTray() {
	host, ok := ui.app.(desktop.App)
	if !ok {
		return
	}
	ui.tray = host
	host.SetSystemTrayIcon(trayIcon())
	ui.refreshTray()
	for _, window := range ui.app.Driver().AllWindows() {
		if window.Title() == systrayMonitorTitle {
			window.SetCloseIntercept(ui.requestClose)
		}
	}
}

func trayIcon() fyne.Resource {
	if runtime.GOOS == "darwin" {
		// Fyne passes a themed resource to macOS as a template image.
		return theme.NewThemedResource(fyne.NewStaticResource("TrayIcon.png", trayTemplateData))
	}
	return appIcon
}

// trayPlaceName is what each platform calls the place the tray icon lives.
func trayPlaceName() string {
	if runtime.GOOS == "darwin" {
		return "menu bar"
	}
	return "notification area"
}

// refreshTray rebuilds the tray menu from the current sites and WireGuard
// state. It runs on the Fyne goroutine and skips the rebuild when nothing a
// user can see has changed, so an open menu is not rebuilt underneath them.
func (ui *mikrotoolUI) refreshTray() {
	if ui.tray == nil {
		return
	}
	menu := ui.trayMenu()
	signature := traySignature(menu, ui.sites)
	if signature == ui.traySignature {
		return
	}
	ui.traySignature = signature
	ui.tray.SetSystemTrayMenu(menu)
}

// trayState is a consistent snapshot of the WireGuard session for one menu.
type trayState struct {
	active      *activeConnection
	busy        bool
	closing     bool
	cancellable bool
	connecting  model.Site
}

func (ui *mikrotoolUI) trayState() trayState {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	return trayState{
		active:      ui.active,
		busy:        ui.busy,
		closing:     ui.closing,
		cancellable: ui.connectCancel != nil,
		connecting:  ui.connectingSite,
	}
}

// trayMenu lists the session status, then every site as "<Site ID> - <Site
// Name>" in name order with its WireGuard and WinBox commands, then the window
// and quit commands. The menu shape follows the WireGuard app's own tray menu.
func (ui *mikrotoolUI) trayMenu() *fyne.Menu {
	state := ui.trayState()
	items := []*fyne.MenuItem{disabledItem(trayStatusText(state))}
	if state.active != nil && state.active.peer.Address != "" && !state.busy {
		items = append(items, disabledItem("Address: "+state.active.peer.Address))
	}
	switch {
	case state.closing:
	case state.active != nil && !state.busy:
		items = append(items, fyne.NewMenuItem("Disconnect WireGuard", ui.toggleWireGuard))
	case state.active == nil && state.busy && state.cancellable:
		items = append(items, fyne.NewMenuItem("Cancel Connection", ui.toggleWireGuard))
	}
	items = append(items, fyne.NewMenuItemSeparator())

	if len(ui.sites) == 0 {
		items = append(items, disabledItem("No saved sites"))
	}
	for _, row := range buildSiteListRows(ui.sites, sortBySiteName, true) {
		if row.siteIndex < 0 {
			continue
		}
		site := ui.sites[row.siteIndex]
		siteItem := fyne.NewMenuItem(trayLabel(site.SiteID+" - "+site.Name), nil)
		siteItem.ChildMenu = fyne.NewMenu("", ui.trayWireGuardItem(site, state), ui.trayWinBoxItem(site, state))
		items = append(items, siteItem)
	}

	quit := fyne.NewMenuItem("Quit Mikrotool", ui.requestClose)
	quit.IsQuit = true
	quit.Disabled = state.closing
	items = append(items,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Open Mikrotool", ui.showWindow),
		fyne.NewMenuItemSeparator(),
		quit,
	)
	return fyne.NewMenu("Mikrotool", items...)
}

func trayStatusText(state trayState) string {
	switch {
	case state.closing:
		return "Status: Quitting…"
	case state.active != nil && state.busy:
		return "Status: Disconnecting from " + traySiteName(state.active.site) + "…"
	case state.active != nil:
		return "Status: Connected to " + traySiteName(state.active.site)
	case state.busy && state.cancellable:
		return "Status: Connecting to " + traySiteName(state.connecting) + "…"
	case state.busy:
		return "Status: Cancelling…"
	default:
		return "Status: Not connected"
	}
}

func traySiteName(site model.Site) string {
	if site.SiteID == "" {
		return trayLabel(site.IP)
	}
	return trayLabel(site.SiteID + " - " + site.Name)
}

func (ui *mikrotoolUI) trayWireGuardItem(site model.Site, state trayState) *fyne.MenuItem {
	switch {
	case state.active != nil && sameTraySite(state.active.site, site):
		item := fyne.NewMenuItem("Disconnect WireGuard", func() { ui.trayWireGuard(site) })
		item.Disabled = state.busy || state.closing
		return item
	case state.active == nil && state.busy && sameTraySite(state.connecting, site):
		item := fyne.NewMenuItem("Cancel WireGuard Connection", func() { ui.trayWireGuard(site) })
		item.Disabled = !state.cancellable || state.closing
		return item
	default:
		item := fyne.NewMenuItem("Connect WireGuard", func() { ui.trayWireGuard(site) })
		item.Disabled = state.active != nil || state.busy || state.closing
		return item
	}
}

func (ui *mikrotoolUI) trayWinBoxItem(site model.Site, state trayState) *fyne.MenuItem {
	item := fyne.NewMenuItem("Open WinBox", func() { ui.openWinBoxFor(site.IP) })
	item.Disabled = state.closing
	return item
}

// trayWireGuard acts on the state at the moment of the click, which may have
// moved on since the menu was built: it disconnects or cancels this site's own
// session, connects when idle, and never touches another site's session.
func (ui *mikrotoolUI) trayWireGuard(site model.Site) {
	state := ui.trayState()
	switch {
	case state.closing:
	case state.active != nil:
		if sameTraySite(state.active.site, site) && !state.busy {
			ui.toggleWireGuard()
		}
	case state.busy:
		if sameTraySite(state.connecting, site) {
			ui.toggleWireGuard()
		}
	default:
		ui.connectWireGuard(site)
	}
}

// sameTraySite matches a session to a menu site by site ID, or by IP address
// for a session recovered after a crash, which records only its router.
func sameTraySite(session, site model.Site) bool {
	session, site = session.Normalized(), site.Normalized()
	if session.SiteID != "" && site.SiteID != "" {
		return model.SameSiteID(session.SiteID, site.SiteID)
	}
	return session.IP != "" && session.IP == site.IP
}

func disabledItem(label string) *fyne.MenuItem {
	item := fyne.NewMenuItem(label, nil)
	item.Disabled = true
	return item
}

// trayLabel keeps a site's text literal in a native menu. Windows reads a
// single & as a keyboard-accelerator marker, so "Smith & Co" would lose it.
func trayLabel(text string) string {
	if runtime.GOOS == "windows" {
		return strings.ReplaceAll(text, "&", "&&")
	}
	return text
}

// traySignature describes everything a menu shows or acts on. Site addresses
// are included because a menu item's action uses them without displaying them.
func traySignature(menu *fyne.Menu, sites []model.Site) string {
	var builder strings.Builder
	for _, site := range sites {
		builder.WriteString(site.SiteID + "\x00" + site.IP + "\n")
	}
	var write func(items []*fyne.MenuItem, depth int)
	write = func(items []*fyne.MenuItem, depth int) {
		for _, item := range items {
			builder.WriteString(strings.Repeat(">", depth))
			if item.IsSeparator {
				builder.WriteString("---\n")
				continue
			}
			builder.WriteString(item.Label)
			if item.Disabled {
				builder.WriteString(" [disabled]")
			}
			builder.WriteByte('\n')
			if item.ChildMenu != nil {
				write(item.ChildMenu.Items, depth+1)
			}
		}
	}
	write(menu.Items, 0)
	return builder.String()
}
