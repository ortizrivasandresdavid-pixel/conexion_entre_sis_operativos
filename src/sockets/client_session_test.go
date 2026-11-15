package sockets

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"operativos/src/login"
	"operativos/src/metrics"
)

func TestClientSessionAuthenticatesSetsPeriodAndSendsCommand(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverResult := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverResult <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

		reader := newMessageReader(conn)
		if err := authenticateServer(
			context.Background(),
			conn,
			reader,
			map[string]string{"admin": login.HashPassword("secret")},
			3,
			login.NewAttemptLimiter(),
			"127.0.0.1",
		); err != nil {
			serverResult <- err
			return
		}

		period, err := reader.Read()
		if err != nil {
			serverResult <- err
			return
		}
		if period.Type != "set_period" || period.PeriodSeconds != 2 {
			serverResult <- fmt.Errorf("unexpected period request: %#v", period)
			return
		}
		if err := writeMessage(conn, Message{
			Type: "command_result",
			ID:   period.ID,
			OK:   true,
		}); err != nil {
			serverResult <- err
			return
		}
		if err := writeMessage(conn, Message{
			Type:    "metric",
			Metrics: &metrics.Snapshot{Timestamp: "test-time", CPUPercent: 12.5},
		}); err != nil {
			serverResult <- err
			return
		}

		command, err := reader.Read()
		if err != nil {
			serverResult <- err
			return
		}
		if command.Type != "command" || command.Command != "help" {
			serverResult <- fmt.Errorf("unexpected command request: %#v", command)
			return
		}
		if err := writeMessage(conn, Message{
			Type:   "command_result",
			ID:     command.ID,
			OK:     true,
			Output: "salida del comando",
		}); err != nil {
			serverResult <- err
			return
		}
		serverResult <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	session, err := ConnectClient(ctx, listener.Addr().String(), 2, "admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	commandID, err := session.SendCommand("help")
	if err != nil {
		t.Fatal(err)
	}
	receivedMetric := false
	receivedResult := false
	for !receivedResult {
		select {
		case event := <-session.Events():
			if event.Err != nil {
				t.Fatalf("session event error: %v", event.Err)
			}
			switch event.Message.Type {
			case "metric":
				if event.Message.Metrics == nil || event.Message.Metrics.CPUPercent != 12.5 {
					t.Fatalf("unexpected metrics event: %#v", event.Message)
				}
				receivedMetric = true
			case "command_result":
				if event.Message.ID != commandID {
					t.Fatalf("command result ID = %d, want %d", event.Message.ID, commandID)
				}
				if event.Message.Output != "salida del comando" {
					t.Errorf("output = %q", event.Message.Output)
				}
				receivedResult = true
			default:
				t.Fatalf("unexpected session event: %#v", event.Message)
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for client events")
		}
	}
	if !receivedMetric {
		t.Fatal("did not receive the metrics event")
	}

	select {
	case err := <-serverResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for server")
	}
}
