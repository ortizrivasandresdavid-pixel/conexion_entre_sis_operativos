package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestAccessTabsShareBodySize(t *testing.T) {
	program := test.NewApp()
	defer program.Quit()
	program.Settings().SetTheme(newTerminalTheme())

	view := newAccessView(&gui{})
	bodySize := fyne.NewSize(accessCardWidth-52, accessBodyHeight)
	view.body.Resize(bodySize)
	loginSize := view.loginForm.Size()

	view.switchTab(1)
	view.body.Resize(fyne.NewSize(bodySize.Width+1, bodySize.Height+1))
	view.body.Resize(bodySize)
	registerSize := view.registerForm.Size()

	if loginSize != bodySize || registerSize != bodySize {
		t.Fatalf("form sizes = %v and %v, want %v", loginSize, registerSize, bodySize)
	}
	if view.loginForm.Visible() || !view.registerForm.Visible() {
		t.Fatal("register tab did not hide the login form")
	}

	view.switchTab(0)
	if !view.loginForm.Visible() || view.registerForm.Visible() {
		t.Fatal("login tab did not hide the register form")
	}

	cardSize := view.root.MinSize()
	if cardSize.Width != accessCardWidth || cardSize.Height != accessCardHeight {
		t.Fatalf("card size = %v, want %dx%d", cardSize, accessCardWidth, accessCardHeight)
	}
}
