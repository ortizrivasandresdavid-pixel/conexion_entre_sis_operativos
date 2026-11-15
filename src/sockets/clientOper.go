package sockets

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
	"operativos/src/login"
)

type ClientEvent struct {
	Message Message
	Err     error
}

type Client struct {
	conn      net.Conn
	cancel    context.CancelFunc
	events    chan ClientEvent
	writeMu   sync.Mutex
	nextID    uint64
	closeOnce sync.Once
}

func ConnectClient(ctx context.Context, address string, reportPeriodSeconds int, username, password string) (*Client, error) {
	if reportPeriodSeconds < 1 || reportPeriodSeconds > 3600 {
		return nil, fmt.Errorf("el período de reporte debe estar entre 1 y 3600 segundos")
	}
	if strings.TrimSpace(username) == "" || password == "" {
		return nil, fmt.Errorf("el usuario y la contraseña son obligatorios")
	}

	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("conectar con el servidor: %w", err)
	}
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()
	fail := func(err error) (*Client, error) {
		_ = conn.Close()
		return nil, err
	}

	reader := newMessageReader(conn)
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return fail(fmt.Errorf("configurar tiempo límite de autenticación: %w", err))
	}
	challenge, err := reader.Read()
	if err != nil {
		return fail(fmt.Errorf("recibir desafío de autenticación: %w", err))
	}
	if challenge.Type != "auth_challenge" || challenge.Nonce == "" {
		return fail(fmt.Errorf("respuesta de autenticación no válida"))
	}
	username = strings.TrimSpace(username)
	if err := writeMessage(conn, Message{
		Type:     "authenticate",
		Username: username,
		Proof:    login.MakeProof(username, password, challenge.Nonce),
	}); err != nil {
		return fail(err)
	}
	result, err := reader.Read()
	if err != nil {
		return fail(fmt.Errorf("recibir resultado de autenticación: %w", err))
	}
	if result.Type != "auth_result" {
		return fail(fmt.Errorf("resultado de autenticación no válido"))
	}
	if !result.OK {
		return fail(fmt.Errorf("autenticación fallida; intentos restantes: %d", result.AttemptsRemaining))
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return fail(fmt.Errorf("limpiar tiempo límite de conexión: %w", err))
	}

	clientCtx, cancel := context.WithCancel(ctx)
	client := &Client{
		conn:   conn,
		cancel: cancel,
		events: make(chan ClientEvent, 16),
		nextID: 2,
	}
	if err := writeMessage(conn, Message{Type: "set_period", ID: 1, PeriodSeconds: reportPeriodSeconds}); err != nil {
		cancel()
		return fail(err)
	}
	stopClientClose := context.AfterFunc(clientCtx, func() { _ = conn.Close() })
	go client.readEvents(clientCtx, reader, stopClientClose)
	return client, nil
}

func (c *Client) Events() <-chan ClientEvent {
	return c.events
}

func (c *Client) SendCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("el comando no puede estar vacío")
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if err := writeMessage(c.conn, Message{Type: "command", ID: c.nextID, Command: command}); err != nil {
		return err
	}
	c.nextID++
	return nil
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.cancel()
		_ = c.conn.Close()
	})
}

func (c *Client) readEvents(ctx context.Context, reader *messageReader, stopClose func() bool) {
	defer close(c.events)
	defer c.cancel()
	defer c.conn.Close()
	defer stopClose()

	for {
		message, err := reader.Read()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) && ctx.Err() == nil {
				select {
				case c.events <- ClientEvent{Err: fmt.Errorf("conexión cerrada por el servidor: %w", err)}:
				case <-ctx.Done():
				}
			}
			return
		}
		select {
		case c.events <- ClientEvent{Message: message}:
		case <-ctx.Done():
			return
		}
	}
}

