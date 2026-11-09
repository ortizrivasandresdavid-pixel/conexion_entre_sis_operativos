package sockets

import (
	"net"
	"testing"

	"operativos/src/login"
)

func TestAuthenticateServerAllowsRetryWithinLimit(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- authenticateServer(
			serverConn,
			newMessageReader(serverConn),
			map[string]string{"admin": login.HashPassword("linux")},
			2,
		)
	}()

	reader := newMessageReader(clientConn)
	for attempt, password := range []string{"wrong", "linux"} {
		challenge, err := reader.Read()
		if err != nil {
			t.Fatal(err)
		}
		if challenge.Type != "auth_challenge" {
			t.Fatalf("message type = %q, want auth_challenge", challenge.Type)
		}
		if err := writeMessage(clientConn, Message{
			Type:     "authenticate",
			Username: "admin",
			Proof:    login.MakeProof("admin", password, challenge.Nonce),
		}); err != nil {
			t.Fatal(err)
		}
		result, err := reader.Read()
		if err != nil {
			t.Fatal(err)
		}
		if result.Type != "auth_result" {
			t.Fatalf("message type = %q, want auth_result", result.Type)
		}
		if result.OK != (attempt == 1) {
			t.Fatalf("attempt %d: authenticated = %t", attempt+1, result.OK)
		}
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}
