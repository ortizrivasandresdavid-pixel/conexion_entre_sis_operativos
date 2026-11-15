package sockets

import (
	"errors"
	"fmt"
)

func authResultError(result Message) error {
	message := result.Error
	if message == "" {
		message = "autenticación fallida"
	}
	message += fmt.Sprintf("; intentos restantes: %d", result.AttemptsRemaining)
	if result.RetryAfterSeconds > 0 {
		message += fmt.Sprintf("; espera %d segundos", result.RetryAfterSeconds)
	}
	return &AuthResultError{Code: result.Code, message: message}
}

type AuthResultError struct {
	Code    string
	message string
}

func (e *AuthResultError) Error() string {
	return e.message
}

func IsAuthLocked(err error) bool {
	var authError *AuthResultError
	return errors.As(err, &authError) && authError.Code == "AUTH_LOCKED"
}
