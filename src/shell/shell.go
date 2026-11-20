package shell

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	MaxOutputBytes = 2 << 20
	DefaultTimeout = 5 * time.Minute
)

type Result struct {
	Output   string
	ExitCode int
	Dir      string
}

type Session struct {
	mu        sync.Mutex
	dir       string
	shellPath string
	username  string
	uid       uint32
	gid       uint32
	groups    []uint32
	Timeout   time.Duration
}

func NewSession(username string) (*Session, error) {
	if runtime.GOOS == "windows" {
		return nil, errors.New("la ejecución remota de comandos requiere Linux o macOS (bash/sh con descriptores extra)")
	}
	systemUser, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("buscar usuario del sistema %q: %w", username, err)
	}
	uid, err := strconv.ParseUint(systemUser.Uid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("UID no válido para %q: %w", username, err)
	}
	gid, err := strconv.ParseUint(systemUser.Gid, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("GID no válido para %q: %w", username, err)
	}
	groupIDs, err := systemUser.GroupIds()
	if err != nil {
		return nil, fmt.Errorf("obtener grupos de %q: %w", username, err)
	}
	groups := make([]uint32, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		value, err := strconv.ParseUint(groupID, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("GID suplementario no válido para %q: %w", username, err)
		}
		groups = append(groups, uint32(value))
	}
	shellPath, err := findShell()
	if err != nil {
		return nil, err
	}
	if systemUser.HomeDir == "" {
		return nil, fmt.Errorf("el usuario %q no tiene un directorio personal configurado", username)
	}
	return &Session{
		dir:       systemUser.HomeDir,
		shellPath: shellPath,
		username:  systemUser.Username,
		uid:       uint32(uid),
		gid:       uint32(gid),
		groups:    groups,
		Timeout:   DefaultTimeout,
	}, nil
}

func CheckAvailable() error {
	if runtime.GOOS == "windows" {
		return errors.New("la ejecución remota de comandos requiere Linux o macOS (bash/sh con descriptores extra)")
	}
	_, err := findShell()
	return err
}

func (s *Session) Dir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dir
}

func (s *Session) Execute(ctx context.Context, command string) (Result, error) {
	command = strings.TrimSpace(command)
	s.mu.Lock()
	defer s.mu.Unlock()

	result := Result{Dir: s.dir}
	if command == "" {
		return result, errors.New("comando vacío")
	}
	if strings.ContainsRune(command, 0) {
		return result, errors.New("el comando contiene un byte NUL no válido")
	}

	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	outputReader, outputWriter, err := os.Pipe()
	if err != nil {
		return result, fmt.Errorf("crear tubería de salida: %w", err)
	}
	defer outputReader.Close()

	dirReader, dirWriter, err := os.Pipe()
	if err != nil {
		_ = outputWriter.Close()
		return result, fmt.Errorf("crear tubería de directorio: %w", err)
	}
	defer dirReader.Close()

	const script = `eval "$DSH_CMD"
__dsh_status=$?
pwd -P >&3 2>/dev/null
exit $__dsh_status`

	cmd := exec.CommandContext(commandCtx, s.shellPath, "-c", script)
	cmd.Dir = s.dir
	cmd.Env = withEnv(os.Environ(), "DSH_CMD", command)
	cmd.Env = withEnv(cmd.Env, "HOME", s.dir)
	cmd.Env = withEnv(cmd.Env, "USER", s.username)
	cmd.Env = withEnv(cmd.Env, "LOGNAME", s.username)
	cmd.Env = withEnv(cmd.Env, "SHELL", s.shellPath)
	cmd.Stdout = outputWriter
	cmd.Stderr = outputWriter
	cmd.ExtraFiles = []*os.File{dirWriter}
	cmd.WaitDelay = 2 * time.Second
	configureProcess(cmd, s.uid, s.gid, s.groups)
	cmd.Cancel = func() error { return killProcessTree(cmd) }
	if err := cmd.Start(); err != nil {
		_ = outputWriter.Close()
		_ = dirWriter.Close()
		return result, fmt.Errorf("iniciar shell: %w", err)
	}
	_ = outputWriter.Close()
	_ = dirWriter.Close()

	var (
		outputMu  sync.Mutex
		output    bytes.Buffer
		truncated bool
	)
	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		buffer := make([]byte, 32*1024)
		for {
			n, readErr := outputReader.Read(buffer)
			if n > 0 {
				outputMu.Lock()
				if output.Len() < MaxOutputBytes {
					remaining := MaxOutputBytes - output.Len()
					if n > remaining {
						output.Write(buffer[:remaining])
						truncated = true
					} else {
						output.Write(buffer[:n])
					}
				} else {
					truncated = true
				}
				outputMu.Unlock()
			}
			if readErr != nil {
				return
			}
		}
	}()

	dirDone := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(dirReader).ReadString('\n')
		dirDone <- strings.TrimRight(line, "\r\n")
	}()

	waitErr := cmd.Wait()

	exitCode := 0
	var runErr error
	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		runErr = ctx.Err()
	case commandCtx.Err() == context.DeadlineExceeded:
		exitCode = 124
		runErr = fmt.Errorf("el comando superó el tiempo límite de %s y se canceló", timeout)
	case waitErr == nil:
	case errors.As(waitErr, &exitErr):
		exitCode = exitErr.ExitCode()
		if exitCode < 0 {
			exitCode = 1
		}
	default:
		runErr = fmt.Errorf("ejecutar shell: %w", waitErr)
	}

	select {
	case <-copyDone:
	case <-time.After(500 * time.Millisecond):
		_ = outputReader.Close()
		<-copyDone
	}

	var newDir string
	select {
	case newDir = <-dirDone:
	case <-time.After(200 * time.Millisecond):
		_ = dirReader.Close()
		newDir = <-dirDone
	}
	if newDir != "" {
		s.dir = newDir
	}
	result.Dir = s.dir

	outputMu.Lock()
	result.Output = output.String()
	outputMu.Unlock()
	if truncated {
		result.Output += "\n[salida truncada a 2 MiB]\n"
	}
	result.ExitCode = exitCode
	return result, runErr
}

func Execute(ctx context.Context, command string) (string, error) {
	defaultMu.Lock()
	if defaultSession == nil {
		currentUser, err := user.Current()
		if err != nil {
			defaultMu.Unlock()
			return "", fmt.Errorf("buscar usuario actual: %w", err)
		}
		session, err := NewSession(currentUser.Username)
		if err != nil {
			defaultMu.Unlock()
			return "", err
		}
		defaultSession = session
	}
	session := defaultSession
	defaultMu.Unlock()

	result, err := session.Execute(ctx, command)
	return result.Output, err
}

var (
	defaultMu      sync.Mutex
	defaultSession *Session
)

func findShell() (string, error) {
	for _, name := range []string{"bash", "sh"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("no se encontró bash ni sh en PATH; instala bash para ejecutar comandos")
}

func withEnv(env []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		result = append(result, item)
	}
	return append(result, prefix+value)
}
