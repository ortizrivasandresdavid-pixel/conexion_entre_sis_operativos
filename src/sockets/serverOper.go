package sockets

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"operativos/src/login"
	"operativos/src/metrics"
	"operativos/src/shell"
)

type ServerConfig struct {
	ListenIP                   string
	Port                       int
	AllowedClientIP            string
	MaxAuthAttempts            int
	UsersFile                  string
	DefaultReportPeriodSeconds int
	MaxReportPeriodSeconds     int
}

func RunServer(ctx context.Context, config ServerConfig) error {
	if err := validateServerConfig(config); err != nil {
		return err
	}
	// Falla al arrancar (y no en la primera conexión) si el servidor no tiene
	// bash/sh disponible para ejecutar los comandos remotos.
	if _, err := shell.NewSession(); err != nil {
		return fmt.Errorf("shell remoto no disponible: %w", err)
	}
	users, err := login.LoadUsers(config.UsersFile)
	if err != nil {
		return err
	}
	attemptLimiter := login.NewAttemptLimiter()

	listener, err := net.Listen("tcp", net.JoinHostPort(config.ListenIP, strconv.Itoa(config.Port)))
	if err != nil {
		return fmt.Errorf("abrir socket servidor: %w", err)
	}
	var handlers sync.WaitGroup
	var activeConnections sync.Map
	stopClose := context.AfterFunc(ctx, func() {
		_ = listener.Close()
		activeConnections.Range(func(key, _ any) bool {
			_ = key.(net.Conn).Close()
			return true
		})
	})
	defer func() {
		stopClose()
		_ = listener.Close()
		activeConnections.Range(func(key, _ any) bool {
			_ = key.(net.Conn).Close()
			return true
		})
		handlers.Wait()
	}()

	log.Printf("Servidor escuchando en %s; IP permitida: %s", listener.Addr(), config.AllowedClientIP)
	slots := make(chan struct{}, 32)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("aceptar conexión: %w", err)
		}
		if ctx.Err() != nil {
			_ = conn.Close()
			continue
		}
		select {
		case slots <- struct{}{}:
			activeConnections.Store(conn, struct{}{})
			handlers.Add(1)
			go func() {
				defer handlers.Done()
				defer func() { <-slots }()
				defer activeConnections.Delete(conn)
				if err := serveConnection(ctx, conn, config, users, attemptLimiter); err != nil {
					log.Printf("Conexión %s: %v", conn.RemoteAddr(), err)
				}
			}()
		default:
			_ = conn.Close()
			log.Printf("Se rechazó %s: límite de conexiones alcanzado", conn.RemoteAddr())
		}
	}
}

func validateServerConfig(config ServerConfig) error {
	if net.ParseIP(config.ListenIP) == nil {
		return fmt.Errorf("ListenIP no es una dirección IP válida")
	}
	if config.Port < 1 || config.Port > 65535 {
		return fmt.Errorf("Port debe estar entre 1 y 65535")
	}
	if config.MaxAuthAttempts < 1 {
		return fmt.Errorf("MaxAuthAttempts debe ser mayor que cero")
	}
	if strings.TrimSpace(config.UsersFile) == "" {
		return fmt.Errorf("UsersFile no puede estar vacío")
	}
	if config.DefaultReportPeriodSeconds < 1 ||
		config.DefaultReportPeriodSeconds > config.MaxReportPeriodSeconds ||
		config.MaxReportPeriodSeconds > 3600 {
		return fmt.Errorf("períodos de reporte no válidos (rango permitido: 1..3600 segundos)")
	}
	if _, err := parseAllowedIP(config.AllowedClientIP); err != nil {
		return err
	}
	return nil
}

func serveConnection(
	ctx context.Context,
	conn net.Conn,
	config ServerConfig,
	users map[string]string,
	attemptLimiter *login.AttemptLimiter,
) error {
	defer conn.Close()
	allowed, err := parseAllowedIP(config.AllowedClientIP)
	if err != nil {
		return err
	}
	remoteIP, err := remoteIP(conn.RemoteAddr())
	if err != nil {
		return err
	}
	if !allowed(remoteIP) {
		return fmt.Errorf("IP no autorizada: %s", remoteIP)
	}

	reader := newMessageReader(conn)
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return fmt.Errorf("configurar tiempo límite de autenticación: %w", err)
	}
	if err := authenticateServer(ctx, conn, reader, users, config.MaxAuthAttempts, attemptLimiter, remoteIP.String()); err != nil {
		return err
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return fmt.Errorf("limpiar tiempo límite de conexión: %w", err)
	}
	log.Printf("Cliente autenticado: %s", remoteIP)
	terminal, err := shell.NewSession()
	if err != nil {
		return fmt.Errorf("preparar shell remoto: %w", err)
	}
	log.Printf("Sesión de shell de %s iniciada en %s", remoteIP, terminal.Dir())
	return runSession(ctx, conn, reader, config, terminal)
}

