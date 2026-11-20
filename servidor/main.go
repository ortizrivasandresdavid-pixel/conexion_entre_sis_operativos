package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"operativos/src/logging"
	"operativos/src/sockets"
)

const (
	// Ruta usada por el servicio systemd. Se puede cambiar con -config.
	defaultConfigPath = "/etc/configProyecto.conf"
	// Los logs se guardan en un directorio para que la rotación funcione con
	// ProtectSystem y LogsDirectory= de systemd.
	defaultLogPath      = sockets.DefaultServerLogFile
	defaultLogMaxSizeMB = 10
	defaultLogBackups   = 5
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run() error {
	configFlag := flag.String("config", defaultConfigPath, "archivo de configuración del servidor")
	usersFlag := flag.String("users", "", "archivo de usuarios (sobrescribe UsersFile)")
	logFlag := flag.String("log", "", "archivo de log (sobrescribe LogFile)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Uso: %s [opciones]\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	configPath, err := resolveConfigPath(*configFlag, flagWasSet("config"))
	if err != nil {
		return err
	}
	config, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("configuración: %w", err)
	}
	if *usersFlag != "" {
		config.UsersFile, err = absolutePath(*usersFlag)
		if err != nil {
			return fmt.Errorf("ruta del archivo de usuarios: %w", err)
		}
	}
	if *logFlag != "" {
		config.LogFile, err = absolutePath(*logFlag)
		if err != nil {
			return fmt.Errorf("ruta del archivo de log: %w", err)
		}
	}

	logger, sink, err := logging.New(logging.Options{
		Path:         config.LogFile,
		MaxSizeBytes: int64(config.LogMaxSizeMB) << 20,
		Backups:      config.LogBackups,
	})
	if err != nil {
		// Si el usuario pidió explícitamente -log, el fallo es fatal: no
		// queremos arrancar sin el registro de auditoría solicitado. Si solo
		// se usó el valor por defecto, se continúa con stdout.
		if flagWasSet("log") {
			return fmt.Errorf("abrir archivo de log %s: %w", config.LogFile, err)
		}
		fmt.Fprintf(os.Stderr, "aviso: no se pudo abrir %s: %v; se registrará solo en stdout\n", config.LogFile, err)
		logger = logging.Console()
	}
	if sink != nil {
		defer func() {
			if err := sink.Close(); err != nil {
				fmt.Fprintln(os.Stderr, "Error al cerrar el archivo de log:", err)
			}
		}()
	}
	slog.SetDefault(logger)
	logger.Info("configuración cargada",
		"archivo", configPath,
		"usuarios", config.UsersFile,
		"log", config.LogFile,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := sockets.RunServer(ctx, config); err != nil {
		logger.Error("el servidor terminó con error", "error", err)
		return fmt.Errorf("servidor: %w", err)
	}
	return nil
}

func flagWasSet(name string) bool {
	set := false
	flag.Visit(func(current *flag.Flag) {
		if current.Name == name {
			set = true
		}
	})
	return set
}

// resolveConfigPath aplica la ruta indicada por -config o, si no se indicó,
// usa /etc/configProyecto.conf. Para no romper el flujo de desarrollo anterior,
// si esa ruta no existe intenta servidor/server.conf relativo al directorio
// actual.
func resolveConfigPath(value string, explicit bool) (string, error) {
	if explicit {
		return value, nil
	}
	if _, err := os.Stat(value); err == nil {
		return value, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("comprobar %s: %w", value, err)
	}

	legacy := filepath.Join("servidor", "server.conf")
	if _, err := os.Stat(legacy); err == nil {
		fmt.Fprintf(os.Stderr, "aviso: no se encontró %s; usando %s (modo desarrollo)\n", value, legacy)
		return legacy, nil
	}
	return value, nil
}

func absolutePath(path string) (string, error) {
	if isAbsolutePath(path) {
		return path, nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return absolute, nil
}

// isAbsolutePath reconoce también las rutas estilo Linux cuando el binario se
// compila o prueba en Windows (por ejemplo, /etc/configProyecto.conf).
func isAbsolutePath(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`)
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
	optional := map[string]bool{
		"LogFile":      true,
		"LogMaxSizeMB": true,
		"LogBackups":   true,
		"EventFile":    true,
	}
	for _, key := range required {
		if values[key] == "" {
			return sockets.ServerConfig{}, fmt.Errorf("falta el parámetro %s", key)
		}
	}
	for key := range values {
		known := optional[key]
		for _, requiredKey := range required {
			if key == requiredKey {
				known = true
				break
			}
		}
		if !known {
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

	parseOptionalInt := func(key string, fallback int) (int, error) {
		raw, ok := values[key]
		if !ok {
			return fallback, nil
		}
		value, err := strconv.Atoi(raw)
		if err != nil {
			return 0, fmt.Errorf("%s debe ser un entero: %w", key, err)
		}
		return value, nil
	}
	logMaxSizeMB, err := parseOptionalInt("LogMaxSizeMB", defaultLogMaxSizeMB)
	if err != nil {
		return sockets.ServerConfig{}, err
	}
	if logMaxSizeMB < 1 {
		return sockets.ServerConfig{}, fmt.Errorf("LogMaxSizeMB debe ser mayor que cero")
	}
	logBackups, err := parseOptionalInt("LogBackups", defaultLogBackups)
	if err != nil {
		return sockets.ServerConfig{}, err
	}
	if logBackups < 0 {
		return sockets.ServerConfig{}, fmt.Errorf("LogBackups no puede ser negativo")
	}

	usersFile := values["UsersFile"]
	if !isAbsolutePath(usersFile) {
		usersFile = filepath.Join(filepath.Dir(path), usersFile)
	}
	logFile := values["LogFile"]
	if logFile == "" {
		logFile = defaultLogPath
	}
	if !isAbsolutePath(logFile) {
		logFile = filepath.Join(filepath.Dir(path), logFile)
	}
	eventFile := sockets.ResolveGUIEventFile(logFile, values["EventFile"], filepath.Dir(path))

	return sockets.ServerConfig{
		ListenIP:                   values["ListenIP"],
		Port:                       port,
		AllowedClientIP:            values["AllowedClientIP"],
		MaxAuthAttempts:            attempts,
		UsersFile:                  usersFile,
		DefaultReportPeriodSeconds: defaultPeriod,
		MaxReportPeriodSeconds:     maxPeriod,
		LogFile:                    logFile,
		LogMaxSizeMB:               logMaxSizeMB,
		LogBackups:                 logBackups,
		EventFile:                  eventFile,
	}, nil
}
