package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEventTailReadsCompleteLinesAndKeepsPartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"command_received\",\"command\":\"one\"}\n{\"type\":\"command_received\""), 0600); err != nil {
		t.Fatal(err)
	}
	tailer := newEventTail(path)

	events, err := tailer.ReadNew()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Command != "one" {
		t.Fatalf("eventos iniciales inesperados: %+v", events)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(",\"command\":\"two\"}\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	events, err = tailer.ReadNew()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Command != "two" {
		t.Fatalf("evento añadido inesperado: %+v", events)
	}
}

func TestEventTailDoesNotCorruptCompleteLineBeforePartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte("{\"type\":\"command_received\",\"command\":\"first\""), 0600); err != nil {
		t.Fatal(err)
	}
	tailer := newEventTail(path)
	if events, err := tailer.ReadNew(); err != nil || len(events) != 0 {
		t.Fatalf("lectura parcial inicial = (%+v, %v)", events, err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("}\n{\"type\":\"command_received\",\"command\":\"second"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	events, err := tailer.ReadNew()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Command != "first" {
		t.Fatalf("evento completo se corrompió: %+v", events)
	}

	file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\"}\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	events, err = tailer.ReadNew()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Command != "second" {
		t.Fatalf("evento parcial no se completó: %+v", events)
	}
}

func TestEventTailContinuesAfterRenameRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	backup := path + ".1"
	if err := os.WriteFile(path, []byte("{\"type\":\"command_received\",\"command\":\"old\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tailer := newEventTail(path)
	if events, err := tailer.ReadNew(); err != nil || len(events) != 1 {
		t.Fatalf("lectura inicial = (%+v, %v)", events, err)
	}
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\"type\":\"command_received\",\"command\":\"new\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	events, err := tailer.ReadNew()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Command != "new" {
		t.Fatalf("eventos tras rotación inesperados: %+v", events)
	}
}

func TestEventTailReturnsValidEventsBeforeMalformedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	data := "{\"type\":\"command_received\",\"command\":\"valid\"}\n{broken}\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	events, err := newEventTail(path).ReadNew()
	if err == nil {
		t.Fatal("se esperaba error por la línea JSON inválida")
	}
	if len(events) != 1 || events[0].Command != "valid" {
		t.Fatalf("no se preservó el evento válido: %+v", events)
	}
}
