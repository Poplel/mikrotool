package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type editableCell struct {
	widget.BaseWidget
	text        string
	bold        bool
	onTap       func()
	onDouble    func()
	onSecondary func(*fyne.PointEvent)
}

func newEditableCell() *editableCell {
	cell := &editableCell{}
	cell.ExtendBaseWidget(cell)
	return cell
}

func (c *editableCell) Set(text string, onTap, onDouble func(), onSecondary func(*fyne.PointEvent)) {
	c.text = text
	c.bold = false
	c.onTap = onTap
	c.onDouble = onDouble
	c.onSecondary = onSecondary
	c.Refresh()
}

func (c *editableCell) SetDivider(text string) {
	c.text = text
	c.bold = true
	c.onTap = nil
	c.onDouble = nil
	c.onSecondary = nil
	c.Refresh()
}

func (c *editableCell) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *editableCell) DoubleTapped(*fyne.PointEvent) {
	if c.onDouble != nil {
		c.onDouble()
	}
}

func (c *editableCell) TappedSecondary(event *fyne.PointEvent) {
	if c.onSecondary != nil {
		c.onSecondary(event)
	}
}

func (c *editableCell) CreateRenderer() fyne.WidgetRenderer {
	text := canvas.NewText(c.text, theme.Color(theme.ColorNameForeground))
	text.TextSize = theme.Size(theme.SizeNameText)
	text.TextStyle = fyne.TextStyle{Bold: c.bold}
	return &editableCellRenderer{cell: c, text: text, objects: []fyne.CanvasObject{text}}
}

type editableCellRenderer struct {
	cell    *editableCell
	text    *canvas.Text
	objects []fyne.CanvasObject
}

func (r *editableCellRenderer) Layout(size fyne.Size) {
	padding := theme.Size(theme.SizeNamePadding)
	lineHeight := fyne.MeasureText("M", theme.Size(theme.SizeNameText), fyne.TextStyle{}).Height
	r.text.Move(fyne.NewPos(padding, (size.Height-lineHeight)/2))
	r.text.Resize(fyne.NewSize(size.Width-2*padding, lineHeight))
}

func (r *editableCellRenderer) MinSize() fyne.Size {
	padding := theme.Size(theme.SizeNamePadding)
	line := fyne.MeasureText("M", theme.Size(theme.SizeNameText), fyne.TextStyle{})
	return fyne.NewSize(2*padding, line.Height+2*padding)
}

func (r *editableCellRenderer) Refresh() {
	r.text.Text = r.cell.text
	r.text.Color = theme.Color(theme.ColorNameForeground)
	r.text.TextSize = theme.Size(theme.SizeNameText)
	r.text.TextStyle = fyne.TextStyle{Bold: r.cell.bold}
	r.text.Refresh()
}

func (r *editableCellRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *editableCellRenderer) Destroy()                     {}
