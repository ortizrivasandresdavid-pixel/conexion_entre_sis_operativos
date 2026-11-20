//go:build !linux

package login

import "errors"

// EnsureSystemUser is only available on the Linux server.
func EnsureSystemUser(string) error {
	return errors.New("la provisión de usuarios del sistema solo está disponible en Linux")
}
