package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"operativos/src/sockets"
)

type eventTail struct {
	path    string
	offset  int64
	info    os.FileInfo
	pending []byte
}

func newEventTail(path string) *eventTail {
	return &eventTail{path: path}
}

// ReadNew reads complete JSONL records without changing the source file. It
// detects truncation and rename-based rotation by comparing the opened file.
func (t *eventTail) ReadNew() ([]sockets.GUIEvent, error) {
	file, err := os.Open(t.path)
	if err != nil {
		return nil, fmt.Errorf("abrir flujo %s: %w", t.path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("consultar flujo %s: %w", t.path, err)
	}
	if t.info != nil && (!os.SameFile(t.info, info) || info.Size() < t.offset) {
		t.offset = 0
		t.pending = nil
	}
	if _, err := file.Seek(t.offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("posicionar flujo %s: %w", t.path, err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("leer flujo %s: %w", t.path, err)
	}
	t.offset += int64(len(data))
	t.info, err = file.Stat()
	if err != nil {
		return nil, fmt.Errorf("consultar flujo %s: %w", t.path, err)
	}

	data = append(t.pending, data...)
	lines := bytes.Split(data, []byte{'\n'})
	if len(lines) == 0 {
		return nil, nil
	}
	pending := append([]byte(nil), lines[len(lines)-1]...)

	events := make([]sockets.GUIEvent, 0, len(lines))
	for _, line := range lines[:len(lines)-1] {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var event sockets.GUIEvent
		if err := json.Unmarshal(line, &event); err != nil {
			t.pending = pending
			return events, fmt.Errorf("decodificar evento GUI: %w", err)
		}
		events = append(events, event)
	}
	t.pending = pending
	return events, nil
}