func authenticateServer(
	ctx context.Context,
	conn net.Conn,
	reader *messageReader,
	users map[string]string,
	maxAttempts int,
	attemptLimiter *login.AttemptLimiter,
	clientIP string,
) error {
	for {
		remaining, retryAfter := attemptLimiter.Check(clientIP, maxAttempts)
		if retryAfter > 0 {
			code := "AUTH_BACKOFF"
			message := fmt.Sprintf("autenticación bloqueada temporalmente; intente de nuevo en %d segundos", durationSeconds(retryAfter))
			if remaining == 0 {
				code = "AUTH_LOCKED"
				message = "se agotaron los intentos de autenticación"
			}
			if err := sendAuthResult(conn, Message{
				Code:              code,
				Error:             message,
				AttemptsRemaining: remaining,
				RetryAfterSeconds: durationSeconds(retryAfter),
			}); err != nil {
				return err
			}
			if remaining == 0 {
				if err := conn.Close(); err != nil {
					return fmt.Errorf("cerrar conexión bloqueada: %w", err)
				}
			}
			return fmt.Errorf("autenticación bloqueada temporalmente para %s", clientIP)
		}
		if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
			return fmt.Errorf("configurar tiempo límite de autenticación: %w", err)
		}

		nonceBytes := make([]byte, 32)
		if _, err := rand.Read(nonceBytes); err != nil {
			return fmt.Errorf("generar desafío de autenticación: %w", err)
		}
		nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
		if err := writeMessage(conn, Message{Type: "auth_challenge", Nonce: nonce}); err != nil {
			return err
		}

		request, err := reader.Read()
		if err != nil {
			return err
		}
		if request.Type != "authenticate" || request.Username == "" {
			return fmt.Errorf("solicitud de autenticación inválida")
		}

		remaining, retryAfter = attemptLimiter.Check(clientIP, maxAttempts)
		if retryAfter > 0 {
			code := "AUTH_BACKOFF"
			message := fmt.Sprintf("autenticación bloqueada temporalmente; intente de nuevo en %d segundos", durationSeconds(retryAfter))
			if remaining == 0 {
				code = "AUTH_LOCKED"
				message = "se agotaron los intentos de autenticación"
			}
			if err := sendAuthResult(conn, Message{
				Code:              code,
				Error:             message,
				AttemptsRemaining: remaining,
				RetryAfterSeconds: durationSeconds(retryAfter),
			}); err != nil {
				return err
			}
			if remaining == 0 {
				if err := conn.Close(); err != nil {
					return fmt.Errorf("cerrar conexión bloqueada: %w", err)
				}
				return fmt.Errorf("se agotaron los intentos de autenticación para %s", clientIP)
			}
			if err := waitForAuthRetry(ctx, retryAfter); err != nil {
				return err
			}
			continue
		}

		ok := login.VerifyProof(users, request.Username, nonce, request.Proof)
		if ok {
			attemptLimiter.RecordSuccess(clientIP)
			return sendAuthResult(conn, Message{OK: true, AttemptsRemaining: maxAttempts})
		}

		remaining, retryAfter = attemptLimiter.RecordFailure(clientIP, maxAttempts)
		if err := conn.SetDeadline(time.Now().Add(retryAfter + 30*time.Second)); err != nil {
			return fmt.Errorf("configurar espera de autenticación: %w", err)
		}
		code := "AUTH_BACKOFF"
		message := fmt.Sprintf("credenciales incorrectas; intente de nuevo en %d segundos", durationSeconds(retryAfter))
		if remaining == 0 {
			code = "AUTH_LOCKED"
			message = "se agotaron los intentos de autenticación"
		}
		if err := sendAuthResult(conn, Message{
			Code:              code,
			Error:             message,
			AttemptsRemaining: remaining,
			RetryAfterSeconds: durationSeconds(retryAfter),
		}); err != nil {
			return err
		}
		if remaining == 0 {
			if err := conn.Close(); err != nil {
				return fmt.Errorf("cerrar conexión bloqueada: %w", err)
			}
			return fmt.Errorf("se agotaron los intentos de autenticación para %s", clientIP)
		}

		if err := waitForAuthRetry(ctx, retryAfter); err != nil {
			return err
		}
	}
}

func sendAuthResult(conn net.Conn, result Message) error {
	result.Type = "auth_result"
	return writeMessage(conn, result)
}

func durationSeconds(duration time.Duration) int {
	if duration <= 0 {
		return 0
	}
	return int((duration + time.Second - 1) / time.Second)
}

