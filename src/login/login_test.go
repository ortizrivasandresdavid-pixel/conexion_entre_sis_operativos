package login

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPasswordProof(t *testing.T) {
	const username = "admin"
	const password = "linux"
	const nonce = "one-time-challenge"
	users := map[string]string{username: HashPassword(password)}

	proof := MakeProof(username, password, nonce)
	if !VerifyProof(users, username, nonce, proof) {
		t.Fatal("valid proof was rejected")
	}
	if VerifyProof(users, username, "another-challenge", proof) {
		t.Fatal("proof was accepted for a different challenge")
	}
	if VerifyProof(users, "unknown", nonce, proof) {
		t.Fatal("proof was accepted for an unknown user")
	}
}

func TestLoadUsers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.conf")
	content := "# laboratorio\nadmin:" + HashPassword("linux") + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	users, err := LoadUsers(path)
	if err != nil {
		t.Fatal(err)
	}
	if users["admin"] != HashPassword("linux") {
		t.Fatalf("unexpected hash loaded: %q", users["admin"])
	}
}

func TestLoadUsersRejectsDuplicate(t *testing.T) {
	hash := HashPassword("linux")
	path := filepath.Join(t.TempDir(), "users.conf")
	content := "admin:" + hash + "\nadmin:" + hash + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadUsers(path); err == nil {
		t.Fatal("duplicate username was accepted")
	}
}
