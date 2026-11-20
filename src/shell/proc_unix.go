//go:build unix

package shell

import (
	"os"
	"os/exec"
	"syscall"
)

// configureProcess hace que el shell sea líder de su propio grupo de procesos,
// para poder cancelar también los procesos que lance el comando.
func configureProcess(cmd *exec.Cmd, uid, gid uint32, groups []uint32) {
	credential := &syscall.Credential{Uid: uid, Gid: gid}
	if os.Geteuid() == 0 {
		credential.Groups = groups
	} else {
		credential.NoSetGroups = true
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Credential: credential}
}

// killProcessTree envía SIGKILL al grupo completo del shell.
func killProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
