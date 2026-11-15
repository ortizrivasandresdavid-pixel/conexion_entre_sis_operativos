package sockets

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
	"operativos/src/login"
)

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
	fmt.Println("Autenticación correcta. Escribe cualquier comando de Linux; 'bye' para salir.")

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
				if event.message.ExitCode != 0 {
					fmt.Fprintf(os.Stderr, "[código de salida: %d]\n", event.message.ExitCode)
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
		if challenge.Type == "auth_result" && !challenge.OK {
			return authResultError(challenge)
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
		if result.AttemptsRemaining == 0 {
			return authResultError(result)
		}
		fmt.Fprintln(os.Stderr, authResultError(result))
		if result.RetryAfterSeconds > 0 {
			time.Sleep(time.Duration(result.RetryAfterSeconds) * time.Second)
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
