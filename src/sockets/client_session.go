package sockets

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
)

type ClientEvent struct {
	Message Message
	Err     error
}

type ClientSession struct {
	conn    net.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	events  chan ClientEvent
	writeMu sync.Mutex
	nextID  atomic.Uint64
}

func ConnectClient(ctx context.Context, address string, reportPeriodSeconds int, username, password string) (*ClientSession, error) {
	access, err := ConnectAccess(ctx, address)
	if err != nil {
		return nil, err
	}
	defer access.Close()
	return access.Login(username, password, reportPeriodSeconds)
}

func (s *ClientSession) Events() <-chan ClientEvent {
	return s.events
}

func (s *ClientSession) SendCommand(command string) (uint64, error) {
	select {
	case <-s.ctx.Done():
		return 0, fmt.Errorf("la sesión del cliente está cerrada")
	default:
	}

	id := s.nextID.Add(1)
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := writeMessage(s.conn, Message{Type: "command", ID: id, Command: strings.TrimSpace(command)}); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *ClientSession) Close() {
	s.cancel()
	_ = s.conn.Close()
}

func (s *ClientSession) readMessages(reader *messageReader, initialMessages []Message) {
	defer close(s.events)
	defer s.cancel()
	defer s.conn.Close()
	stopClose := context.AfterFunc(s.ctx, func() {
		_ = s.conn.Close()
	})
	defer stopClose()

	for _, message := range initialMessages {
		select {
		case s.events <- ClientEvent{Message: message}:
		case <-s.ctx.Done():
			return
		}
	}

	for {
		message, err := reader.Read()
		select {
		case s.events <- ClientEvent{Message: message, Err: err}:
		case <-s.ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}
