package login

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

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
		if !ok || strings.TrimSpace(username) != username ||
			username == "" || len(passwordHash) != 64 {
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
