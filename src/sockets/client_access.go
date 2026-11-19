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
	// accessTimeout es el tiempo que el servidor espera las credenciales tras
	// enviar el desafío. Coincide con el margen del servidor para que el usuario
	// pueda escribir en el Paso 2 sin que la conexión expire.
	accessTimeout = 5 * time.Minute
	// sessionSetupTimeout limita el intercambio de set_period antes de entregar
	// la sesión ya autenticada a la GUI.
	sessionSetupTimeout = 30 * time.Second
)

// ErrAccessClosed indica que la conexión del Paso 1 se cerró antes de completar
// el login o el registro.
var ErrAccessClosed = errors.New("la conexión con el servidor está cerrada")

// AccessConnection representa una conexión TCP que ya superó la validación de
// conectividad/autorización del servidor (recibió un auth_challenge) pero aún no
// se ha autenticado. Permite mantener una única conexión entre el Paso 1 y el
// Paso 2 y reutilizarla si el usuario corrige sus credenciales.
type AccessConnection struct {
	conn    net.Conn
	reader  *messageReader
	address string
	ctx     context.Context

	// mu serializa las operaciones del protocolo de acceso.
	mu sync.Mutex
	// connMu protege la propiedad de conn y el cierre desde la UI sin esperar a
	// que termine una lectura bloqueada.
	connMu      sync.Mutex
	closed      bool
	stopWatcher func() bool

	nonce string
}

// ConnectAccess abre el socket TCP y valida la conectividad y la autorización
// por IP del cliente leyendo el primer desafío del servidor. No envía
// credenciales: el login o el registro se realizan después con Login/Register.
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

// Address devuelve la dirección IP:puerto validada en el Paso 1.
func (a *AccessConnection) Address() string {
	return a.address
}

// Login autentica la conexión mediante el desafío SHA-256 y, si el servidor lo
// acepta, configura el período de reporte y devuelve una ClientSession lista
// para la terminal y las métricas.
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
			// El servidor enviará un desafío nuevo después de la espera
			// progresiva; el siguiente intento lo consumirá.
			a.nonce = ""
		}
		return nil, authResultError(result)
	}
	return a.finishLocked(reportPeriodSeconds)
}

// Register valida los datos en el cliente, envía el SHA-256 de la contraseña y
// crea la cuenta en el servidor. Si el registro se acepta, la conexión queda
// autenticada y se entrega la sesión de terminal/métricas.
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

// Close cierra la conexión de acceso. Es seguro llamarlo mientras otra
// goroutine espera una respuesta: el cierre desbloquea la lectura sin necesidad
// de adquirir mu.
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

// ensureChallengeLocked obtiene el desafío actual. Tras un intento fallido el
// servidor envía uno nuevo cuando termina la espera progresiva, por lo que esta
// lectura puede bloquearse unos segundos.
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
			// Resultado rezagado de un intento de registro anterior; el
			// siguiente mensaje es el desafío nuevo.
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
			// Desafío adelantado del siguiente intento; se reutiliza.
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

	// La sesión pasa a ser dueña de conn: a partir de aquí Close ya no la cierra.
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
