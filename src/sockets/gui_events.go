package sockets

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultServerLogFile es el log de auditoría cuando LogFile no viene en
	// la configuración. La GUI del servidor no lo lee.
	DefaultServerLogFile = "/var/log/proyecto-operativos/servidor.log"
	// GUIEventFileName es el flujo JSONL que el monitor gráfico lee en vivo.
	// Vive junto al log salvo que EventFile indique otra ruta.
	GUIEventFileName = "eventos-gui.jsonl"
	// DefaultGUIEventFile es la ruta de producción, legible por el usuario
	// de escritorio cuando el directorio queda en 0755 y el archivo en 0644.
	DefaultGUIEventFile = "/var/log/proyecto-operativos/" + GUIEventFileName

	maxGUIEventBytes = 8 << 20
)

// GUIEvent es una línea del flujo local que consume la GUI del servidor.
// El servidor TCP no usa este tipo en el protocolo con los clientes.
type GUIEvent struct {
	Type          string `json:"type"`
	Timestamp     string `json:"timestamp"`
	Clock         string `json:"clock"`
	ClientIP      string `json:"client_ip,omitempty"`
	User          string `json:"user,omitempty"`
	Status        string `json:"status,omitempty"`
	Report        uint64 `json:"report,omitempty"`
	Command       string `json:"command,omitempty"`
	CommandID     uint64 `json:"command_id,omitempty"`
	ActiveClients int    `json:"active_clients"`
	Action        string `json:"action,omitempty"`
	Authenticated bool   `json:"authenticated,omitempty"`
	Error         string `json:"error,omitempty"`
}

// ResolveGUIEventFile elige el JSONL de la GUI con la misma regla en el
// servidor y en el monitor: EventFile si está definido; si no, el archivo
// eventos-gui.jsonl en el directorio de LogFile.
func ResolveGUIEventFile(logFile, eventFile, baseDir string) string {
	eventFile = strings.TrimSpace(eventFile)
	if eventFile != "" {
		if isConfigAbs(eventFile) {
			return eventFile
		}
		return filepath.Join(baseDir, eventFile)
	}
	logFile = strings.TrimSpace(logFile)
	if logFile == "" {
		return DefaultGUIEventFile
	}
	if !isConfigAbs(logFile) {
		logFile = filepath.Join(baseDir, logFile)
	}
	return joinConfigPath(dirOfConfigPath(logFile), GUIEventFileName)
}

