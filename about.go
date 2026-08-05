package main

import (
	_ "embed"
	"strings"
	"sync"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The license texts are compiled into Mikrotool so every distributed build
// carries its notices without relying on separately delivered files.

//go:embed LICENSE
var mikrotoolLicense string

//go:embed licenses/THIRD-PARTY-NOTICES.md
var thirdPartyNotices string

//go:embed licenses/Go-Standard-Library-BSD-3-Clause.txt
var goStandardLibraryLicense string

//go:embed licenses/GO-DEPENDENCY-LICENSES.txt
var goDependencyLicenses string

//go:embed licenses/wireguard-tools-GPL-2.0.txt
var wireguardToolsLicense string

//go:embed licenses/wireguard-go-MIT.txt
var wireguardGoLicense string

//go:embed licenses/Bash-GPL-3.0-or-later.txt
var bashLicense string

const aboutMikrotool = `Mikrotool v2.0
Made by Elan Fergusson
Mikrotool is a virtual address book for Mikrotik routers, allowing you to quickly access sites via WinBox or WireGuard without any lengthy setup.

Here goes the licenses:`

const mikrotoolLicenseNotice = `Mikrotool
Copyright (C) 2026 Elan Fergusson

Mikrotool is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, version 3 only.

Mikrotool comes with ABSOLUTELY NO WARRANTY. You may convey the software under the conditions in the GNU General Public License. The complete license follows.`

type dependencyLicenseSection struct {
	path    string
	title   string
	purpose string
	license string
}

var dependencyLicenseSections = []dependencyLicenseSection{
	{"al.essio.dev/pkg/shellescape", "Shellescape", "Escapes arguments used by the macOS credential-store helper.", "MIT"},
	{"fyne.io/fyne/v2", "Fyne", "Provides Mikrotool's cross-platform graphical interface and embedded fonts.", "BSD-3-Clause, SIL OFL 1.1, and Bitstream/Arev/DejaVu font terms"},
	{"fyne.io/systray", "Fyne Systray", "Provides desktop system-tray integration used by Fyne.", "Apache-2.0"},
	{"github.com/BurntSushi/toml", "BurntSushi TOML", "Reads application metadata used by Fyne packaging.", "MIT"},
	{"github.com/danieljoos/wincred", "Wincred", "Stores Mikrotool credentials in Windows Credential Manager.", "MIT"},
	{"github.com/fredbi/uri", "URI", "Handles local file and storage URIs in Fyne dialogs.", "MIT"},
	{"github.com/fsnotify/fsnotify", "FSNotify", "Provides file-system notifications used by Fyne.", "BSD-3-Clause"},
	{"github.com/fyne-io/image", "Fyne Image", "Loads application images and Windows icon resources.", "BSD-3-Clause and Apache-2.0"},
	{"github.com/fyne-io/oksvg", "Fyne OKSVG", "Renders SVG icons in the interface.", "BSD-3-Clause"},
	{"github.com/go-gl/gl", "Go OpenGL", "Provides OpenGL bindings used to draw the desktop interface.", "MIT"},
	{"github.com/go-gl/glfw/v3.3/glfw", "GLFW", "Creates and manages native desktop windows and input.", "BSD-3-Clause and zlib"},
	{"github.com/go-text/render", "Go Text Render", "Renders text in the Fyne interface.", "Unlicense or BSD-3-Clause"},
	{"github.com/go-text/typesetting", "Go Text Typesetting", "Shapes and lays out text in the Fyne interface.", "Unlicense or BSD-3-Clause, with MIT HarfBuzz components"},
	{"github.com/godbus/dbus/v5", "D-Bus", "Connects the credential store to Linux desktop secret services where supported.", "BSD-2-Clause"},
	{"github.com/jeandeaual/go-locale", "Go Locale", "Detects the operating system locale for the interface.", "MIT"},
	{"github.com/jsummers/gobmp", "Go BMP", "Decodes bitmap images used by the interface.", "MIT"},
	{"github.com/nfnt/resize", "Resize", "Resizes raster images used by Fyne.", "ISC"},
	{"github.com/nicksnyder/go-i18n/v2", "Go i18n", "Provides interface localization support used by Fyne.", "MIT"},
	{"github.com/srwiley/oksvg", "OKSVG", "Parses and renders SVG graphics used by Fyne.", "BSD-3-Clause"},
	{"github.com/srwiley/rasterx", "Rasterx", "Rasterizes vector graphics used by Fyne.", "BSD-3-Clause"},
	{"github.com/yuin/goldmark", "Goldmark", "Renders formatted text used by Fyne widgets.", "MIT"},
	{"github.com/zalando/go-keyring", "Go Keyring", "Stores the router password in the operating system credential manager.", "MIT"},
	{"golang.org/x/crypto", "Go Cryptography", "Provides SSH, secure key handling, and cryptographic primitives.", "BSD-3-Clause"},
	{"golang.org/x/image", "Go Image", "Provides image and font processing used by Fyne.", "BSD-3-Clause"},
	{"golang.org/x/net", "Go Networking", "Provides supplemental networking support used by dependencies.", "BSD-3-Clause"},
	{"golang.org/x/sys", "Go System Calls", "Provides native macOS and Windows system integration.", "BSD-3-Clause"},
	{"golang.org/x/text", "Go Text", "Provides Unicode and text processing used by the interface and networking code.", "BSD-3-Clause"},
	{"golang.zx2c4.com/wireguard/wgctrl", "WireGuard Control", "Generates WireGuard keys and represents WireGuard configuration safely.", "MIT"},
}

var (
	licensesAboutOnce        sync.Once
	licensesAboutWrappedOnce sync.Once
	licensesAbout            string
	licensesAboutWrapped     string
)

func licensesAboutText() string {
	licensesAboutOnce.Do(func() {
		licensesAbout = buildLicensesAboutText()
	})
	return licensesAbout
}

func buildLicensesAboutText() string {
	parts := []string{strings.TrimSpace(aboutMikrotool)}
	parts = append(parts, licenseSection(
		"Mikrotool",
		"Purpose: The Mikrotool application.",
		mikrotoolLicenseNotice+"\n\n"+mikrotoolLicense,
	))
	parts = append(parts, licenseSection(
		"Go Runtime and Standard Library",
		"Purpose: The Go runtime and standard-library code compiled into both applications.\nLicense: BSD-3-Clause with the Go patent grant.",
		goStandardLibraryLicense,
	))
	parts = append(parts, licenseSection(
		"Distribution Notices",
		"Purpose: Release inventory and source information for redistributed components.",
		thirdPartyNotices,
	))
	parts = append(parts, licenseSection(
		"WireGuard Tools",
		"Purpose: The wg and wg-quick helpers bundled in the macOS application.\nLicense: GPL-2.0-only.",
		wireguardToolsLicense,
	))
	parts = append(parts, licenseSection(
		"WireGuard Go",
		"Purpose: The userspace WireGuard implementation bundled in the macOS application.\nLicense: MIT.",
		wireguardGoLicense,
	))
	parts = append(parts, licenseSection(
		"GNU Bash",
		"Purpose: The shell runtime bundled in the macOS application for wg-quick.\nLicense: GPL-3.0-or-later.",
		bashLicense,
	))

	blocks := parseGoDependencyLicenseBlocks(goDependencyLicenses)
	for _, dependency := range dependencyLicenseSections {
		parts = append(parts, licenseSection(
			dependency.title,
			"Purpose: "+dependency.purpose+"\nLicense: "+dependency.license+".",
			blocks[dependency.path],
		))
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func wrappedLicensesAboutText() string {
	licensesAboutWrappedOnce.Do(func() {
		licensesAboutWrapped = wrapStaticText(licensesAboutText(), 92)
	})
	return licensesAboutWrapped
}

func licenseSection(title, description, text string) string {
	return licenseHeading(title) + "\n\n" + strings.TrimSpace(description) + "\n\n" + strings.TrimSpace(text)
}

func licenseHeading(title string) string {
	const width = 72
	if len(title)+2 >= width {
		return "-- " + title + " --"
	}
	remaining := width - len(title) - 2
	left := remaining / 2
	right := remaining - left
	return strings.Repeat("-", left) + " " + title + " " + strings.Repeat("-", right)
}

func parseGoDependencyLicenseBlocks(contents string) map[string]string {
	separator := strings.Repeat("=", 78)
	segments := strings.Split(contents, separator)
	blocks := make(map[string]string, len(segments)/2)
	for index := 1; index+1 < len(segments); index += 2 {
		header := strings.TrimSpace(segments[index])
		fields := strings.Fields(header)
		if len(fields) < 2 {
			continue
		}
		blocks[fields[0]] = header + "\n\n" + strings.TrimSpace(segments[index+1])
	}
	return blocks
}

func newCopyableTextLabel(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapBreak
	label.TextStyle = fyne.TextStyle{Monospace: true}
	label.Selectable = true
	return label
}

func wrapStaticText(text string, columns int) string {
	if columns < 1 {
		return text
	}
	lines := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped = append(wrapped, wrapStaticLine(line, columns)...)
	}
	return strings.Join(wrapped, "\n")
}

func wrapStaticLine(line string, columns int) []string {
	runes := []rune(line)
	if len(runes) <= columns {
		return []string{line}
	}

	indentEnd := 0
	for indentEnd < len(runes) && unicode.IsSpace(runes[indentEnd]) {
		indentEnd++
	}
	indent := append([]rune(nil), runes[:indentEnd]...)
	remaining := runes
	result := make([]string, 0, (len(runes)/columns)+1)
	for len(remaining) > columns {
		breakAt := columns
		for index := columns; index > 0; index-- {
			if unicode.IsSpace(remaining[index]) {
				breakAt = index
				break
			}
		}
		result = append(result, strings.TrimRightFunc(string(remaining[:breakAt]), unicode.IsSpace))
		remaining = remaining[breakAt:]
		for len(remaining) > 0 && unicode.IsSpace(remaining[0]) {
			remaining = remaining[1:]
		}
		if len(indent) > 0 && len(remaining) > 0 {
			remaining = append(append([]rune(nil), indent...), remaining...)
		}
	}
	result = append(result, string(remaining))
	return result
}

func (ui *mikrotoolUI) showAbout() {
	ui.recordAction("Opened Licenses / About.")
	ui.setSettingsLog(nil)

	text := licensesAboutText()
	back := widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), ui.showSettings)
	copyAll := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		ui.window.Clipboard().SetContent(text)
		ui.recordAction("Copied Licenses / About.")
	})
	copyAll.Importance = widget.LowImportance
	header := container.NewBorder(nil, nil, back, copyAll)
	if ui.aboutTextGrid == nil {
		ui.aboutTextGrid = widget.NewTextGridFromString(wrappedLicensesAboutText())
		ui.aboutTextGrid.Scroll = fyne.ScrollVerticalOnly
	}
	ui.window.SetContent(container.NewBorder(header, nil, nil, nil, ui.aboutTextGrid))
}
