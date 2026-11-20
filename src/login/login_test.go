package login

import (
	"errors"
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

func TestLoadUsersRejectsInvalidLinuxUsername(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.conf")
	content := "Admin:" + HashPassword("linux") + "\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadUsers(path); err == nil {
		t.Fatal("invalid Linux username was accepted")
	}
}

func TestUserStoreRegisterPersistsAndAuthenticates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.conf")
	// Sin salto de línea final: el registro debe insertar el separador.
	content := "admin:" + HashPassword("linux")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	provisionCalls := 0
	store, err := OpenUserStoreWithProvisioner(path, func(username string) error {
		provisionCalls++
		if username != "operador" {
			t.Fatalf("provisioned username = %q, want operador", username)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	const username = "operador"
	const password = "clave-segura"
	if err := store.Register(username, HashPassword(password)); err != nil {
		t.Fatal(err)
	}
	if provisionCalls != 1 {
		t.Fatalf("provision calls = %d, want 1", provisionCalls)
	}
	const nonce = "challenge"
	if !store.Verify(username, nonce, MakeProof(username, password, nonce)) {
		t.Fatal("registered user cannot authenticate")
	}

	reopened, err := OpenUserStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Verify(username, nonce, MakeProof(username, password, nonce)) {
		t.Fatal("registered user was not written to the users file")
	}
	if !reopened.Verify("admin", nonce, MakeProof("admin", "linux", nonce)) {
		t.Fatal("registration broke the existing users in the file")
	}

	if err := store.Register(username, HashPassword("otra")); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate register error = %v, want ErrUserExists", err)
	}
	if provisionCalls != 1 {
		t.Fatalf("duplicate registration provision calls = %d, want 1", provisionCalls)
	}
}

func TestUserStoreDoesNotPersistWhenSystemProvisioningFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.conf")
	if err := os.WriteFile(path, []byte("admin:"+HashPassword("linux")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	provisionErr := errors.New("useradd failed")
	store, err := OpenUserStoreWithProvisioner(path, func(string) error {
		return provisionErr
	})
	if err != nil {
		t.Fatal(err)
	}

	err = store.Register("operador", HashPassword("clave"))
	if !errors.Is(err, provisionErr) {
		t.Fatalf("Register error = %v, want provisioning error", err)
	}
	if store.Verify("operador", "nonce", MakeProof("operador", "clave", "nonce")) {
		t.Fatal("user was registered in memory after provisioning failed")
	}
	users, err := LoadUsers(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := users["operador"]; exists {
		t.Fatal("user was persisted after provisioning failed")
	}
}

func TestUserStoreRejectsInvalidRegistration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.conf")
	if err := os.WriteFile(path, []byte("admin:"+HashPassword("linux")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenUserStore(path)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		username string
		hash     string
		want     error
	}{
		{"empty username", "", HashPassword("x"), ErrInvalidUsername},
		{"colon in username", "ope:rador", HashPassword("x"), ErrInvalidUsername},
		{"space in username", "ope rador", HashPassword("x"), ErrInvalidUsername},
		{"uppercase username", "Operador", HashPassword("x"), ErrInvalidUsername},
		{"leading hyphen", "-operador", HashPassword("x"), ErrInvalidUsername},
		{"username over 32 characters", "operador1234567890123456789012345", HashPassword("x"), ErrInvalidUsername},
		{"short hash", "operador", "abc", ErrInvalidPasswordHash},
		{"non hexadecimal hash", "operador", "zz" + HashPassword("x")[2:], ErrInvalidPasswordHash},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := store.Register(testCase.username, testCase.hash)
			if !errors.Is(err, testCase.want) {
				t.Fatalf("Register(%q) error = %v, want %v", testCase.username, err, testCase.want)
			}
		})
	}
}

func TestUserStoreEnsureSystemUserUsesProvisioner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "users.conf")
	if err := os.WriteFile(path, []byte("admin:"+HashPassword("linux")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	provisioned := ""
	store, err := OpenUserStoreWithProvisioner(path, func(username string) error {
		provisioned = username
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureSystemUser("admin"); err != nil {
		t.Fatal(err)
	}
	if provisioned != "admin" {
		t.Fatalf("provisioned username = %q, want admin", provisioned)
	}
}
