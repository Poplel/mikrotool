package main

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"mikrotool/internal/model"
	"mikrotool/internal/router"
)

type recordingTray struct {
	menus []*fyne.Menu
}

func (r *recordingTray) SetSystemTrayMenu(menu *fyne.Menu) { r.menus = append(r.menus, menu) }
func (r *recordingTray) SetSystemTrayIcon(fyne.Resource)   {}

func newTrayTestUI(t *testing.T, sites ...model.Site) (*mikrotoolUI, *recordingTray) {
	t.Helper()
	application := test.NewApp()
	t.Cleanup(application.Quit)
	window := application.NewWindow("Mikrotool tray test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	window.SetContent(ui.mainPage())
	if err := ui.sitesStore.Save(sites); err != nil {
		t.Fatal(err)
	}
	ui.sites = sites
	recorder := &recordingTray{}
	ui.tray = recorder
	ui.refreshSiteList("")
	return ui, recorder
}

func trayItem(t *testing.T, menu *fyne.Menu, label string) *fyne.MenuItem {
	t.Helper()
	for _, item := range menu.Items {
		if item.Label == label {
			return item
		}
	}
	t.Fatalf("tray menu has no %q item:\n%s", label, traySignature(menu, nil))
	return nil
}

func TestTrayListsSitesByNameWithTheirCommands(t *testing.T) {
	_, recorder := newTrayTestUI(t,
		model.Site{IP: "192.0.2.30", SiteID: "300", Name: "Zulu Yard"},
		model.Site{IP: "192.0.2.10", SiteID: "100", Name: "alpha Office"},
		model.Site{IP: "192.0.2.20", SiteID: "200", Name: "Bravo Depot"},
	)
	if len(recorder.menus) == 0 {
		t.Fatal("the tray menu was never set")
	}
	menu := recorder.menus[len(recorder.menus)-1]
	var siteLabels []string
	for _, item := range menu.Items {
		if item.ChildMenu != nil {
			siteLabels = append(siteLabels, item.Label)
		}
	}
	want := []string{"100 - alpha Office", "200 - Bravo Depot", "300 - Zulu Yard"}
	if strings.Join(siteLabels, "|") != strings.Join(want, "|") {
		t.Fatalf("site items = %q, want %q", siteLabels, want)
	}
	children := trayItem(t, menu, "200 - Bravo Depot").ChildMenu.Items
	if len(children) != 2 || children[0].Label != "Connect WireGuard" || children[1].Label != "Open WinBox" {
		t.Fatalf("unexpected site commands:\n%s", traySignature(fyne.NewMenu("", children...), nil))
	}
	if children[0].Disabled || children[1].Disabled {
		t.Fatal("site commands are disabled while Mikrotool is idle")
	}
	if status := menu.Items[0]; status.Label != "Status: Not connected" || !status.Disabled {
		t.Fatalf("unexpected status item %q disabled=%v", status.Label, status.Disabled)
	}
	last := menu.Items[len(menu.Items)-1]
	if !last.IsQuit || last.Label != "Quit Mikrotool" || last.Action == nil {
		t.Fatalf("the tray menu does not end with Mikrotool's own Quit item: %+v", last)
	}
}

func TestTrayShowsTheActiveSessionAndProtectsIt(t *testing.T) {
	alpha := model.Site{IP: "192.0.2.10", SiteID: "100", Name: "Alpha"}
	bravo := model.Site{IP: "192.0.2.20", SiteID: "200", Name: "Bravo"}
	ui, recorder := newTrayTestUI(t, alpha, bravo)
	active := &activeConnection{site: alpha, peer: router.Peer{Address: "10.9.0.7/32"}}
	ui.mu.Lock()
	ui.active = active
	ui.mu.Unlock()
	ui.setWireGuardState("Disconnect WireGuard", true)

	menu := recorder.menus[len(recorder.menus)-1]
	if menu.Items[0].Label != "Status: Connected to 100 - Alpha" {
		t.Fatalf("unexpected status %q", menu.Items[0].Label)
	}
	trayItem(t, menu, "Address: 10.9.0.7/32")
	trayItem(t, menu, "Disconnect WireGuard")
	if item := trayItem(t, menu, "100 - Alpha").ChildMenu.Items[0]; item.Label != "Disconnect WireGuard" || item.Disabled {
		t.Fatalf("the connected site offers %q disabled=%v", item.Label, item.Disabled)
	}
	if item := trayItem(t, menu, "200 - Bravo").ChildMenu.Items[0]; item.Label != "Connect WireGuard" || !item.Disabled {
		t.Fatalf("another site offers %q disabled=%v during a live session", item.Label, item.Disabled)
	}

	ui.trayWireGuard(bravo)
	ui.mu.Lock()
	unchanged := ui.active == active && !ui.busy
	ui.mu.Unlock()
	if !unchanged {
		t.Fatal("a tray click on another site changed the live session")
	}
}

func TestTrayRecoveredSessionMatchesItsSiteByAddress(t *testing.T) {
	if !sameTraySite(model.Site{IP: "192.0.2.10"}, model.Site{IP: "192.0.2.10", SiteID: "100", Name: "Alpha"}) {
		t.Fatal("a recovered session did not match its site by IP address")
	}
	if sameTraySite(model.Site{IP: "192.0.2.10", SiteID: "100"}, model.Site{IP: "192.0.2.10", SiteID: "200"}) {
		t.Fatal("two site IDs sharing one router were treated as the same site")
	}
}

func TestTrayRebuildsOnlyWhenSomethingChanges(t *testing.T) {
	ui, recorder := newTrayTestUI(t, model.Site{IP: "192.0.2.10", SiteID: "100", Name: "Alpha"})
	before := len(recorder.menus)
	ui.refreshTray()
	ui.setSiteSort(sortByIP)
	if len(recorder.menus) != before {
		t.Fatalf("the tray was rebuilt %d time(s) with nothing changed", len(recorder.menus)-before)
	}

	// An address change keeps the label but must reach the WinBox action.
	ui.sites[0].IP = "192.0.2.11"
	ui.refreshSiteList("")
	if len(recorder.menus) != before+1 {
		t.Fatal("the tray kept a stale site address")
	}
}

func TestClosingTheWindowKeepsMikrotoolRunning(t *testing.T) {
	ui, _ := newTrayTestUI(t)
	ui.hideWindow()
	ui.mu.Lock()
	closing := ui.closing
	ui.mu.Unlock()
	if closing {
		t.Fatal("closing the window started quitting Mikrotool")
	}
	if text := ui.settingsActionLogText(); !strings.Contains(text, "still running in the "+trayPlaceName()) {
		t.Fatalf("closing to the tray was not logged: %q", text)
	}
}
