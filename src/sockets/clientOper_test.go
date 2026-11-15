package sockets

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"operativos/src/login"
)

func TestConnectClientExchangesCommandsAndEvents(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()

		const username, password, nonce = "admin", "linux", "challenge"
		if err := writeMessage(conn, Message{Type: "auth_challenge", Nonce: nonce}); err != nil {
			serverDone <- err
			return
		}
		reader := newMessageReader(conn)
		request, err := reader.Read()
		if err != nil {
			serverDone <- err
			return
		}
		if request.Type != "authenticate" || !login.VerifyProof(
			map[string]string{username: login.HashPassword(password)},
			request.Username,
			nonce,
			request.Proof,
		) {
			serverDone <- fmt.Errorf("invalid authentication request: %#v", request)
			return
		}
		if err := writeMessage(conn, Message{Type: "auth_result", OK: true}); err != nil {
			serverDone <- err
			return
		}
		period, err := reader.Read()
		if err != nil {
			serverDone <- err
			return
		}
		if period.Type != "set_period" || period.ID != 1 || period.PeriodSeconds != 5 {
			serverDone <- fmt.Errorf("unexpected period request: %#v", period)
			return
		}
		if err := writeMessage(conn, Message{Type: "command_result", ID: period.ID, OK: true}); err != nil {
			serverDone <- err
			return
		}
		command, err := reader.Read()
		if err != nil {
			serverDone <- err
			return
		}
		if command.Type != "command" || command.ID != 2 || command.Command != "whoami" {
			serverDone <- fmt.Errorf("unexpected command request: %#v", command)
			return
		}
		if err := writeMessage(conn, Message{Type: "command_result", ID: command.ID, OK: true, Output: "admin"}); err != nil {
			serverDone <- err
			return
		}
		serverDone <- nil
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := ConnectClient(ctx, listener.Addr().String(), 5, "admin", "linux")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.SendCommand("whoami"); err != nil {
		t.Fatal(err)
	}
	receivedPeriod, receivedCommand := false, false
	for !receivedCommand {
		select {
		case event, open := <-client.Events():
			if !open {
				t.Fatal("client event stream closed before command response")
			}
			if event.Err != nil {
				t.Fatal(event.Err)
			}
			if event.Message.Type == "command_result" && event.Message.ID == 1 {
				receivedPeriod = true
			}
			if event.Message.Type == "command_result" && event.Message.Output == "admin" {
				receivedCommand = true
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for server events")
		}
	}
	if !receivedPeriod {
		t.Fatal("client did not receive the report-period acknowledgement")
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}
