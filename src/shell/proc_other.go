//go:build !unix

package shell

import "os/exec"

func configureProcess(cmd *exec.Cmd, _, _ uint32, _ []uint32) {}

func killProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
