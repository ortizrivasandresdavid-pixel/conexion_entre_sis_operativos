package sockets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"operativos/src/login"
)

const (
	accessTimeout       = 5 * time.Minute
	sessionSetupTimeout = 30 * time.Second
)

var ErrAccessClosed = errors.New("la conexión con el servidor está cerrada")

type AccessConnection struct {
	conn    net.Conn
	reader  *messageReader
	address string
	ctx     context.Context

	mu          sync.Mutex
	connMu      sync.Mutex
	closed      bool
	stopWatcher func() bool

	nonce string
}

func ConnectAccess(ctx context.Context, address string) (*AccessConnection, error) {
	if strings.TrimSpace(address) == "" {
		return nil, fmt.Errorf("la dirección del servidor no puede estar vacía")
	}
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("conectar con el servidor: %w", err)
	}

	access := &AccessConnection{
		conn:    conn,
		reader:  newMessageReader(conn),
		address: address,
		ctx:     ctx,
	}
	access.stopWatcher = context.AfterFunc(ctx, func() {
		_ = conn.Close()
	})

	if err := conn.SetDeadline(time.Now().Add(accessTimeout)); err != nil {
		_ = access.Close()
		return nil, fmt.Errorf("configurar tiempo límite de acceso: %w", err)
	}
	message, err := access.reader.Read()
	if err != nil {
		_ = access.Close()
		return nil, access.readError(err)
	}
	switch {
	case message.Type == "auth_result" && !message.OK:
		_ = access.Close()
		return nil, authResultError(message)
	case message.Type == "auth_challenge" && message.Nonce != "":
		access.nonce = message.Nonce
		return access, nil
	default:
		_ = access.Close()
		return nil, fmt.Errorf("respuesta de conexión no válida del servidor: %s", message.Type)
	}
}

func (a *AccessConnection) Address() string {
	return a.address
}

func (a *AccessConnection) Login(username, password string, reportPeriodSeconds int) (*ClientSession, error) {
	if err := validateReportPeriod(reportPeriodSeconds); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("el usuario no puede estar vacío")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.isClosed() {
		return nil, ErrAccessClosed
	}
	if err := a.prepareAuthLocked(); err != nil {
		return nil, err
	}
	request := Message{
		Type:     "authenticate",
		Username: username,
		Proof:    login.MakeProof(username, password, a.nonce),
	}
	if err := writeMessage(a.conn, request); err != nil {
		return nil, a.writeError(err)
	}
	result, err := a.readExpectedLocked("auth_result")
	if err != nil {
		return nil, err
	}
	if !result.OK {
		locked := result.Code == "AUTH_LOCKED" || result.AttemptsRemaining <= 0
		if locked {
			_ = a.Close()
		} else {
			a.nonce = ""
		}
		return nil, authResultError(result)
	}
	return a.finishLocked(reportPeriodSeconds)
}

