package sockets

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"operativos/src/login"
	"operativos/src/metrics"
	"operativos/src/shell"
)

const (
	serverAuthTimeout     = 5 * time.Minute
	serverAuthResultGrace = 30 * time.Second
)

type ServerConfig struct {
	ListenIP                   string
	Port                       int
	AllowedClientIP            string
	MaxAuthAttempts            int
	UsersFile                  string
	DefaultReportPeriodSeconds int
	MaxReportPeriodSeconds     int
	LogFile                    string
	LogMaxSizeMB               int
	LogBackups                 int
	EventFile                  string
}

func RunServer(ctx context.Context, config ServerConfig) error {
	if err := validateServerConfig(config); err != nil {
		return err
	}
	if err := shell.CheckAvailable(); err != nil {
		return fmt.Errorf("shell remoto no disponible: %w", err)
	}
	accounts, err := login.OpenUserStore(config.UsersFile)
	if err != nil {
		return err
	}
	attemptLimiter := login.NewAttemptLimiter()

	listener, err := net.Listen("tcp", net.JoinHostPort(config.ListenIP, strconv.Itoa(config.Port)))
	if err != nil {
		return fmt.Errorf("abrir socket servidor: %w", err)
	}
	events, eventErr := openGUIEventLog(config.EventFile)
	if eventErr != nil {
		slog.Warn("monitor gráfico sin flujo de eventos; las sesiones siguen activas",
			"archivo", config.EventFile,
			"error", eventErr,
		)
	} else {
		defer events.Close()
		slog.Info("flujo de eventos para la GUI", "archivo", config.EventFile)
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
	defer slog.Info("servidor detenido")
	defer func() {
		stopClose()
		_ = listener.Close()
		activeConnections.Range(func(key, _ any) bool {
			_ = key.(net.Conn).Close()
			return true
		})
		handlers.Wait()
	}()

	slog.Info("servidor escuchando",
		"direccion", listener.Addr().String(),
		"ip_permitida", config.AllowedClientIP,
	)
	slots := make(chan struct{}, 32)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Error("error al aceptar conexión", "error", err)
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
				if err := serveConnection(ctx, conn, config, accounts, attemptLimiter, events); err != nil {
					slog.Warn("conexión finalizada con error",
						"remota", conn.RemoteAddr().String(),
						"error", err,
					)
				}
			}()
		default:
			_ = conn.Close()
			slog.Warn("conexión rechazada: límite de conexiones alcanzado",
				"remota", conn.RemoteAddr().String(),
			)
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
	accounts *login.UserStore,
	attemptLimiter *login.AttemptLimiter,
	events *guiEventLog,
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
		slog.Warn("conexión rechazada: IP no autorizada", "ip", remoteIP.String())
		return fmt.Errorf("IP no autorizada: %s", remoteIP)
	}
	clientIP := remoteIP.String()
	slog.Info("conexión entrante", "ip", clientIP)
	defer slog.Info("conexión cerrada", "ip", clientIP)

	events.session("connect", clientIP, "", false)
	var username string
	authenticated := false
	defer func() {
		events.session("disconnect", clientIP, username, authenticated)
	}()

	reader := newMessageReader(conn)
	if err := conn.SetDeadline(time.Now().Add(serverAuthTimeout)); err != nil {
		return fmt.Errorf("configurar tiempo límite de autenticación: %w", err)
	}
	username, err = authenticateServer(ctx, conn, reader, accounts, config.MaxAuthAttempts, attemptLimiter, clientIP)
	if err != nil {
		return err
	}
	authenticated = true
	events.session("login", clientIP, username, true)
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return fmt.Errorf("limpiar tiempo límite de conexión: %w", err)
	}
	terminal, err := shell.NewSession(username)
	if err != nil {
		return fmt.Errorf("preparar shell remoto: %w", err)
	}
	slog.Info("sesión de shell iniciada", "ip", clientIP, "usuario", username, "directorio", terminal.Dir())
	return runSession(ctx, conn, reader, config, terminal, clientIP, username, events)
}

