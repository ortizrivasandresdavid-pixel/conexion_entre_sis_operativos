package login

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

func HashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func MakeProof(username, password, nonce string) string {
	return makeProof(username, HashPassword(password), nonce)
}

func VerifyProof(users map[string]string, username, nonce, proof string) bool {
	storedHash, ok := users[username]
	if !ok {
		return false
	}
	return VerifyProofHash(storedHash, username, nonce, proof)
}

// VerifyProofHash comprueba una prueba de autenticación contra un hash ya
// almacenado. Se usa tanto en el login como al validar el registro de una
// cuenta nueva, donde el hash llega en la propia solicitud.
func VerifyProofHash(storedHash, username, nonce, proof string) bool {
	if len(storedHash) != sha256.Size*2 || len(proof) != sha256.Size*2 {
		return false
	}

	expected := makeProof(username, strings.ToLower(storedHash), nonce)
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(proof)), []byte(expected)) == 1
}

func makeProof(username, passwordHash, nonce string) string {
	sum := sha256.Sum256([]byte(username + ":" + passwordHash + ":" + nonce))
	return hex.EncodeToString(sum[:])
}