func isConfigAbs(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`)
}

func dirOfConfigPath(path string) string {
	if strings.HasPrefix(path, "/") {
		if i := strings.LastIndex(path, "/"); i > 0 {
			return path[:i]
		}
	}
	return filepath.Dir(path)
}

func joinConfigPath(dir, name string) string {
	if strings.HasPrefix(dir, "/") {
		return strings.TrimRight(dir, "/") + "/" + name
	}
	return filepath.Join(dir, name)
}

// guiEventLog escribe eventos en un JSONL local. Un fallo de disco no se
// propaga a la sesión TCP: el monitor es opcional.
type guiEventLog struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	size    int64
	reports uint64
	clients int
	warned  bool
}

func openGUIEventLog(path string) (*guiEventLog, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("ruta de eventos vacía")
	}
	if err := ensureEventDir(path); err != nil {
		return nil, err
	}
	file, err := createGUIEventFile(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("consultar %s: %w", path, err)
	}
	log := &guiEventLog{file: file, path: path, size: info.Size()}
	log.session("ready", "", "", false)
	return log, nil
}

func (e *guiEventLog) Close() error {
	if e == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.file == nil {
		return nil
	}
	err := e.file.Close()
	e.file = nil
	return err
}

// session registra conexión, login o desconexión. authenticated solo resta
// un cliente activo cuando la sesión había entrado con login.
func (e *guiEventLog) session(action, ip, user string, authenticated bool) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	switch action {
	case "login":
		e.clients++
		authenticated = true
	case "disconnect":
		if authenticated && e.clients > 0 {
			e.clients--
		}
	}
	e.writeLocked(GUIEvent{
		Type:          "session",
		Action:        action,
		ClientIP:      ip,
		User:          user,
		Authenticated: authenticated,
	})
}

func (e *guiEventLog) commandReceived(ip, user string, id uint64, command string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.writeLocked(GUIEvent{
		Type:      "command_received",
		ClientIP:  ip,
		User:      user,
		CommandID: id,
		Command:   shortCommand(strings.TrimSpace(command)),
	})
}

func (e *guiEventLog) reportSent(ip, user, status string, cause error) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.reports++
	event := GUIEvent{
		Type:     "report_sent",
		ClientIP: ip,
		User:     user,
		Status:   status,
		Report:   e.reports,
	}
	if cause != nil {
		event.Error = shortCommand(cause.Error())
	}
	e.writeLocked(event)
}

func (e *guiEventLog) writeLocked(event GUIEvent) {
	if e.file == nil {
		return
	}
	now := time.Now()
	event.Timestamp = now.Format(time.RFC3339)
	event.Clock = now.Format("15:04:05")
	event.ActiveClients = e.clients
	payload, err := json.Marshal(event)
	if err != nil {
		e.warnLocked(err)
		return
	}
	payload = append(payload, '\n')
	if e.size > 0 && e.size+int64(len(payload)) > maxGUIEventBytes {
		if err := e.rotateLocked(); err != nil {
			e.warnLocked(err)
			return
		}
	}
	n, err := e.file.Write(payload)
	e.size += int64(n)
	if err != nil {
		e.warnLocked(err)
	}
}

func (e *guiEventLog) rotateLocked() error {
	if e.file != nil {
		_ = e.file.Close()
		e.file = nil
	}
	backup := e.path + ".1"
	_ = os.Remove(backup)
	if err := os.Rename(e.path, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
		file, openErr := createGUIEventFile(e.path)
		if openErr != nil {
			return err
		}
		e.file = file
		e.size = 0
		return err
	}
	file, err := createGUIEventFile(e.path)
	if err != nil {
		return err
	}
	e.file = file
	e.size = 0
	return nil
}

func (e *guiEventLog) warnLocked(err error) {
	if e.warned || err == nil {
		return
	}
	e.warned = true
	slog.Warn("no se pudo escribir el flujo de la GUI; las sesiones siguen activas",
		"archivo", e.path,
		"error", err,
	)
}

func ensureEventDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	_, statErr := os.Stat(dir)
	missing := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !missing {
		return fmt.Errorf("comprobar directorio de eventos %s: %w", dir, statErr)
	}
	if missing {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("crear directorio de eventos %s: %w", dir, err)
		}
	}
	if missing || isSharedEventDir(dir) {
		// 0755 deja entrar al usuario de escritorio. No se tocan directorios
		// del proyecto: ahí puede vivir users.conf.
		if err := os.Chmod(dir, 0755); err != nil && missing {
			return fmt.Errorf("permisos de %s: %w", dir, err)
		}
	}
	return nil
}

func isSharedEventDir(dir string) bool {
	slash := filepath.ToSlash(filepath.Clean(dir))
	switch slash {
	case "/var/log/proyecto-operativos", "/run/servidor-operativos":
		return true
	default:
		return strings.HasPrefix(dir, "/var/log/proyecto-operativos") ||
			strings.HasPrefix(dir, "/run/servidor-operativos")
	}
}

func createGUIEventFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("abrir flujo de eventos %s: %w", path, err)
	}
	// La umask del servicio (0027) dejaría el archivo en 0640. El monitor
	// corre con el usuario de escritorio y necesita poder leerlo.
	if err := os.Chmod(path, 0644); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("permisos de %s: %w", path, err)
	}
	return file, nil
}
