package main

import (
	"strings"
	"testing"

	"operativos/src/sockets"
)

func TestFormatCommandRemovesTerminalControlCharacters(t *testing.T) {
	line := formatCommand(sockets.GUIEvent{
		Clock:    "19:55:14",
		User:     "nuevo",
		ClientIP: "100.83.168.60",
		Command:  "ls \x1b[2J\n-la",
	})
	if strings.ContainsRune(line, '\x1b') {
		t.Fatalf("el comando conserva una secuencia de control: %q", line)
	}
	if !strings.Contains(line, `ls [2J\n-la`) {
		t.Fatalf("el comando no quedó visible en una sola línea: %q", line)
	}
}

func TestProgressBarClampsPercent(t *testing.T) {
	if got, want := progressBar(-10), "[--------------------]"; got != want {
		t.Fatalf("barra para valor negativo = %q; se esperaba %q", got, want)
	}
	if got, want := progressBar(125), "[####################]"; got != want {
		t.Fatalf("barra para valor mayor que 100 = %q; se esperaba %q", got, want)
	}
}

func TestActivityStateCountsSuccessfulReportsAndCommands(t *testing.T) {
	state := activityState{}
	state.apply([]sockets.GUIEvent{
		{Type: "report_sent", Status: "enviado", ActiveClients: 2},
		{Type: "report_sent", Status: "error", ActiveClients: 2},
		{Type: "command_received", Command: "pwd", ActiveClients: 2},
	})

	if state.reportCount != 1 || state.commandCount != 1 || state.activeClients != 2 {
		t.Fatalf("contadores inesperados: %+v", state)
	}
}
