package sockets

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"operativos/src/login"
)

// fakeSessionServer ejecuta el handshake real del servidor sobre una conexión
// y, una vez autenticada o registrada, responde al período y a los comandos.
func fakeSessionServer(t *testing.T, conn net.Conn, store *login.UserStore, limiter *login.AttemptLimiter) {
	t.Helper()
	defer conn.Close()

	reader := newMessageReader(conn)
	if _, err := authenticateServer(
		context.Background(), conn, reader, store, 3, limiter, "127.0.0.1",
	); err != nil {
		return
	}
	for {
		message, err := reader.Read()
		if err != nil {
			return
		}
		switch message.Type {
		case "set_period":
			if err := writeMessage(conn, Message{
				Type: "command_result",
				ID:   message.ID,
				OK:   true,
			}); err != nil {
				return
			}
		case "command":
			if err := writeMessage(conn, Message{
				Type:   "command_result",
				ID:     message.ID,
				OK:     true,
				Output: "ok:" + message.Command,
				Dir:    "/tmp",
			}); err != nil {
				return
			}
		}
	}
}

func startAccessTestServer(t *testing.T, users map[string]string) (string, *login.UserStore) {
	t.Helper()
	store := testUserStore(t, users)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	limiter := login.NewAttemptLimiter()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go fakeSessionServer(t, conn, store, limiter)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
	})
	return listener.Addr().String(), store
}

func waitForResult(t *testing.T, session *ClientSession, commandID uint64) Message {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case event, open := <-session.Events():
			if !open {
				t.Fatal("session closed before the command result")
			}
			if event.Err != nil {
				t.Fatalf("session event error: %v", event.Err)
			}
			if event.Message.Type == "command_result" && event.Message.ID == commandID {
				return event.Message
			}
		case <-timeout.C:
			t.Fatal("timed out waiting for the command result")
		}
	}
}

func TestConnectAccessLoginAndSession(t *testing.T) {
	address, _ := startAccessTestServer(t, map[string]string{"admin": login.HashPassword("linux")})
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()

	access, err := ConnectAccess(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	if access.Address() != address {
		t.Fatalf("access address = %q, want %q", access.Address(), address)
	}
	session, err := access.Login("admin", "linux", 5)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	id, err := session.SendCommand("ls")
	if err != nil {
		t.Fatal(err)
	}
	result := waitForResult(t, session, id)
	if result.Output != "ok:ls" {
		t.Fatalf("command output = %q, want ok:ls", result.Output)
	}
}

func TestAccessLoginRetriesAfterRejectedPassword(t *testing.T) {
	address, _ := startAccessTestServer(t, map[string]string{"admin": login.HashPassword("linux")})
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()

	access, err := ConnectAccess(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	_, err = access.Login("admin", "incorrecta", 5)
	var authError *AuthResultError
	if !errors.As(err, &authError) {
		t.Fatalf("login error = %v, want AuthResultError", err)
	}
	if authError.AttemptsRemaining != 2 || authError.RetryAfterSeconds < 1 {
		t.Fatalf("auth error = %#v, want 2 attempts and a retry delay", authError)
	}

	// El servidor envía el desafío nuevo tras la espera progresiva; el cliente
	// debe consumirlo y autenticarse en el mismo socket.
	session, err := access.Login("admin", "linux", 5)
	if err != nil {
		t.Fatal(err)
	}
	session.Close()
}

func TestConnectAccessRegisterPersistsAndLogsIn(t *testing.T) {
	address, store := startAccessTestServer(t, map[string]string{"admin": login.HashPassword("linux")})
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()

	access, err := ConnectAccess(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	const username = "operador"
	const password = "clave-segura"
	session, err := access.Register(username, password, password, 5)
	if err != nil {
		t.Fatal(err)
	}
	id, err := session.SendCommand("pwd")
	if err != nil {
		t.Fatal(err)
	}
	if result := waitForResult(t, session, id); result.Output != "ok:pwd" {
		t.Fatalf("command output = %q, want ok:pwd", result.Output)
	}
	session.Close()

	const nonce = "post-register"
	if !store.Verify(username, nonce, login.MakeProof(username, password, nonce)) {
		t.Fatal("registered user cannot authenticate on the server store")
	}

	// Una nueva conexión debe poder autenticarse con la cuenta creada.
	second, err := ConnectAccess(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	secondSession, err := second.Login(username, password, 5)
	if err != nil {
		t.Fatal(err)
	}
	secondSession.Close()
}

func TestAccessRegisterValidatesBeforeSending(t *testing.T) {
	address, _ := startAccessTestServer(t, map[string]string{"admin": login.HashPassword("linux")})
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()

	access, err := ConnectAccess(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	defer access.Close()

	cases := []struct {
		name         string
		username     string
		password     string
		confirmation string
	}{
		{"empty user", "", "clave", "clave"},
		{"colon in user", "ope:rador", "clave", "clave"},
		{"empty password", "operador", "", ""},
		{"mismatched confirmation", "operador", "clave", "otra"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := access.Register(testCase.username, testCase.password, testCase.confirmation, 5); err == nil {
				t.Fatal("Register accepted invalid input")
			}
		})
	}
}
