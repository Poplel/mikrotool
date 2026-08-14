package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"

	"mikrotool/internal/deeplink"
	"mikrotool/internal/model"
)

func TestMainPageConstruction(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	if page := ui.mainPage(); page == nil {
		t.Fatal("main page is nil")
	}
}

func TestWireGuardButtonCancelsPendingConnection(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool cancellation test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	window.SetContent(ui.mainPage())
	ctx, cancel := context.WithCancel(context.Background())
	ui.mu.Lock()
	ui.busy = true
	ui.connectCancel = cancel
	ui.mu.Unlock()
	ui.toggleWireGuard()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("second WireGuard button press did not cancel setup")
	}
	if ui.wgButton.Text != "Cancelling…" || !ui.wgButton.Disabled() {
		t.Fatalf("unexpected cancellation button state text=%q disabled=%v", ui.wgButton.Text, ui.wgButton.Disabled())
	}
}

func TestSettingsPageRecordsAndDisplaysActionLog(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool settings test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	ui.showSettings()
	if window.Content() == nil {
		t.Fatal("settings page is nil")
	}
	text := ui.settingsActionLogText()
	if !strings.Contains(text, "Opened Settings") {
		t.Fatalf("settings action log was not displayed: %q", text)
	}
}

func TestLicensesAboutPageIsEmbeddedAndLogged(t *testing.T) {
	checks := map[string]string{
		"about":                    aboutMikrotool,
		"Mikrotool license notice": mikrotoolLicenseNotice,
		"Mikrotool GPLv3":          mikrotoolLicense,
		"notices":                  thirdPartyNotices,
		"Go dependencies":          goDependencyLicenses,
		"Go standard library":      goStandardLibraryLicense,
		"wireguard-tools":          wireguardToolsLicense,
		"wireguard-go":             wireguardGoLicense,
		"Bash":                     bashLicense,
	}
	for name, contents := range checks {
		if strings.TrimSpace(contents) == "" {
			t.Fatalf("%s license/about text was not embedded", name)
		}
	}
	if !strings.Contains(aboutMikrotool, "Made by Elan Fergusson") ||
		!strings.Contains(aboutMikrotool, "Mikrotool v2.1") {
		t.Fatal("about text is missing the author or display version")
	}

	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool about test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	ui.showAbout()
	if window.Content() == nil {
		t.Fatal("licenses/about page is nil")
	}
	if text := ui.settingsActionLogText(); !strings.Contains(text, "Opened Licenses / About") {
		t.Fatalf("opening licenses/about was not logged: %q", text)
	}
	allText := licensesAboutText()
	if !strings.HasPrefix(allText, `Mikrotool v2.1
Made by Elan Fergusson
Mikrotool is a virtual address book for Mikrotik routers, allowing you to quickly access sites via WinBox or WireGuard without any lengthy setup.

Here goes the licenses:`) {
		t.Fatal("single-page About text does not begin with the requested product text")
	}
	for _, expected := range []string{
		"Here goes the licenses:",
		"Copyright (C) 2026 Elan Fergusson",
		"version 3 only",
		"ABSOLUTELY NO WARRANTY",
		"GNU GENERAL PUBLIC LICENSE",
		"MIKROTOOL GO DEPENDENCY LICENSES",
		"theme/font/LICENSE.txt",
		"theme/font/LICENSE_DejaVu-Powerline.txt",
		"theme/font/LICENSE_Inter.txt",
		"ico/LICENSE-reader",
		"ico/LICENSE-writer",
		"glfw/LICENSE.md",
		"harfbuzz/LICENSE",
		"Go Runtime and Standard Library",
		"Additional IP Rights Grant (Patents)",
		"Purpose: Provides Mikrotool's cross-platform graphical interface",
		"Purpose: Stores the router password in the operating system credential manager",
	} {
		if !strings.Contains(allText, expected) {
			t.Fatalf("single-page licenses text is missing %q", expected)
		}
	}
	blocks := parseGoDependencyLicenseBlocks(goDependencyLicenses)
	if len(blocks) != len(dependencyLicenseSections) {
		t.Fatalf("embedded Go license inventory has %d blocks, expected %d", len(blocks), len(dependencyLicenseSections))
	}
	for _, dependency := range dependencyLicenseSections {
		if strings.TrimSpace(blocks[dependency.path]) == "" {
			t.Errorf("missing embedded license block for %s", dependency.path)
		}
	}
}

