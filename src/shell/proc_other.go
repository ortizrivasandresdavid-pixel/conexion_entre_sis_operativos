//go:build !unix

package shell

import "os/exec"

// configureProcess no hace nada fuera de Unix; este proyecto ejecuta los
// comandos remotos en Linux o macOS.
func configureProcess(cmd *exec.Cmd) {}

// killProcessTree intenta terminar el proceso del shell; en Windows no existe
// el concepto de grupo de procesos POSIX.
func killProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
