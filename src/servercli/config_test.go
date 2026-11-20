package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"operativos/src/sockets"
)

func TestLoadServerConfigResolvesDefaultEventFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.conf")
	content := "ListenIP=127.0.0.1\nPort=2306\nDefaultReportPeriodSeconds=5\nLogFile=server.log\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := loadServerConfig(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if config.ListenIP != "127.0.0.1" || config.Port != 2306 || config.Period != 5 {
		t.Fatalf("configuración inesperada: %+v", config)
	}
	want := filepath.Join(dir, sockets.GUIEventFileName)
	if config.EventFile != want {
		t.Fatalf("EventFile = %q; se esperaba %q", config.EventFile, want)
	}
}

func TestLoadServerConfigRejectsInvalidPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.conf")
	content := "ListenIP=127.0.0.1\nPort=70000\nDefaultReportPeriodSeconds=5\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := loadServerConfig(path, "")
	if err == nil || !strings.Contains(err.Error(), "Port") {
		t.Fatalf("se esperaba un error de puerto, se obtuvo %v", err)
	}
}
