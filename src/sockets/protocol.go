package sockets

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	"operativos/src/metrics"
)

type Message struct {
	Type              string            `json:"type"`
	ID                uint64            `json:"id,omitempty"`
	Username          string            `json:"username,omitempty"`
	Nonce             string            `json:"nonce,omitempty"`
	Proof             string            `json:"proof,omitempty"`
	Command           string            `json:"command,omitempty"`
	Output            string            `json:"output,omitempty"`
	Error             string            `json:"error,omitempty"`
	OK                bool              `json:"ok,omitempty"`
	AttemptsRemaining int               `json:"attempts_remaining,omitempty"`
	RetryAfterSeconds int               `json:"retry_after_seconds,omitempty"`
	PeriodSeconds     int               `json:"period_seconds,omitempty"`
	ExitCode          int               `json:"exit_code,omitempty"`
	Dir               string            `json:"dir,omitempty"`
	Metrics           *metrics.Snapshot `json:"metrics,omitempty"`
}

type messageReader struct {
	scanner *bufio.Scanner
}

func newMessageReader(reader io.Reader) *messageReader {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	return &messageReader{scanner: scanner}
}

func (r *messageReader) Read() (Message, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return Message{}, fmt.Errorf("leer mensaje: %w", err)
		}
		return Message{}, io.EOF
	}
	var message Message
	if err := json.Unmarshal(r.scanner.Bytes(), &message); err != nil {
		return Message{}, fmt.Errorf("decodificar mensaje: %w", err)
	}
	return message, nil
}

func writeMessage(writer io.Writer, message Message) error {
	if err := json.NewEncoder(writer).Encode(message); err != nil {
		return fmt.Errorf("enviar mensaje: %w", err)
	}
	return nil
}
