package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"operativos/src/sockets"
)

type serverConfig struct {
	Path      string
	ListenIP  string
	Port      int
	Period    int
	EventFile string
}

func findConfigPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	for _, path := range []string{
		"/etc/configProyecto.conf",
		filepath.Join("servidor", "server.conf"),
	} {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("comprobar configuración %s: %w", path, err)
		}
	}
	return "", errors.New("no se encontró /etc/configProyecto.conf ni servidor/server.conf; use -config")
}

func loadServerConfig(path, eventOverride string) (serverConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return serverConfig{}, fmt.Errorf("abrir configuración %s: %w", path, err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 64*1024)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" || value == "" {
			return serverConfig{}, fmt.Errorf("configuración inválida en %s:%d", path, lineNumber)
		}
		if _, exists := values[key]; exists {
			return serverConfig{}, fmt.Errorf("parámetro duplicado %q en %s:%d", key, path, lineNumber)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return serverConfig{}, fmt.Errorf("leer configuración %s: %w", path, err)
	}

	listenIP := values["ListenIP"]
	if net.ParseIP(listenIP) == nil {
		return serverConfig{}, errors.New("ListenIP no es una dirección IP válida")
	}
	port, err := strconv.Atoi(values["Port"])
	if err != nil || port < 1 || port > 65535 {
		return serverConfig{}, errors.New("Port debe estar entre 1 y 65535")
	}
	period, err := strconv.Atoi(values["DefaultReportPeriodSeconds"])
	if err != nil || period < 1 {
		return serverConfig{}, errors.New("DefaultReportPeriodSeconds debe ser mayor que cero")
	}

	configDir := filepath.Dir(path)
	logFile := values["LogFile"]
	if logFile == "" {
		logFile = sockets.DefaultServerLogFile
	} else if !isConfigAbs(logFile) {
		logFile = filepath.Join(configDir, logFile)
	}
	eventFile := sockets.ResolveGUIEventFile(logFile, values["EventFile"], configDir)
	if eventOverride != "" {
		eventFile = eventOverride
		if !isConfigAbs(eventFile) {
			eventFile, err = filepath.Abs(eventFile)
			if err != nil {
				return serverConfig{}, fmt.Errorf("resolver archivo de eventos: %w", err)
			}
		}
	}

	return serverConfig{
		Path:      path,
		ListenIP:  listenIP,
		Port:      port,
		Period:    period,
		EventFile: eventFile,
	}, nil
}

func isConfigAbs(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`)
}
