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

func TestLoadConfigUsesDefaultLogSettings(t *testing.T) {
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
	if config.LogFile != defaultLogPath {
		t.Fatalf("LogFile = %q, want %q", config.LogFile, defaultLogPath)
	}
	if config.LogMaxSizeMB != defaultLogMaxSizeMB || config.LogBackups != defaultLogBackups {
		t.Fatalf("log defaults = (%d, %d), want (%d, %d)",
			config.LogMaxSizeMB, config.LogBackups, defaultLogMaxSizeMB, defaultLogBackups)
	}
	if config.EventFile != "/var/log/proyecto-operativos/eventos-gui.jsonl" {
		t.Fatalf("EventFile = %q, want the log directory default", config.EventFile)
	}
}

func TestLoadConfigResolvesRelativeEventFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.conf")
	content := "ListenIP=127.0.0.1\n" +
		"Port=2306\n" +
		"AllowedClientIP=127.0.0.1\n" +
		"MaxAuthAttempts=3\n" +
		"UsersFile=users.conf\n" +
		"DefaultReportPeriodSeconds=5\n" +
		"MaxReportPeriodSeconds=60\n" +
		"EventFile=eventos.jsonl\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "eventos.jsonl")
	if config.EventFile != want {
		t.Fatalf("EventFile = %q, want %q", config.EventFile, want)
	}
}

func TestLoadConfigResolvesRelativeLogFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.conf")
	content := "ListenIP=127.0.0.1\n" +
		"Port=2306\n" +
		"AllowedClientIP=127.0.0.1\n" +
		"MaxAuthAttempts=3\n" +
		"UsersFile=users.conf\n" +
		"DefaultReportPeriodSeconds=5\n" +
		"MaxReportPeriodSeconds=60\n" +
		"LogFile=logs/servidor.log\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "logs", "servidor.log")
	if config.LogFile != want {
		t.Fatalf("LogFile = %q, want %q", config.LogFile, want)
	}
}

func TestResolveConfigPathHonorsExplicitPath(t *testing.T) {
	path, err := resolveConfigPath("ruta/elegida.conf", true)
	if err != nil {
		t.Fatal(err)
	}
	if path != "ruta/elegida.conf" {
		t.Fatalf("path = %q, want the explicit value", path)
	}
}
