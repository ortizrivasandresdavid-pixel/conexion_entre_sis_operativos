package sockets

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"operativos/src/login"
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
	if reportPeriodSeconds < 1 || reportPeriodSeconds > 3600 {
		return nil, fmt.Errorf("el período de reporte debe estar entre 1 y 3600 segundos")
	}
	if strings.TrimSpace(username) == "" {
		return nil, fmt.Errorf("el usuario no puede estar vacío")
	}

	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("conectar con el servidor: %w", err)
	}
	stopClose := context.AfterFunc(ctx, func() {
		_ = conn.Close()
	})
	defer stopClose()
	connected := false
	defer func() {
		if !connected {
			_ = conn.Close()
		}
	}()

	reader := newMessageReader(conn)
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, fmt.Errorf("configurar tiempo límite de autenticación: %w", err)
	}
	challenge, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("recibir desafío de autenticación: %w", err)
	}
	if challenge.Type == "auth_result" && !challenge.OK {
		return nil, authResultError(challenge)
	}
	if challenge.Type != "auth_challenge" || challenge.Nonce == "" {
		return nil, fmt.Errorf("respuesta de autenticación no válida")
	}
	proof := login.MakeProof(strings.TrimSpace(username), password, challenge.Nonce)
	if err := writeMessage(conn, Message{
		Type:     "authenticate",
		Username: strings.TrimSpace(username),
		Proof:    proof,
	}); err != nil {
		return nil, err
	}
	result, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("recibir resultado de autenticación: %w", err)
	}
	if result.Type != "auth_result" {
		return nil, fmt.Errorf("resultado de autenticación no válido")
	}
	if !result.OK {
		return nil, authResultError(result)
	}

	const periodRequestID = 1
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, fmt.Errorf("configurar tiempo límite de sesión: %w", err)
	}
	if err := writeMessage(conn, Message{
		Type:          "set_period",
		ID:            periodRequestID,
		PeriodSeconds: reportPeriodSeconds,
	}); err != nil {
		return nil, err
	}

	var initialMessages []Message
	for {
		message, err := reader.Read()
		if err != nil {
			return nil, fmt.Errorf("recibir confirmación del período de reporte: %w", err)
		}
		if message.Type == "command_result" && message.ID == periodRequestID {
			if !message.OK {
				if message.Error == "" {
					message.Error = "el servidor rechazó el período de reporte"
				}
				return nil, fmt.Errorf("%s", message.Error)
			}
			break
		}
		initialMessages = append(initialMessages, message)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("limpiar tiempo límite de conexión: %w", err)
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	session := &ClientSession{
		conn:   conn,
		ctx:    sessionCtx,
		cancel: cancel,
		events: make(chan ClientEvent, 16),
	}
	session.nextID.Store(periodRequestID)
	connected = true
	go session.readMessages(reader, initialMessages)
	return session, nil
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
