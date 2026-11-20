package logging

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const defaultMaxSize = 10 << 20

type Options struct {
	Path         string
	MaxSizeBytes int64
	Backups      int
}

func New(options Options) (*slog.Logger, *RotatingWriter, error) {
	if options.MaxSizeBytes <= 0 {
		options.MaxSizeBytes = defaultMaxSize
	}
	if options.Backups < 0 {
		options.Backups = 0
	}

	writer, err := NewRotatingWriter(options.Path, options.MaxSizeBytes, options.Backups)
	if err != nil {
		return nil, nil, err
	}

	handler := slog.NewTextHandler(io.MultiWriter(os.Stdout, writer), &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler), writer, nil
}

func Console() *slog.Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler)
}

type RotatingWriter struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	backups int
	file    *os.File
	size    int64
	closed  bool
}

func NewRotatingWriter(path string, maxSize int64, backups int) (*RotatingWriter, error) {
	if path == "" {
		return nil, errors.New("la ruta del archivo de log no puede estar vacía")
	}
	if maxSize <= 0 {
		maxSize = defaultMaxSize
	}
	if backups < 0 {
		backups = 0
	}

	file, err := openLogFile(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("consultar tamaño de %s: %w", path, err)
	}

	return &RotatingWriter{
		path:    path,
		maxSize: maxSize,
		backups: backups,
		file:    file,
		size:    info.Size(),
	}, nil
}

func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, os.ErrClosed
	}
	if w.file == nil {
		return 0, os.ErrClosed
	}
	if w.size > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true
	if w.file == nil {
		return nil
	}
	_ = w.file.Sync()
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *RotatingWriter) Path() string {
	return w.path
}

func (w *RotatingWriter) rotateLocked() error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}

	if err := rotateFiles(w.path, w.backups); err != nil {
		fmt.Fprintf(os.Stderr, "logging: no se pudo rotar %s: %v\n", w.path, err)
	}

	file, err := openLogFile(w.path)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("consultar tamaño de %s: %w", w.path, err)
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func rotateFiles(path string, backups int) error {
	if backups == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}

	oldest := fmt.Sprintf("%s.%d", path, backups)
	if err := os.Remove(oldest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for i := backups - 1; i >= 1; i-- {
		from := fmt.Sprintf("%s.%d", path, i)
		to := fmt.Sprintf("%s.%d", path, i+1)
		if err := os.Rename(from, to); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Rename(path, path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func openLogFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return nil, fmt.Errorf("crear directorio de logs %s: %w", dir, err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return nil, fmt.Errorf("abrir archivo de log %s: %w", path, err)
	}
	return file, nil
}
