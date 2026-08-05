package main

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed Icon.png
var iconData []byte

var appIcon = fyne.NewStaticResource("Icon.png", iconData)
