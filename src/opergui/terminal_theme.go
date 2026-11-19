package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

var (
	terminalGreen      = color.NRGBA{R: 0x66, G: 0xf0, B: 0x8a, A: 0xff}
	terminalGreenSoft  = color.NRGBA{R: 0x4f, G: 0xc4, B: 0x6c, A: 0xff}
	terminalGreenDim   = color.NRGBA{R: 0x33, G: 0x7a, B: 0x45, A: 0xff}
	terminalGreenMuted = color.NRGBA{R: 0x1f, G: 0x45, B: 0x2b, A: 0xff}
	terminalBackground = color.NRGBA{R: 0x05, G: 0x0a, B: 0x05, A: 0xff}
	terminalPanel      = color.NRGBA{R: 0x0b, G: 0x17, B: 0x0d, A: 0xff}
	terminalInput      = color.NRGBA{R: 0x08, G: 0x13, B: 0x0a, A: 0xff}
	terminalBorder     = color.NRGBA{R: 0x35, G: 0x7a, B: 0x45, A: 0xff}
	terminalError      = color.NRGBA{R: 0xff, G: 0x7a, B: 0x7a, A: 0xff}
	terminalOnGreen    = color.NRGBA{R: 0x03, G: 0x12, B: 0x07, A: 0xff}
)

type terminalTheme struct {
	fallback fyne.Theme
}

func newTerminalTheme() fyne.Theme {
	return terminalTheme{fallback: theme.DarkTheme()}
}

func (t terminalTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground,
		theme.ColorNameMenuBackground,
		theme.ColorNameOverlayBackground:
		return terminalBackground
	case theme.ColorNameInputBackground:
		return terminalInput
	case theme.ColorNameButton, theme.ColorNameHeaderBackground:
		return terminalPanel
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 0x0d, G: 0x1a, B: 0x10, A: 0xff}
	case theme.ColorNameDisabled:
		return terminalGreenMuted
	case theme.ColorNameForeground,
		theme.ColorNameHyperlink:
		return terminalGreen
	case theme.ColorNameForegroundOnSuccess:
		return terminalOnGreen
	case theme.ColorNamePlaceHolder:
		return terminalGreenDim
	case theme.ColorNamePrimary, theme.ColorNameSuccess, theme.ColorNameFocus:
		return terminalGreen
	case theme.ColorNameForegroundOnPrimary:
		return terminalOnGreen
	case theme.ColorNameError:
		return terminalError
	case theme.ColorNameForegroundOnError:
		return terminalOnGreen
	case theme.ColorNameWarning:
		return terminalGreenSoft
	case theme.ColorNameForegroundOnWarning:
		return terminalOnGreen
	case theme.ColorNameInputBorder, theme.ColorNameInnerWindowBorder:
		return terminalBorder
	case theme.ColorNameInnerWindowBorderInactive:
		return terminalGreenMuted
	case theme.ColorNameSeparator, theme.ColorNameScrollBar:
		return terminalGreenDim
	case theme.ColorNameScrollBarBackground:
		return terminalBackground
	case theme.ColorNameHover, theme.ColorNamePressed, theme.ColorNameSelection:
		return color.NRGBA{R: 0x1c, G: 0x3a, B: 0x24, A: 0xff}
	case theme.ColorNameShadow:
		return color.NRGBA{A: 0xb0}
	default:
		return t.fallback.Color(name, variant)
	}
}

// Font fuerza la tipografía monoespaciada en toda la aplicación, salvo los
// iconos simbólicos, para mantener la estética de terminal.
func (t terminalTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Symbol {
		return t.fallback.Font(style)
	}
	return t.fallback.Font(fyne.TextStyle{
		Bold:      style.Bold,
		Italic:    style.Italic,
		Monospace: true,
	})
}

func (t terminalTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.fallback.Icon(name)
}

func (t terminalTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.fallback.Size(name)
}
