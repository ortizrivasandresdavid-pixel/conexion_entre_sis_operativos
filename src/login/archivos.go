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
	// ErrUserExists indica que el nombre de usuario ya está registrado.
	ErrUserExists = errors.New("el usuario ya existe")
	// ErrInvalidUsername indica que el nombre no puede persistirse de forma segura.
	ErrInvalidUsername = errors.New("nombre de usuario no válido")
	// ErrInvalidPasswordHash indica que el hash recibido no tiene forma SHA-256 hexadecimal.
	ErrInvalidPasswordHash = errors.New("hash SHA-256 no válido")
)

// UserStore mantiene en memoria los usuarios cargados y serializa las
// escrituras sobre el archivo plano. Un único store se comparte entre todas las
// conexiones del servidor, por lo que el mutex protege tanto el mapa como el
// archivo de usuarios.
type UserStore struct {
	mu                    sync.RWMutex
	path                  string
	users                 map[string]string
	systemUserProvisioner func(string) error
}

// OpenUserStore carga el archivo de usuarios y devuelve un store listo para
// verificar credenciales, registrar cuentas nuevas y provisionar sus cuentas
// de sistema Linux.
func OpenUserStore(path string) (*UserStore, error) {
	return OpenUserStoreWithProvisioner(path, EnsureSystemUser)
}

// OpenUserStoreWithProvisioner permite inyectar la provisión de cuenta del
// sistema. El servidor usa OpenUserStore; esta variante sirve para pruebas y
// herramientas que no deben modificar las cuentas del equipo.
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

// Verify comprueba la prueba de autenticación de un usuario sin bloquear las
// lecturas concurrentes del archivo de usuarios.
func (s *UserStore) Verify(username, nonce, proof string) bool {
	s.mu.RLock()
	storedHash, ok := s.users[username]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	return VerifyProofHash(storedHash, username, nonce, proof)
}

// Register valida los datos, provisiona la cuenta Linux y persiste la cuenta
// de aplicación. La comprobación de duplicados y estas operaciones se
// serializan para evitar registros concurrentes del mismo nombre.
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

// ValidateUsername rechaza nombres que romperían el formato usuario:hash o que
// no podrían distinguirse de un comentario.
func ValidateUsername(username string) error {
	if !usernamePattern.MatchString(username) {
		return fmt.Errorf("%w: use de 1 a 32 caracteres, comenzando con una letra minúscula o '_' y continuando con minúsculas, dígitos, '_' o '-'", ErrInvalidUsername)
	}
	return nil
}

// EnsureSystemUser asegura que el usuario registrado existe en el sistema
// operativo antes de abrir una sesión de shell.
func (s *UserStore) EnsureSystemUser(username string) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}
	if err := s.systemUserProvisioner(username); err != nil {
		return fmt.Errorf("asegurar la cuenta Linux para %q: %w", username, err)
	}
	return nil
}

// NormalizePasswordHash valida que el valor sea un SHA-256 hexadecimal de 64
// caracteres y lo devuelve en minúsculas.
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

// appendUser agrega una línea al archivo sin reemplazarlo para conservar las
// cuentas registradas en reinicios.
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
