package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingWriterRotatesAndKeepsBackups(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "servidor.log")

	writer, err := NewRotatingWriter(path, 80, 2)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := fmt.Fprintf(writer, "linea %02d: contenido de prueba\n", i); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	for _, suffix := range []string{"", ".1", ".2"} {
		info, err := os.Stat(path + suffix)
		if err != nil {
			t.Fatalf("no se creó %s: %v", path+suffix, err)
		}
		if info.Size() == 0 {
			t.Fatalf("%s está vacío", path+suffix)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("archivos de log = %d, want 3 (activo + 2 respaldos)", len(entries))
	}
}

func TestNewWritesStructuredLogToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servidor.log")
	logger, sink, err := New(Options{Path: path, MaxSizeBytes: 1 << 20, Backups: 1})
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("conexión entrante", "ip", "127.0.0.1")
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, fragment := range []string{"conexión entrante", "ip=127.0.0.1", "level=INFO"} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("el archivo de log no contiene %q:\n%s", fragment, content)
		}
	}
}