func authenticateServer(
	ctx context.Context,
	conn net.Conn,
	reader *messageReader,
	accounts *login.UserStore,
	maxAttempts int,
	attemptLimiter *login.AttemptLimiter,
	clientIP string,
) (string, error) {
	for {
		remaining, retryAfter := attemptLimiter.Check(clientIP, maxAttempts)
		if retryAfter > 0 {
			code := "AUTH_BACKOFF"
			message := fmt.Sprintf("autenticación bloqueada temporalmente; intente de nuevo en %d segundos", durationSeconds(retryAfter))
			if remaining == 0 {
				code = "AUTH_LOCKED"
				message = "se agotaron los intentos de autenticación"
			}
			slog.Warn("autenticación bloqueada",
				"ip", clientIP,
				"codigo", code,
				"intentos_restantes", remaining,
				"reintentar_en_s", durationSeconds(retryAfter),
			)
			if err := sendAuthResult(conn, Message{
				Code:              code,
				Error:             message,
				AttemptsRemaining: remaining,
				RetryAfterSeconds: durationSeconds(retryAfter),
			}); err != nil {
				return "", err
			}
			if remaining == 0 {
				if err := conn.Close(); err != nil {
					return "", fmt.Errorf("cerrar conexión bloqueada: %w", err)
				}
			}
			return "", fmt.Errorf("autenticación bloqueada temporalmente para %s", clientIP)
		}
		if err := conn.SetDeadline(time.Now().Add(serverAuthTimeout)); err != nil {
			return "", fmt.Errorf("configurar tiempo límite de autenticación: %w", err)
		}

		nonceBytes := make([]byte, 32)
		if _, err := rand.Read(nonceBytes); err != nil {
			return "", fmt.Errorf("generar desafío de autenticación: %w", err)
		}
		nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
		if err := writeMessage(conn, Message{Type: "auth_challenge", Nonce: nonce}); err != nil {
			return "", err
		}

		request, err := reader.Read()
		if err != nil {
			return "", err
		}
		if request.Type == "register" {
			registered, err := handleRegistration(conn, accounts, request, nonce, clientIP, attemptLimiter)
			if err != nil {
				return "", err
			}
			if registered {
				return request.Username, nil
			}
			continue
		}
		if request.Type != "authenticate" || request.Username == "" {
			slog.Warn("solicitud de autenticación inválida",
				"ip", clientIP,
				"tipo", request.Type,
			)
			return "", fmt.Errorf("solicitud de autenticación inválida")
		}

		remaining, retryAfter = attemptLimiter.Check(clientIP, maxAttempts)
		if retryAfter > 0 {
			code := "AUTH_BACKOFF"
			message := fmt.Sprintf("autenticación bloqueada temporalmente; intente de nuevo en %d segundos", durationSeconds(retryAfter))
			if remaining == 0 {
				code = "AUTH_LOCKED"
				message = "se agotaron los intentos de autenticación"
			}
			slog.Warn("autenticación bloqueada",
				"ip", clientIP,
				"usuario", request.Username,
				"codigo", code,
				"intentos_restantes", remaining,
				"reintentar_en_s", durationSeconds(retryAfter),
			)
			if err := sendAuthResult(conn, Message{
				Code:              code,
				Error:             message,
				AttemptsRemaining: remaining,
				RetryAfterSeconds: durationSeconds(retryAfter),
			}); err != nil {
				return "", err
			}
			if remaining == 0 {
				if err := conn.Close(); err != nil {
					return "", fmt.Errorf("cerrar conexión bloqueada: %w", err)
				}
				return "", fmt.Errorf("se agotaron los intentos de autenticación para %s", clientIP)
			}
			if err := waitForAuthRetry(ctx, retryAfter); err != nil {
				return "", err
			}
			continue
		}

		ok := accounts.Verify(request.Username, nonce, request.Proof)
		if ok {
			if err := accounts.EnsureSystemUser(request.Username); err != nil {
				slog.Error("no se pudo asegurar la cuenta Linux", "ip", clientIP, "usuario", request.Username, "error", err)
				if sendErr := sendAuthResult(conn, Message{
					Code:              "SYSTEM_USER_FAILED",
					Error:             fmt.Sprintf("no se pudo preparar la cuenta del sistema: %v", err),
					AttemptsRemaining: maxAttempts,
				}); sendErr != nil {
					return "", sendErr
				}
				return "", err
			}
			attemptLimiter.RecordSuccess(clientIP)
			slog.Info("autenticación correcta",
				"ip", clientIP,
				"usuario", request.Username,
			)
			if err := sendAuthResult(conn, Message{OK: true, AttemptsRemaining: maxAttempts}); err != nil {
				return "", err
			}
			return request.Username, nil
		}

		remaining, retryAfter = attemptLimiter.RecordFailure(clientIP, maxAttempts)
		if err := conn.SetDeadline(time.Now().Add(retryAfter + serverAuthResultGrace)); err != nil {
			return "", fmt.Errorf("configurar espera de autenticación: %w", err)
		}
		code := "AUTH_BACKOFF"
		message := fmt.Sprintf("credenciales incorrectas; intente de nuevo en %d segundos", durationSeconds(retryAfter))
		if remaining == 0 {
			code = "AUTH_LOCKED"
			message = "se agotaron los intentos de autenticación"
		}
		slog.Warn("autenticación fallida",
			"ip", clientIP,
			"usuario", request.Username,
			"codigo", code,
			"intentos_restantes", remaining,
			"reintentar_en_s", durationSeconds(retryAfter),
		)
		if err := sendAuthResult(conn, Message{
			Code:              code,
			Error:             message,
			AttemptsRemaining: remaining,
			RetryAfterSeconds: durationSeconds(retryAfter),
		}); err != nil {
			return "", err
		}
		if remaining == 0 {
			if err := conn.Close(); err != nil {
				return "", fmt.Errorf("cerrar conexión bloqueada: %w", err)
			}
			return "", fmt.Errorf("se agotaron los intentos de autenticación para %s", clientIP)
		}

		if err := waitForAuthRetry(ctx, retryAfter); err != nil {
			return "", err
		}
	}
}

