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
