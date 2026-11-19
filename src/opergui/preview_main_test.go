package main

import (
	"image/png"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestPreviewMainView(t *testing.T) {
	program := test.NewApp()
	defer program.Quit()
	program.Settings().SetTheme(newTerminalTheme())

	g := &gui{period: 5}
	view := newMainView(g)
	view.reset("100.83.168.60:2306", "admin", 5)
	window := test.NewWindow(view.root)
	window.Resize(fyne.NewSize(mainWindowWidth, mainWindowHeight))
	file, err := os.Create("../../.tmp/main.png")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, window.Canvas().Capture()); err != nil {
		t.Fatal(err)
	}
}