func handleRegistration(
	conn net.Conn,
	accounts *login.UserStore,
	request Message,
	nonce string,
	clientIP string,
	attemptLimiter *login.AttemptLimiter,
) (bool, error) {
	username := request.Username
	if err := login.ValidateUsername(username); err != nil {
		return false, sendRegistrationError(conn, "INVALID_USERNAME", username, err, clientIP)
	}
	hash, err := login.NormalizePasswordHash(request.PasswordHash)
	if err != nil {
		return false, sendRegistrationError(conn, "INVALID_HASH", username, err, clientIP)
	}
	if !login.VerifyProofHash(hash, username, nonce, request.Proof) {
		return false, sendRegistrationError(
			conn, "INVALID_PROOF", username,
			fmt.Errorf("la prueba de registro no es válida"), clientIP,
		)
	}
	if err := accounts.Register(username, hash); err != nil {
		code := "REGISTER_FAILED"
		if errors.Is(err, login.ErrUserExists) {
			code = "USER_EXISTS"
		}
		return false, sendRegistrationError(conn, code, username, err, clientIP)
	}

	attemptLimiter.RecordSuccess(clientIP)
	slog.Info("usuario registrado", "ip", clientIP, "usuario", username)
	if err := writeMessage(conn, Message{Type: "register_result", OK: true, Username: username}); err != nil {
		return false, err
	}
	return true, nil
}

func sendRegistrationError(conn net.Conn, code, username string, cause error, clientIP string) error {
	slog.Warn("registro rechazado",
		"ip", clientIP,
		"usuario", username,
		"codigo", code,
		"error", cause,
	)
	return writeMessage(conn, Message{Type: "register_result", Code: code, Error: cause.Error()})
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
	command  string
	duration time.Duration
	err      error
}

func runSession(
	ctx context.Context,
	conn net.Conn,
	reader *messageReader,
	config ServerConfig,
	terminal *shell.Session,
	clientIP string,
	username string,
	events *guiEventLog,
) error {
	sessionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer slog.Info("sesión finalizada", "ip", clientIP)

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
				slog.Info("período de reporte cambiado", "ip", clientIP, "segundos", seconds)
				if err := writeMessage(conn, Message{Type: "command_result", ID: event.message.ID, OK: true, Output: fmt.Sprintf("Período de reporte: %d segundos", seconds)}); err != nil {
					return err
				}
			case "command":
				command := strings.TrimSpace(event.message.Command)
				events.commandReceived(clientIP, username, event.message.ID, command)
				if commandRunning {
					if err := writeMessage(conn, Message{Type: "command_result", ID: event.message.ID, Error: "espera a que termine el comando actual"}); err != nil {
						return err
					}
					continue
				}
				slog.Info("comando recibido", "ip", clientIP, "comando", shortCommand(command))
				if output, handled := sessionHistoryCommand(command, &sessionHistory); handled {
					if err := writeMessage(conn, Message{Type: "command_result", ID: event.message.ID, OK: true, Output: output}); err != nil {
						return err
					}
					continue
				}
				if command == "bye" {
					sessionHistory = append(sessionHistory, command)
					slog.Info("cierre de sesión solicitado", "ip", clientIP)
					if err := writeMessage(conn, Message{Type: "command_result", ID: event.message.ID, OK: true, Output: "Sesión cerrada"}); err != nil {
						return err
					}
					return nil
				}
				sessionHistory = append(sessionHistory, command)
				commandRunning = true
				started := time.Now()
				go func(id uint64, command string) {
					output, err := terminal.Execute(sessionCtx, command)
					select {
					case results <- commandResult{
						id:       id,
						output:   output.Output,
						exitCode: output.ExitCode,
						dir:      output.Dir,
						command:  command,
						duration: time.Since(started),
						err:      err,
					}:
					case <-sessionCtx.Done():
					}
				}(event.message.ID, event.message.Command)
			default:
				slog.Warn("tipo de solicitud no reconocido", "ip", clientIP, "tipo", event.message.Type)
				if err := writeMessage(conn, Message{Type: "command_result", ID: event.message.ID, Error: "tipo de solicitud no reconocido"}); err != nil {
					return err
				}
			}
		case result := <-results:
			commandRunning = false
			attributes := []any{
				"ip", clientIP,
				"comando", shortCommand(result.command),
				"codigo_salida", result.exitCode,
				"duracion_ms", result.duration.Milliseconds(),
			}
			if result.err != nil {
				attributes = append(attributes, "error", result.err)
			}
			slog.Info("comando finalizado", attributes...)
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
				writeErr := writeMessage(conn, Message{Type: "metric", Error: err.Error()})
				events.reportSent(clientIP, username, "error", err)
				if writeErr != nil {
					return writeErr
				}
				continue
			}
			if err := writeMessage(conn, Message{Type: "metric", Metrics: &snapshot}); err != nil {
				events.reportSent(clientIP, username, "error", err)
				return err
			}
			events.reportSent(clientIP, username, "enviado", nil)
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

func shortCommand(command string) string {
	const maxBytes = 1024
	if len(command) <= maxBytes {
		return command
	}
	return command[:maxBytes] + "...[comando truncado en el log]"
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
