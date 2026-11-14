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
	if !ok || len(proof) != sha256.Size*2 {
		return false
	}

	expected := makeProof(username, storedHash, nonce)
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(proof)), []byte(expected)) == 1
}

func makeProof(username, passwordHash, nonce string) string {
	sum := sha256.Sum256([]byte(username + ":" + passwordHash + ":" + nonce))
	return hex.EncodeToString(sum[:])
}
