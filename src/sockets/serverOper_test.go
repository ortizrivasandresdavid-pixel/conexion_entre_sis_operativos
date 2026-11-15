package sockets

import (
	"context"
	"net"
	"testing"
	"time"

	"operativos/src/login"
)

func TestAuthenticateServerAllowsRetryWithinLimit(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- authenticateServer(
			context.Background(),
			serverConn,
			newMessageReader(serverConn),
			map[string]string{"admin": login.HashPassword("linux")},
			2,
			login.NewAttemptLimiter(),
			"127.0.0.1",
		)
	}()

	reader := newMessageReader(clientConn)
	var retryStartedAt time.Time
	for attempt, password := range []string{"wrong", "linux"} {
		challenge, err := reader.Read()
		if err != nil {
			t.Fatal(err)
		}
		if attempt == 1 && time.Since(retryStartedAt) < 900*time.Millisecond {
			t.Fatal("server allowed a retry before the backoff elapsed")
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
		wantRemaining := 1
		wantRetry := 1
		if attempt == 1 {
			wantRemaining = 2
			wantRetry = 0
		}
		if result.AttemptsRemaining != wantRemaining || result.RetryAfterSeconds != wantRetry {
			t.Fatalf("attempt %d: remaining = %d, retry = %d; want %d, %d",
				attempt+1, result.AttemptsRemaining, result.RetryAfterSeconds, wantRemaining, wantRetry)
		}
		if attempt == 0 {
			retryStartedAt = time.Now()
		}
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticateServerLocksAfterMaxAttempts(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- authenticateServer(
			context.Background(),
			serverConn,
			newMessageReader(serverConn),
			map[string]string{"admin": login.HashPassword("linux")},
			1,
			login.NewAttemptLimiter(),
			"127.0.0.1",
		)
	}()

	reader := newMessageReader(clientConn)
	challenge, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeMessage(clientConn, Message{
		Type:     "authenticate",
		Username: "admin",
		Proof:    login.MakeProof("admin", "wrong", challenge.Nonce),
	}); err != nil {
		t.Fatal(err)
	}
	result, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if result.Code != "AUTH_LOCKED" || result.AttemptsRemaining != 0 || result.RetryAfterSeconds <= 0 {
		t.Fatalf("lock result = %#v, want AUTH_LOCKED with no attempts and a retry delay", result)
	}
	if _, err := reader.Read(); err == nil {
		t.Fatal("server kept the connection open after exhausting attempts")
	}
	if err := <-serverDone; err == nil {
		t.Fatal("authentication succeeded after exhausting attempts")
	}
}

func TestSessionHistoryCommand(t *testing.T) {
	history := []string{"pwd", "ls -la"}
	output, handled := sessionHistoryCommand("history", &history)
	if !handled {
		t.Fatal("history was not handled by the session")
	}
	if want := "1  pwd\n2  ls -la\n3  history"; output != want {
		t.Fatalf("history output = %q, want %q", output, want)
	}

	output, handled = sessionHistoryCommand("history -c", &history)
	if !handled || output != "Historial de sesión limpiado" || len(history) != 0 {
		t.Fatalf("history -c = (%q, %t, %v), want cleared session history", output, handled, history)
	}
}