func TestCopyableTextLabelsWrapAndAllowSelection(t *testing.T) {
	label := newCopyableTextLabel("copy me")
	if !label.Selectable {
		t.Fatal("copyable text label is not selectable")
	}
	if label.Wrapping != fyne.TextWrapBreak {
		t.Fatalf("copyable text label wrapping = %v, want TextWrapBreak", label.Wrapping)
	}
}

func TestAboutUsesReusableStaticVirtualizedGrid(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool static About test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	ui.showAbout()
	first := ui.aboutTextGrid
	if first == nil {
		t.Fatal("About text grid was not created")
	}
	if first.Scroll != fyne.ScrollVerticalOnly {
		t.Fatalf("About grid scroll direction = %v, want vertical only", first.Scroll)
	}
	ui.showAbout()
	if ui.aboutTextGrid != first {
		t.Fatal("About grid was rebuilt instead of reused")
	}
	for lineNumber, line := range strings.Split(wrappedLicensesAboutText(), "\n") {
		if length := len([]rune(line)); length > 92 {
			t.Fatalf("wrapped About line %d has %d columns", lineNumber+1, length)
		}
	}
}

func TestDeleteSitePersistsOnlyTheRequestedRemoval(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool deletion test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	ui.sites = []model.Site{
		{IP: "10.0.0.1", SiteID: "ONE", Name: "One"},
		{IP: "10.0.0.2", SiteID: "TWO", Name: "Two"},
	}
	if err := ui.sitesStore.Save(ui.sites); err != nil {
		t.Fatal(err)
	}
	ui.refreshSiteList("")
	ui.deleteSite("one")
	loaded, err := ui.sitesStore.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].SiteID != "TWO" {
		t.Fatalf("unexpected sites after delete: %#v", loaded)
	}
}

func TestMainPageSearchSelectsAndLoadsClosestSite(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool search test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	ui.sites = []model.Site{
		{IP: "10.0.0.1", SiteID: "NORTH", Name: "North Warehouse"},
		{IP: "10.0.0.2", SiteID: "SOUTH", Name: "South Office"},
	}
	ui.refreshSiteList("")
	window.SetContent(ui.mainPage())
	ui.searchEntry.SetText("soth office")
	if ui.siteIDEntry.Text != "SOUTH" || ui.ipEntry.Text != "10.0.0.2" {
		t.Fatalf("search loaded siteID=%q IP=%q", ui.siteIDEntry.Text, ui.ipEntry.Text)
	}
	if ui.sortField != sortBySiteName || ui.sortAscending {
		t.Fatalf("unexpected default sort field=%v ascending=%v", ui.sortField, ui.sortAscending)
	}
	if ui.sortButtons[sortBySiteName].Text != "Site Name ↓" {
		t.Fatalf("unexpected default site sort button text %q", ui.sortButtons[sortBySiteName].Text)
	}
}

func TestAddressBookRowTemplateRendersSiteText(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool row rendering test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	ui.sites = []model.Site{{IP: "10.1.2.3", SiteID: "ACME", Name: "Acme Office"}}
	ui.refreshSiteList("")

	rowID := -1
	for index, row := range ui.siteRows {
		if row.siteIndex == 0 {
			rowID = index
			break
		}
	}
	if rowID < 0 {
		t.Fatal("site row was not created")
	}
	object := ui.siteList.CreateItem()
	ui.siteList.UpdateItem(rowID, object)
	cells := object.(*fyne.Container)
	got := []string{
		cells.Objects[0].(*editableCell).text,
		cells.Objects[1].(*editableCell).text,
		cells.Objects[2].(*editableCell).text,
	}
	want := []string{"10.1.2.3", "ACME", "Acme Office"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("cell %d text = %q, want %q", index, got[index], want[index])
		}
		if cells.Objects[index].MinSize().Height < fyne.MeasureText("M", theme.Size(theme.SizeNameText), fyne.TextStyle{}).Height {
			t.Fatalf("cell %d is too short to render text", index)
		}
	}
	markup := test.RenderObjectToMarkup(object)
	for _, expected := range want {
		if !strings.Contains(markup, expected) {
			t.Fatalf("rendered row markup is missing %q", expected)
		}
	}
}

