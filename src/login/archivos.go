package login

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

var usernamePattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

var (
	ErrUserExists          = errors.New("el usuario ya existe")
	ErrInvalidUsername     = errors.New("nombre de usuario no válido")
	ErrInvalidPasswordHash = errors.New("hash SHA-256 no válido")
)

type UserStore struct {
	mu                    sync.RWMutex
	path                  string
	users                 map[string]string
	systemUserProvisioner func(string) error
}

func OpenUserStore(path string) (*UserStore, error) {
	return OpenUserStoreWithProvisioner(path, EnsureSystemUser)
}

func OpenUserStoreWithProvisioner(path string, provisioner func(string) error) (*UserStore, error) {
	if provisioner == nil {
		return nil, errors.New("el provisionador de usuarios del sistema no puede ser nil")
	}
	users, err := LoadUsers(path)
	if err != nil {
		return nil, err
	}
	return &UserStore{path: path, users: users, systemUserProvisioner: provisioner}, nil
}

func (s *UserStore) Path() string {
	return s.path
}

func (s *UserStore) Verify(username, nonce, proof string) bool {
	s.mu.RLock()
	storedHash, ok := s.users[username]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	return VerifyProofHash(storedHash, username, nonce, proof)
}

func (s *UserStore) Register(username, passwordHash string) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	hash, err := NormalizePasswordHash(passwordHash)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.users[username]; exists {
		return ErrUserExists
	}
	if err := s.systemUserProvisioner(username); err != nil {
		return fmt.Errorf("crear la cuenta Linux para %q: %w", username, err)
	}
	if err := appendUser(s.path, username, hash); err != nil {
		return err
	}
	s.users[username] = hash
	return nil
}

func ValidateUsername(username string) error {
	if !usernamePattern.MatchString(username) {
		return fmt.Errorf("%w: use de 1 a 32 caracteres, comenzando con una letra minúscula o '_' y continuando con minúsculas, dígitos, '_' o '-'", ErrInvalidUsername)
	}
	return nil
}

func (s *UserStore) EnsureSystemUser(username string) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	if err := s.systemUserProvisioner(username); err != nil {
		return fmt.Errorf("asegurar la cuenta Linux para %q: %w", username, err)
	}
	return nil
}

func NormalizePasswordHash(passwordHash string) (string, error) {
	hash := strings.ToLower(strings.TrimSpace(passwordHash))
	if len(hash) != 64 {
		return "", fmt.Errorf("%w: debe tener 64 caracteres hexadecimales", ErrInvalidPasswordHash)
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return "", fmt.Errorf("%w: contiene caracteres que no son hexadecimales", ErrInvalidPasswordHash)
	}
	return hash, nil
}

func appendUser(path, username, passwordHash string) error {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0o640)
	if err != nil {
		return fmt.Errorf("abrir %s para registrar el usuario: %w", path, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("comprobar %s: %w", path, err)
	}
	prefix := ""
	if info.Size() > 0 {
		lastByte := make([]byte, 1)
		if _, err := file.ReadAt(lastByte, info.Size()-1); err != nil {
			return fmt.Errorf("leer el final de %s: %w", path, err)
		}
		if lastByte[0] != '\n' {
			prefix = "\n"
		}
	}
	if _, err := file.WriteString(prefix + username + ":" + passwordHash + "\n"); err != nil {
		return fmt.Errorf("escribir el usuario en %s: %w", path, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sincronizar %s: %w", path, err)
	}
	return nil
}

func LoadUsers(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abrir archivo de usuarios: %w", err)
	}
	defer file.Close()

	users := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 64*1024)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		username, passwordHash, ok := strings.Cut(line, ":")
		if !ok || ValidateUsername(username) != nil || len(passwordHash) != 64 {
			return nil, fmt.Errorf("formato inválido en %s:%d", path, lineNumber)
		}
		if _, err := hex.DecodeString(passwordHash); err != nil {
			return nil, fmt.Errorf("hash SHA-256 inválido en %s:%d", path, lineNumber)
		}
		if _, exists := users[username]; exists {
			return nil, fmt.Errorf("usuario duplicado %q en %s:%d", username, path, lineNumber)
		}
		users[username] = strings.ToLower(passwordHash)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("leer archivo de usuarios: %w", err)
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("el archivo de usuarios %s no contiene usuarios", path)
	}
	return users, nil
}