func (a *AccessConnection) Register(username, password, confirmation string, reportPeriodSeconds int) (*ClientSession, error) {
	if err := validateReportPeriod(reportPeriodSeconds); err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if err := login.ValidateUsername(username); err != nil {
		return nil, err
	}
	if password == "" {
		return nil, fmt.Errorf("la contraseña no puede estar vacía")
	}
	if password != confirmation {
		return nil, fmt.Errorf("las contraseñas no coinciden")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.isClosed() {
		return nil, ErrAccessClosed
	}
	if err := a.prepareAuthLocked(); err != nil {
		return nil, err
	}
	request := Message{
		Type:         "register",
		Username:     username,
		PasswordHash: login.HashPassword(password),
		Proof:        login.MakeProof(username, password, a.nonce),
	}
	if err := writeMessage(a.conn, request); err != nil {
		return nil, a.writeError(err)
	}
	result, err := a.readExpectedLocked("register_result")
	if err != nil {
		return nil, err
	}
	if !result.OK {
		a.nonce = ""
		if result.Code == "AUTH_LOCKED" {
			_ = a.Close()
		}
		return nil, registerResultError(result)
	}
	return a.finishLocked(reportPeriodSeconds)
}

func (a *AccessConnection) Close() error {
	a.connMu.Lock()
	defer a.connMu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	if a.stopWatcher != nil {
		a.stopWatcher()
		a.stopWatcher = nil
	}
	return a.conn.Close()
}

func (a *AccessConnection) isClosed() bool {
	a.connMu.Lock()
	defer a.connMu.Unlock()
	return a.closed
}

func (a *AccessConnection) prepareAuthLocked() error {
	if err := a.conn.SetDeadline(time.Now().Add(accessTimeout)); err != nil {
		return fmt.Errorf("configurar tiempo límite de autenticación: %w", err)
	}
	return a.ensureChallengeLocked()
}

func (a *AccessConnection) ensureChallengeLocked() error {
	if a.nonce != "" {
		return nil
	}
	for {
		message, err := a.reader.Read()
		if err != nil {
			return a.readError(err)
		}
		switch message.Type {
		case "auth_challenge":
			if message.Nonce == "" {
				continue
			}
			a.nonce = message.Nonce
			return nil
		case "auth_result":
			if message.OK {
				return fmt.Errorf("el servidor ya autenticó esta conexión")
			}
			if message.Code == "AUTH_LOCKED" || message.AttemptsRemaining <= 0 {
				_ = a.Close()
			}
			return authResultError(message)
		case "register_result":
			continue
		default:
			return fmt.Errorf("respuesta inesperada del servidor: %s", message.Type)
		}
	}
}

func (a *AccessConnection) readExpectedLocked(expected string) (Message, error) {
	for {
		message, err := a.reader.Read()
		if err != nil {
			return Message{}, a.readError(err)
		}
		switch message.Type {
		case expected:
			return message, nil
		case "auth_challenge":
			if message.Nonce != "" {
				a.nonce = message.Nonce
			}
		default:
			return Message{}, fmt.Errorf("respuesta inesperada del servidor: %s", message.Type)
		}
	}
}

func (a *AccessConnection) finishLocked(reportPeriodSeconds int) (*ClientSession, error) {
	const periodRequestID uint64 = 1
	if err := a.conn.SetDeadline(time.Now().Add(sessionSetupTimeout)); err != nil {
		return nil, fmt.Errorf("configurar tiempo límite de sesión: %w", err)
	}
	if err := writeMessage(a.conn, Message{
		Type:          "set_period",
		ID:            periodRequestID,
		PeriodSeconds: reportPeriodSeconds,
	}); err != nil {
		return nil, a.writeError(err)
	}

	var initialMessages []Message
	for {
		message, err := a.reader.Read()
		if err != nil {
			return nil, a.readError(fmt.Errorf("confirmar el período de reporte: %w", err))
		}
		if message.Type == "command_result" && message.ID == periodRequestID {
			if !message.OK {
				text := message.Error
				if text == "" {
					text = "el servidor rechazó el período de reporte"
				}
				_ = a.Close()
				return nil, fmt.Errorf("%s", text)
			}
			break
		}
		initialMessages = append(initialMessages, message)
	}
	if err := a.conn.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("limpiar tiempo límite de conexión: %w", err)
	}

	a.connMu.Lock()
	if a.closed {
		a.connMu.Unlock()
		return nil, ErrAccessClosed
	}
	a.closed = true
	if a.stopWatcher != nil {
		a.stopWatcher()
		a.stopWatcher = nil
	}
	a.connMu.Unlock()

	sessionCtx, cancel := context.WithCancel(a.ctx)
	session := &ClientSession{
		conn:   a.conn,
		ctx:    sessionCtx,
		cancel: cancel,
		events: make(chan ClientEvent, 16),
	}
	session.nextID.Store(periodRequestID)
	go session.readMessages(a.reader, initialMessages)
	return session, nil
}

func (a *AccessConnection) readError(err error) error {
	_ = a.Close()
	if a.ctx.Err() != nil {
		return a.ctx.Err()
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return fmt.Errorf("el servidor cerró la conexión; verifica la IP autorizada y que el servicio esté activo")
	}
	return fmt.Errorf("leer respuesta del servidor: %w", err)
}

func (a *AccessConnection) writeError(err error) error {
	_ = a.Close()
	if a.ctx.Err() != nil {
		return a.ctx.Err()
	}
	return fmt.Errorf("enviar solicitud al servidor: %w", err)
}

func validateReportPeriod(seconds int) error {
	if seconds < 1 || seconds > 3600 {
		return fmt.Errorf("el período de reporte debe estar entre 1 y 3600 segundos")
	}
	return nil
}
