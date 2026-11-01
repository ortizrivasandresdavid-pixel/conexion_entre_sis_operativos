package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
)

const archivoUsuarios = "oper.user.db"

// validarUsuario busca "usuario:hash" en el archivo plano
func validarUsuario(usuario, password string) bool {
	archivo, err := os.ReadFile(archivoUsuarios)
	if err != nil {
		fmt.Println("No se pudo leer", archivoUsuarios, ":", err)
		return false
	}
	// hash del password recibido (igual que en login.go)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(password)))

	usuarios := strings.Split(string(archivo), "\n")
	for i := 0; i < len(usuarios); i++ {
		campos := strings.Split(strings.TrimSpace(usuarios[i]), ":")
		if len(campos) == 2 && campos[0] == usuario && campos[1] == hash {
			return true
		}
	}
	return false
}
