//go:build linux

package login

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

func EnsureSystemUser(username string) error {
	if err := ValidateUsername(username); err != nil {
		return err
	}

	systemUser, err := user.Lookup(username)
	if err != nil {
		var unknown user.UnknownUserError
		if !errors.As(err, &unknown) {
			return fmt.Errorf("buscar usuario Linux %q: %w", username, err)
		}

		command := exec.Command("useradd", "-m", "-d", filepath.Join("/home", username), "-s", "/bin/bash", "--", username)
		output, createErr := command.CombinedOutput()
		if createErr != nil {
			systemUser, err = user.Lookup(username)
			if err != nil {
				return fmt.Errorf("ejecutar useradd para %q: %w: %s", username, createErr, output)
			}
		} else {
			systemUser, err = user.Lookup(username)
			if err != nil {
				return fmt.Errorf("buscar %q después de useradd: %w", username, err)
			}
		}
	}

	uid, err := strconv.Atoi(systemUser.Uid)
	if err != nil {
		return fmt.Errorf("UID no válido para %q: %w", username, err)
	}
	gid, err := strconv.Atoi(systemUser.Gid)
	if err != nil {
		return fmt.Errorf("GID no válido para %q: %w", username, err)
	}
	if uid < 1000 {
		return fmt.Errorf("el usuario Linux %q tiene UID %d; se requiere una cuenta regular (UID >= 1000)", username, uid)
	}

	expectedHome := filepath.Join("/home", username)
	if systemUser.HomeDir != expectedHome {
		return fmt.Errorf("el home Linux de %q es %q; se requiere %q", username, systemUser.HomeDir, expectedHome)
	}
	if err := ensureHomeDirectory(expectedHome, uid, gid); err != nil {
		return fmt.Errorf("preparar home de %q: %w", username, err)
	}
	return nil
}

func ensureHomeDirectory(home string, uid, gid int) error {
	info, err := os.Lstat(home)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(home), 0o755); err != nil {
			return err
		}
		if err := os.Mkdir(home, 0o750); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err = os.Lstat(home)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s no es un directorio seguro", home)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("no se pudo leer el propietario de %s", home)
	}
	if stat.Uid != uint32(uid) || stat.Gid != uint32(gid) {
		if err := os.Chown(home, uid, gid); err != nil {
			return err
		}
	}
	if info.Mode().Perm() != 0o750 {
		if err := os.Chmod(home, 0o750); err != nil {
			return err
		}
	}
	return nil
}
