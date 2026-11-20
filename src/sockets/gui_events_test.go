package sockets

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveGUIEventFileUsesLogDirectory(t *testing.T) {
	got := ResolveGUIEventFile(DefaultServerLogFile, "", "/etc")
	if got != DefaultGUIEventFile {
		t.Fatalf("event file = %q, want %q", got, DefaultGUIEventFile)
	}

	dir := t.TempDir()
	got = ResolveGUIEventFile("servidor.log", "", dir)
	want := filepath.Join(dir, GUIEventFileName)
	if got != want {
		t.Fatalf("event file = %q, want %q", got, want)
	}

	got = ResolveGUIEventFile(DefaultServerLogFile, "eventos.jsonl", dir)
	want = filepath.Join(dir, "eventos.jsonl")
	if got != want {
		t.Fatalf("explicit event file = %q, want %q", got, want)
	}
}

func TestGUIEventLogRecordsSessionCommandAndReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, GUIEventFileName)
	log, err := openGUIEventLog(path)
	if err != nil {
		t.Fatal(err)
	}
	log.session("connect", "100.83.168.60", "", false)
	log.session("login", "100.83.168.60", "admin", true)
	log.commandReceived("100.83.168.60", "admin", 4, "ls -la")
	log.reportSent("100.83.168.60", "admin", "enviado", nil)
	log.session("login", "10.0.0.2", "nuevo", true)
	log.session("disconnect", "10.0.0.2", "nuevo", true)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0444 != 0444 {
			t.Fatalf("permisos %o, el usuario de escritorio debe poder leer el archivo", info.Mode().Perm())
		}
	}

	events := readGUIEvents(t, path)
	if len(events) != 7 {
		t.Fatalf("eventos = %d, want 7 (ready + sesión + comando + reporte)", len(events))
	}
	if events[0].Type != "session" || events[0].Action != "ready" || events[0].ActiveClients != 0 {
		t.Fatalf("primer evento = %+v, want ready con 0 clientes", events[0])
	}
	command := events[3]
	if command.Type != "command_received" || command.Command != "ls -la" || command.CommandID != 4 {
		t.Fatalf("comando = %+v", command)
	}
	if command.User != "admin" || command.ClientIP != "100.83.168.60" || command.Clock == "" {
		t.Fatalf("comando sin origen o reloj: %+v", command)
	}
	report := events[4]
	if report.Type != "report_sent" || report.Status != "enviado" || report.Report != 1 {
		t.Fatalf("reporte = %+v", report)
	}
	if events[len(events)-1].ActiveClients != 1 {
		t.Fatalf("clientes activos = %d, want 1", events[len(events)-1].ActiveClients)
	}
}

func TestGUIEventLogNilIsHarmless(t *testing.T) {
	var log *guiEventLog
	log.session("login", "127.0.0.1", "admin", true)
	log.commandReceived("127.0.0.1", "admin", 1, "pwd")
	log.reportSent("127.0.0.1", "admin", "error", os.ErrClosed)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
}

func readGUIEvents(t *testing.T, path string) []GUIEvent {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var events []GUIEvent
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event GUIEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("json: %v", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}
