//go:build unix

package shell

import (
	"os"
	"os/exec"
	"syscall"
)

func configureProcess(cmd *exec.Cmd, uid, gid uint32, groups []uint32) {
	credential := &syscall.Credential{Uid: uid, Gid: gid}
	if os.Geteuid() == 0 {
		credential.Groups = groups
	} else {
		credential.NoSetGroups = true
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Credential: credential}
}

func killProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
