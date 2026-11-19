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
	return &AuthResultError{
		Code:              result.Code,
		Message:           message,
		AttemptsRemaining: result.AttemptsRemaining,
		RetryAfterSeconds: result.RetryAfterSeconds,
	}
}

func registerResultError(result Message) error {
	message := result.Error
	if message == "" {
		message = "no se pudo crear la cuenta"
	}
	return &RegistrationError{Code: result.Code, Message: message}
}

type AuthResultError struct {
	Code              string
	Message           string
	AttemptsRemaining int
	RetryAfterSeconds int
}

func (e *AuthResultError) Error() string {
	return e.Message
}

// RegistrationError describe un rechazo del registro (por ejemplo, un usuario
// duplicado o un hash inválido). La GUI lo muestra sin abandonar el Paso 2.
type RegistrationError struct {
	Code    string
	Message string
}

func (e *RegistrationError) Error() string {
	return e.Message
}

func IsAuthLocked(err error) bool {
	var authError *AuthResultError
	return errors.As(err, &authError) && authError.Code == "AUTH_LOCKED"
}

func IsUserExists(err error) bool {
	var registrationError *RegistrationError
	return errors.As(err, &registrationError) && registrationError.Code == "USER_EXISTS"
}
