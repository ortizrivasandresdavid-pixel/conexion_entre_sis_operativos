// Package shell ejecuta comandos reales en el servidor, como una terminal.
//
// Cada sesión conserva el directorio de trabajo entre comandos, de modo que
// "cd carpeta" afecta a los comandos siguientes. No hay lista blanca: el
// comando llega al servidor y se evalúa con bash (o sh), devolviendo stdout y
// stderr mezclados en el orden en que se produjeron.
package shell

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// MaxOutputBytes limita la salida que se envía al cliente para no agotar
	// la memoria ni romper el protocolo. Si se supera se añade un aviso.
	MaxOutputBytes = 2 << 20 // 2 MiB
	// DefaultTimeout evita que un comando interactivo o colgado bloquee la
	// sesión para siempre.
	DefaultTimeout = 5 * time.Minute
)

// Result es el resultado de un comando.
type Result struct {
	// Output contiene stdout y stderr mezclados.
	Output string
	// ExitCode es el código de salida de bash (0 si todo fue bien).
	ExitCode int
	// Dir es el directorio de trabajo después de ejecutar el comando.
	Dir string
}

// Session representa una sesión de shell remota con su propio directorio de
// trabajo. Los comandos de una misma sesión se ejecutan de forma serializada.
type Session struct {
	mu        sync.Mutex
	dir       string
	shellPath string
	// Timeout es el tiempo máximo por comando. Si es cero se usa
	// DefaultTimeout.
	Timeout time.Duration
}

// NewSession crea una sesión de shell que empieza en el directorio de trabajo
// actual del proceso servidor.
func NewSession() (*Session, error) {
	if runtime.GOOS == "windows" {
		return nil, errors.New("la ejecución remota de comandos requiere Linux o macOS (bash/sh con descriptores extra)")
	}
	shellPath, err := findShell()
	if err != nil {
		return nil, err
	}
	dir, err := os.Getwd()
	if err != nil {
		dir = string(os.PathSeparator)
	}
	return &Session{dir: dir, shellPath: shellPath, Timeout: DefaultTimeout}, nil
}

// Dir devuelve el directorio de trabajo actual de la sesión.
func (s *Session) Dir() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dir
}

// Execute evalúa command en la sesión y devuelve su salida y su código de
// salida. Solo devuelve error cuando el comando no se pudo ejecutar (shell
// ausente, tiempo límite, contexto cancelado); un comando que termina con
// código distinto de cero no es un error de ejecución.
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

	// El comando viaja en una variable de entorno para no tener que citarlo
	// dentro del script. eval lo ejecuta en el shell actual, así "cd" cambia
	// el directorio de verdad. El script siempre informa el nuevo directorio
	// por el descriptor 3 y termina con el código de salida del comando.
	const script = `eval "$DSH_CMD"
__dsh_status=$?
pwd -P >&3 2>/dev/null
exit $__dsh_status`

	cmd := exec.CommandContext(commandCtx, s.shellPath, "-c", script)
	cmd.Dir = s.dir
	cmd.Env = withEnv(os.Environ(), "DSH_CMD", command)
	cmd.Stdout = outputWriter
	cmd.Stderr = outputWriter
	cmd.ExtraFiles = []*os.File{dirWriter}
	cmd.WaitDelay = 2 * time.Second
	configureProcess(cmd)
	cmd.Cancel = func() error { return killProcessTree(cmd) }
	if err := cmd.Start(); err != nil {
		_ = outputWriter.Close()
		_ = dirWriter.Close()
		return result, fmt.Errorf("iniciar shell: %w", err)
	}
	// El proceso hijo ya heredó los descriptores; cerramos nuestras copias
	// para que las lecturas terminen cuando el hijo cierre las suyas.
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

	// Si un proceso lanzado en segundo plano heredó la tubería, no esperamos
	// indefinidamente: cerramos y conservamos lo recibido.
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

// Execute conserva la antigua API de conveniencia: ejecuta command en una
// sesión compartida que conserva el directorio de trabajo. La consola local de
// src/shell.go la usa; los clientes remotos usan una Session por conexión.
func Execute(ctx context.Context, command string) (string, error) {
	defaultMu.Lock()
	if defaultSession == nil {
		session, err := NewSession()
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

// withEnv reemplaza (o agrega) key en env sin duplicarla.
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
