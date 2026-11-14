package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigResolvesUsersFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.conf")
	content := "ListenIP=127.0.0.1\n" +
		"Port=2306\n" +
		"AllowedClientIP=127.0.0.1\n" +
		"MaxAuthAttempts=3\n" +
		"UsersFile=users.conf\n" +
		"DefaultReportPeriodSeconds=5\n" +
		"MaxReportPeriodSeconds=60\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.UsersFile != filepath.Join(dir, "users.conf") {
		t.Fatalf("UsersFile = %q, want path relative to config directory", config.UsersFile)
	}
}

func TestLoadConfigRejectsUnknownSetting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.conf")
	if err := os.WriteFile(path, []byte("Unexpected=value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("unknown configuration setting was accepted")
	}
}
