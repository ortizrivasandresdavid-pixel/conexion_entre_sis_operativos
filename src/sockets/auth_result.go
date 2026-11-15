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
	return errors.New(message)
}
