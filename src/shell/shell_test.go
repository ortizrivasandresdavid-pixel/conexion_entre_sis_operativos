package shell

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newTestSession crea una sesión real de shell o salta la prueba cuando el
// entorno no permite ejecutar bash/sh (por ejemplo, Windows).
func newTestSession(t *testing.T) *Session {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("la ejecución remota de comandos requiere Linux o macOS")
	}
	session, err := NewSession()
	if err != nil {
		t.Skipf("shell no disponible: %v", err)
	}
	return session
}

func TestExecuteRunsAnyCommandWithPipes(t *testing.T) {
	session := newTestSession(t)
	result, err := session.Execute(context.Background(), "printf 'uno\\ndos\\ntres\\n' | sort | head -n 2")
	if err != nil {
		t.Fatal(err)
	}
	if result.Output != "dos\ntres\n" {
		t.Fatalf("output = %q", result.Output)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d", result.ExitCode)
	}
}

func TestExecuteCapturesStderrAndExitCode(t *testing.T) {
	session := newTestSession(t)
	result, err := session.Execute(context.Background(), "echo 'algo falló' >&2; exit 7")
	if err != nil {
		t.Fatalf("a non-zero exit code must not be an execution error: %v", err)
	}
	if !strings.Contains(result.Output, "algo falló") {
		t.Fatalf("stderr was not captured: %q", result.Output)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
}

func TestSessionKeepsWorkingDirectory(t *testing.T) {
	session := newTestSession(t)
	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	result, err := session.Execute(context.Background(), "cd "+quote(dir))
	if err != nil {
		t.Fatal(err)
	}
	if result.Dir != resolved {
		t.Fatalf("session dir = %q, want %q", result.Dir, resolved)
	}

	if _, err := session.Execute(context.Background(), "mkdir sub"); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Execute(context.Background(), "cd sub"); err != nil {
		t.Fatal(err)
	}
	result, err = session.Execute(context.Background(), "pwd")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result.Output) != filepath.Join(resolved, "sub") {
		t.Fatalf("pwd = %q, want %q", result.Output, filepath.Join(resolved, "sub"))
	}
}

func TestExecuteRejectsEmptyCommand(t *testing.T) {
	session := newTestSession(t)
	if _, err := session.Execute(context.Background(), "   "); err == nil {
		t.Fatal("an empty command must be rejected")
	}
}

func TestExecuteStopsCommandAfterTimeout(t *testing.T) {
	session := newTestSession(t)
	session.Timeout = 200 * time.Millisecond
	started := time.Now()
	result, err := session.Execute(context.Background(), "sleep 10")
	if err == nil || !strings.Contains(err.Error(), "tiempo límite") {
		t.Fatalf("err = %v, want a timeout error", err)
	}
	if result.ExitCode != 124 {
		t.Fatalf("exit code = %d, want 124", result.ExitCode)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
}

func quote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
