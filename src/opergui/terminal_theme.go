package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type terminalTheme struct {
	fallback fyne.Theme
}

func newTerminalTheme() fyne.Theme {
	return terminalTheme{fallback: theme.DarkTheme()}
}

func (t terminalTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground, theme.ColorNameInputBackground:
		return color.NRGBA{A: 0xff}
	case theme.ColorNameDisabled,
		theme.ColorNameDisabledButton,
		theme.ColorNameForeground,
		theme.ColorNamePlaceHolder:
		return color.NRGBA{G: 0xff, A: 0xff}
	default:
		return t.fallback.Color(name, variant)
	}
}

func (t terminalTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.fallback.Font(style)
}

func (t terminalTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.fallback.Icon(name)
}

func (t terminalTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.fallback.Size(name)
}
