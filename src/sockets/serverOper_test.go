package sockets

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"operativos/src/login"
)

type authenticationOutcome struct {
	username string
	err      error
}

func testUserStore(t *testing.T, users map[string]string) *login.UserStore {
	t.Helper()
	lines := make([]string, 0, len(users))
	for username, hash := range users {
		lines = append(lines, username+":"+hash)
	}
	path := filepath.Join(t.TempDir(), "users.conf")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := login.OpenUserStoreWithProvisioner(path, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestAuthenticateServerAllowsRetryWithinLimit(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	store := testUserStore(t, map[string]string{"admin": login.HashPassword("linux")})

	serverDone := make(chan authenticationOutcome, 1)
	go func() {
		username, err := authenticateServer(
			context.Background(),
			serverConn,
			newMessageReader(serverConn),
			store,
			2,
			login.NewAttemptLimiter(),
			"127.0.0.1",
		)
		serverDone <- authenticationOutcome{username: username, err: err}
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
	outcome := <-serverDone
	if outcome.err != nil {
		t.Fatal(outcome.err)
	}
	if outcome.username != "admin" {
		t.Fatalf("authenticated username = %q, want admin", outcome.username)
	}
}

func TestAuthenticateServerLocksAfterMaxAttempts(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	store := testUserStore(t, map[string]string{"admin": login.HashPassword("linux")})

	serverDone := make(chan error, 1)
	go func() {
		_, err := authenticateServer(
			context.Background(),
			serverConn,
			newMessageReader(serverConn),
			store,
			1,
			login.NewAttemptLimiter(),
			"127.0.0.1",
		)
		serverDone <- err
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

func TestAuthenticateServerRegistersUserAndPersistsIt(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	store := testUserStore(t, map[string]string{"admin": login.HashPassword("linux")})
	serverDone := make(chan authenticationOutcome, 1)
	go func() {
		username, err := authenticateServer(
			context.Background(),
			serverConn,
			newMessageReader(serverConn),
			store,
			3,
			login.NewAttemptLimiter(),
			"127.0.0.1",
		)
		serverDone <- authenticationOutcome{username: username, err: err}
	}()

	reader := newMessageReader(clientConn)
	challenge, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	const username = "operador"
	const password = "clave-segura"
	if err := writeMessage(clientConn, Message{
		Type:         "register",
		Username:     username,
		PasswordHash: login.HashPassword(password),
		Proof:        login.MakeProof(username, password, challenge.Nonce),
	}); err != nil {
		t.Fatal(err)
	}
	result, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != "register_result" || !result.OK {
		t.Fatalf("register result = %#v, want successful register_result", result)
	}
	outcome := <-serverDone
	if outcome.err != nil {
		t.Fatal(outcome.err)
	}
	if outcome.username != username {
		t.Fatalf("registered username = %q, want %q", outcome.username, username)
	}

	const nonce = "verification-nonce"
	if !store.Verify(username, nonce, login.MakeProof(username, password, nonce)) {
		t.Fatal("registered user cannot authenticate")
	}
	reopened, err := login.OpenUserStore(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Verify(username, nonce, login.MakeProof(username, password, nonce)) {
		t.Fatal("registered user was not persisted to the users file")
	}
}

func TestAuthenticateServerRejectsDuplicateRegistration(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	store := testUserStore(t, map[string]string{"admin": login.HashPassword("linux")})
	serverDone := make(chan error, 1)
	go func() {
		_, err := authenticateServer(
			context.Background(),
			serverConn,
			newMessageReader(serverConn),
			store,
			3,
			login.NewAttemptLimiter(),
			"127.0.0.1",
		)
		serverDone <- err
	}()

	reader := newMessageReader(clientConn)
	challenge, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	const username = "admin"
	const password = "otra-clave"
	if err := writeMessage(clientConn, Message{
		Type:         "register",
		Username:     username,
		PasswordHash: login.HashPassword(password),
		Proof:        login.MakeProof(username, password, challenge.Nonce),
	}); err != nil {
		t.Fatal(err)
	}
	result, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != "register_result" || result.OK || result.Code != "USER_EXISTS" {
		t.Fatalf("register result = %#v, want USER_EXISTS rejection", result)
	}

	next, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if next.Type != "auth_challenge" {
		t.Fatalf("message after rejected register = %q, want auth_challenge", next.Type)
	}
	if err := serverConn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err == nil {
		t.Fatal("server accepted a duplicate registration")
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