func waitForAuthRetry(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type readEvent struct {
	message Message
	err     error
}

type commandResult struct {
	id       uint64
	output   string
	exitCode int
	dir      string
	err      error
}

func runSession(
	ctx context.Context,
	conn net.Conn,
	reader *messageReader,
	config ServerConfig,
	terminal *shell.Session,
) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	incoming := make(chan readEvent, 1)
	go func() {
		defer close(incoming)
		for {
			message, err := reader.Read()
			select {
			case incoming <- readEvent{message: message, err: err}:
			case <-sessionCtx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	results := make(chan commandResult, 1)
	collector := metrics.NewCollector()
	period := time.Duration(config.DefaultReportPeriodSeconds) * time.Second
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	commandRunning := false
	var sessionHistory []string

	for {
		select {
		case <-sessionCtx.Done():
			return sessionCtx.Err()
		case event, open := <-incoming:
			if !open {
				return nil
			}
			if event.err != nil {
				if event.err == context.Canceled {
					return nil
				}
				return event.err
			}
			switch event.message.Type {
			case "set_period":
				seconds := event.message.PeriodSeconds
				if seconds < 1 || seconds > config.MaxReportPeriodSeconds {
					if err := writeMessage(conn, Message{Type: "command_result", ID: event.message.ID, Error: fmt.Sprintf("el período debe estar entre 1 y %d segundos", config.MaxReportPeriodSeconds)}); err != nil {
						return err
					}
					continue
				}
				ticker.Reset(time.Duration(seconds) * time.Second)
				if err := writeMessage(conn, Message{Type: "command_result", ID: event.message.ID, OK: true, Output: fmt.Sprintf("Período de reporte: %d segundos", seconds)}); err != nil {
					return err
				}
			case "command":
				command := strings.TrimSpace(event.message.Command)
				if commandRunning {
					if err := writeMessage(conn, Message{Type: "command_result", ID: event.message.ID, Error: "espera a que termine el comando actual"}); err != nil {
						return err
					}
					continue
				}
				if output, handled := sessionHistoryCommand(command, &sessionHistory); handled {
					if err := writeMessage(conn, Message{Type: "command_result", ID: event.message.ID, OK: true, Output: output}); err != nil {
						return err
					}
					continue
				}
				if command == "bye" {
					sessionHistory = append(sessionHistory, command)
					if err := writeMessage(conn, Message{Type: "command_result", ID: event.message.ID, OK: true, Output: "Sesión cerrada"}); err != nil {
						return err
					}
					return nil
				}
				sessionHistory = append(sessionHistory, command)
				commandRunning = true
				go func(id uint64, command string) {
					output, err := terminal.Execute(sessionCtx, command)
					select {
					case results <- commandResult{
						id:       id,
						output:   output.Output,
						exitCode: output.ExitCode,
						dir:      output.Dir,
						err:      err,
					}:
					case <-sessionCtx.Done():
					}
				}(event.message.ID, event.message.Command)
			default:
				if err := writeMessage(conn, Message{Type: "command_result", ID: event.message.ID, Error: "tipo de solicitud no reconocido"}); err != nil {
					return err
				}
			}
		case result := <-results:
			commandRunning = false
			response := Message{
				Type:     "command_result",
				ID:       result.id,
				OK:       result.err == nil,
				Output:   result.output,
				ExitCode: result.exitCode,
				Dir:      result.dir,
			}
			if result.err != nil {
				response.Error = result.err.Error()
			}
			if err := writeMessage(conn, response); err != nil {
				return err
			}
		case <-ticker.C:
			snapshot, err := collector.Snapshot()
			if err != nil {
				if writeErr := writeMessage(conn, Message{Type: "metric", Error: err.Error()}); writeErr != nil {
					return writeErr
				}
				continue
			}
			if err := writeMessage(conn, Message{Type: "metric", Metrics: &snapshot}); err != nil {
				return err
			}
		}
	}
}

func sessionHistoryCommand(command string, history *[]string) (string, bool) {
	switch strings.Join(strings.Fields(command), " ") {
	case "history":
		*history = append(*history, "history")
		lines := make([]string, len(*history))
		for i, entry := range *history {
			lines[i] = fmt.Sprintf("%d  %s", i+1, entry)
		}
		return strings.Join(lines, "\n"), true
	case "history -c":
		*history = (*history)[:0]
		return "Historial de sesión limpiado", true
	default:
		return "", false
	}
}

func parseAllowedIP(value string) (func(net.IP) bool, error) {
	if ip := net.ParseIP(value); ip != nil {
		return func(remote net.IP) bool { return remote.Equal(ip) }, nil
	}
	_, network, err := net.ParseCIDR(value)
	if err != nil {
		return nil, fmt.Errorf("AllowedClientIP debe ser una IP o una red CIDR válida")
	}
	return network.Contains, nil
}

func remoteIP(address net.Addr) (net.IP, error) {
	tcpAddress, ok := address.(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("dirección remota no es TCP: %s", address)
	}
	return tcpAddress.IP, nil
}
