package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"operativos/src/sockets"
)

func main() {
	configPath := flag.String("config", "servidor/server.conf", "archivo de configuración")
	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error de configuración:", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := sockets.RunServer(ctx, config); err != nil {
		fmt.Fprintln(os.Stderr, "Error del servidor:", err)
		os.Exit(1)
	}
}

func loadConfig(path string) (sockets.ServerConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return sockets.ServerConfig{}, fmt.Errorf("abrir %s: %w", path, err)
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
		if !ok || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			return sockets.ServerConfig{}, fmt.Errorf("formato inválido en %s:%d", path, lineNumber)
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if _, exists := values[key]; exists {
			return sockets.ServerConfig{}, fmt.Errorf("parámetro duplicado %q en %s:%d", key, path, lineNumber)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return sockets.ServerConfig{}, fmt.Errorf("leer %s: %w", path, err)
	}

	required := []string{
		"ListenIP",
		"Port",
		"AllowedClientIP",
		"MaxAuthAttempts",
		"UsersFile",
		"DefaultReportPeriodSeconds",
		"MaxReportPeriodSeconds",
	}
	for _, key := range required {
		if values[key] == "" {
			return sockets.ServerConfig{}, fmt.Errorf("falta el parámetro %s", key)
		}
	}
	for key := range values {
		found := false
		for _, requiredKey := range required {
			if key == requiredKey {
				found = true
				break
			}
		}
		if !found {
			return sockets.ServerConfig{}, fmt.Errorf("parámetro desconocido %q", key)
		}
	}

	parseInt := func(key string) (int, error) {
		value, err := strconv.Atoi(values[key])
		if err != nil {
			return 0, fmt.Errorf("%s debe ser un entero: %w", key, err)
		}
		return value, nil
	}
	port, err := parseInt("Port")
	if err != nil {
		return sockets.ServerConfig{}, err
	}
	attempts, err := parseInt("MaxAuthAttempts")
	if err != nil {
		return sockets.ServerConfig{}, err
	}
	defaultPeriod, err := parseInt("DefaultReportPeriodSeconds")
	if err != nil {
		return sockets.ServerConfig{}, err
	}
	maxPeriod, err := parseInt("MaxReportPeriodSeconds")
	if err != nil {
		return sockets.ServerConfig{}, err
	}

	usersFile := values["UsersFile"]
	if !filepath.IsAbs(usersFile) {
		usersFile = filepath.Join(filepath.Dir(path), usersFile)
	}
	return sockets.ServerConfig{
		ListenIP:                   values["ListenIP"],
		Port:                       port,
		AllowedClientIP:            values["AllowedClientIP"],
		MaxAuthAttempts:            attempts,
		UsersFile:                  usersFile,
		DefaultReportPeriodSeconds: defaultPeriod,
		MaxReportPeriodSeconds:     maxPeriod,
	}, nil
}