func TestLinkAddsThenUpdatesASiteAndLoadsItIntoTheFields(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool link test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	window.SetContent(ui.mainPage())

	ui.openLinkForTest(t, "mikrotool://site?ip=203.0.113.10&id=ACME&name=Acme+HQ")
	if ui.ipEntry.Text != "203.0.113.10" || ui.siteIDEntry.Text != "ACME" || ui.nameEntry.Text != "Acme HQ" {
		t.Fatalf("link loaded ip=%q id=%q name=%q", ui.ipEntry.Text, ui.siteIDEntry.Text, ui.nameEntry.Text)
	}
	saved, err := ui.sitesStore.Load()
	if err != nil || len(saved) != 1 || saved[0].SiteID != "ACME" {
		t.Fatalf("link did not save one site: sites=%+v err=%v", saved, err)
	}

	ui.openLinkForTest(t, "mikrotool://site?ip=203.0.113.11&id=acme&name=Acme+Head+Office")
	saved, err = ui.sitesStore.Load()
	if err != nil || len(saved) != 1 {
		t.Fatalf("link created a duplicate site: sites=%+v err=%v", saved, err)
	}
	if saved[0].IP != "203.0.113.11" || saved[0].Name != "Acme Head Office" {
		t.Fatalf("link did not update the matching site ID: %+v", saved[0])
	}
	if text := ui.settingsActionLogText(); !strings.Contains(text, "Link updated site acme") {
		t.Fatalf("the link update was not logged: %q", text)
	}
}

func TestRejectedLinkIsLoggedAndChangesNothing(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool bad link test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	window.SetContent(ui.mainPage())

	ui.openLink("mikrotool://site?ip=203.0.113.10&id=ACME&name=HQ&winbox&wireguard")
	if saved, err := ui.sitesStore.Load(); err != nil || len(saved) != 0 {
		t.Fatalf("a rejected link changed the site list: sites=%+v err=%v", saved, err)
	}
	if text := ui.settingsActionLogText(); !strings.Contains(text, "Link rejected") {
		t.Fatalf("a rejected link was not logged: %q", text)
	}
}

func TestLinkNeverDisconnectsAnActiveWireGuardSession(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool active link test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	window.SetContent(ui.mainPage())

	active := &activeConnection{}
	ui.mu.Lock()
	ui.active = active
	ui.mu.Unlock()
	ui.connectWireGuardFromLink()
	ui.mu.Lock()
	unchanged := ui.active == active && !ui.busy
	ui.mu.Unlock()
	if !unchanged {
		t.Fatal("a link started a disconnect on the live session")
	}
	if !strings.Contains(ui.status.Text, "already connected") {
		t.Fatalf("unexpected status for a link during a live session: %q", ui.status.Text)
	}
}

func TestCopiedSiteLinkParsesBackToTheSameSite(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	window := application.NewWindow("Mikrotool copy link test")
	ui := newMikrotoolUI(application, window, t.TempDir())
	window.SetContent(ui.mainPage())

	site := model.Site{IP: "203.0.113.12", SiteID: "NORTH", Name: "North Warehouse"}
	ui.copySiteLink(site)
	copied := application.Clipboard().Content()
	link, err := deeplink.Parse(copied)
	if err != nil {
		t.Fatalf("copied link %q was rejected: %v", copied, err)
	}
	if link.Site != site || link.Action != deeplink.ActionNone {
		t.Fatalf("copied link produced %+v", link)
	}
}

// openLinkForTest applies a link on the calling goroutine. The application loop
// is not running in tests, so the queued work fyne.Do performs is done here.
func (ui *mikrotoolUI) openLinkForTest(t *testing.T, raw string) {
	t.Helper()
	link, err := deeplink.Parse(raw)
	if err != nil {
		t.Fatalf("link %q was rejected: %v", raw, err)
	}
	ui.applyLink(link)
}

func TestLocalTunnelName(t *testing.T) {
	got := localTunnelName("mikrotool-user-010203040506")
	if got != "mt-010203040506" {
		t.Fatalf("unexpected tunnel name %q", got)
	}
	if len(got) > 15 {
		t.Fatalf("macOS WireGuard tunnel name is too long: %q", got)
	}
}