func RunClient(ctx context.Context, address string, reportPeriodSeconds int) error {
	if reportPeriodSeconds < 1 || reportPeriodSeconds > 3600 {
		return fmt.Errorf("el período de reporte debe estar entre 1 y 3600 segundos")
	}
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("conectar con el servidor: %w", err)
	}
	defer conn.Close()

	reader := newMessageReader(conn)
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return fmt.Errorf("configurar tiempo límite de autenticación: %w", err)
	}
	console := bufio.NewReader(os.Stdin)
	if err := authenticateClient(reader, conn, console); err != nil {
		return err
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return fmt.Errorf("limpiar tiempo límite de conexión: %w", err)
	}
	fmt.Println("Autenticación correcta. Escribe help para ver los diagnósticos permitidos; bye para salir.")

	const periodRequestID = 1
	if err := writeMessage(conn, Message{Type: "set_period", ID: periodRequestID, PeriodSeconds: reportPeriodSeconds}); err != nil {
		return err
	}

	incoming := make(chan readEvent, 1)
	go func() {
		defer close(incoming)
		for {
			message, err := reader.Read()
			select {
			case incoming <- readEvent{message: message, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	input := make(chan string)
	inputErrors := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(console)
		for scanner.Scan() {
			select {
			case input <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		inputErrors <- scanner.Err()
		close(input)
	}()

	var nextID uint64 = periodRequestID + 1
	closing := false
	var closingID uint64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case command, open := <-input:
			if !open {
				if err := <-inputErrors; err != nil {
					return fmt.Errorf("leer consola: %w", err)
				}
				if !closing {
					if err := writeMessage(conn, Message{Type: "command", ID: nextID, Command: "bye"}); err != nil {
						return err
					}
					closing = true
					closingID = nextID
					nextID++
				}
				input = nil
				continue
			}
			if closing {
				continue
			}
			if err := writeMessage(conn, Message{Type: "command", ID: nextID, Command: strings.TrimSpace(command)}); err != nil {
				return err
			}
			if strings.TrimSpace(command) == "bye" {
				closing = true
				closingID = nextID
				input = nil
			}
			nextID++
		case event, open := <-incoming:
			if !open {
				return fmt.Errorf("el servidor cerró la conexión sin respuesta")
			}
			if event.err != nil {
				return fmt.Errorf("conexión cerrada por el servidor: %w", event.err)
			}
			switch event.message.Type {
			case "metric":
				printMetrics(event.message)
			case "command_result":
				if event.message.Output != "" {
					fmt.Print(event.message.Output)
					if !strings.HasSuffix(event.message.Output, "\n") {
						fmt.Println()
					}
				}
				if event.message.Error != "" {
					fmt.Fprintln(os.Stderr, "Error:", event.message.Error)
				}
				if event.message.ID == periodRequestID && event.message.OK {
					fmt.Printf("Recibirás métricas cada %d segundos.\n", reportPeriodSeconds)
				}
				if closing && event.message.ID == closingID {
					return nil
				}
			default:
				fmt.Fprintf(os.Stderr, "Mensaje del servidor no reconocido: %s\n", event.message.Type)
			}
		}
	}
}

func authenticateClient(reader *messageReader, conn net.Conn, input *bufio.Reader) error {
	for {
		challenge, err := reader.Read()
		if err != nil {
			return fmt.Errorf("recibir desafío de autenticación: %w", err)
		}
		if challenge.Type != "auth_challenge" || challenge.Nonce == "" {
			return fmt.Errorf("respuesta de autenticación no válida")
		}

		fmt.Print("Usuario: ")
		username, err := input.ReadString('\n')
		if err != nil {
			return fmt.Errorf("leer usuario: %w", err)
		}
		username = strings.TrimSpace(username)
		if username == "" {
			return fmt.Errorf("el usuario no puede estar vacío")
		}

		fmt.Print("Password: ")
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("leer password sin mostrarlo: %w", err)
		}
		request := Message{
			Type:     "authenticate",
			Username: username,
			Proof:    login.MakeProof(username, string(password), challenge.Nonce),
		}
		for i := range password {
			password[i] = 0
		}
		if err := writeMessage(conn, request); err != nil {
			return err
		}
		result, err := reader.Read()
		if err != nil {
			return fmt.Errorf("recibir resultado de autenticación: %w", err)
		}
		if result.Type != "auth_result" {
			return fmt.Errorf("resultado de autenticación no válido")
		}
		if result.OK {
			return nil
		}
		fmt.Fprintf(os.Stderr, "Autenticación fallida; intentos restantes: %d\n", result.AttemptsRemaining)
		if result.AttemptsRemaining == 0 {
			return fmt.Errorf("se agotaron los intentos de autenticación")
		}
	}
}

func printMetrics(message Message) {
	if message.Error != "" {
		fmt.Fprintln(os.Stderr, "No se pudieron leer las métricas:", message.Error)
		return
	}
	if message.Metrics == nil {
		fmt.Fprintln(os.Stderr, "El servidor envió un reporte de métricas vacío")
		return
	}
	report := message.Metrics
	fmt.Printf(
		"\n[Métricas %s] CPU %.1f%% | Procesos %d | RAM %d/%d MB | Disco %d/%d GB\n",
		report.Timestamp,
		report.CPUPercent,
		report.ActiveProcesses,
		report.MemoryUsedMB,
		report.MemoryTotalMB,
		report.DiskUsedGB,
		report.DiskTotalGB,
	)
}
