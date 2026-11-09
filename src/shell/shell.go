package shell

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const maxOutputBytes = 64 * 1024

var diagnosticCommands = map[string][]string{
	"date":   {"-Is"},
	"df":     {"-h"},
	"free":   {"-h"},
	"ps":     {"-eo", "pid,comm,%cpu,%mem", "--sort=-%cpu"},
	"pwd":    nil,
	"uname":  {"-a"},
	"uptime": nil,
	"whoami": nil,
}

func AllowedCommands() []string {
	return []string{"date", "df -h", "free -h", "ps -eo pid,comm,%cpu,%mem --sort=-%cpu", "pwd", "uname -a", "uptime", "whoami"}
}

func Execute(ctx context.Context, command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("comando vacío")
	}
	if command == "help" {
		return "Comandos permitidos: " + strings.Join(AllowedCommands(), ", ") + "\n", nil
	}

	fields := strings.Fields(command)
	arguments, allowed := diagnosticCommands[fields[0]]
	if !allowed || strings.Join(fields, " ") != command || !sameArguments(fields[1:], arguments) {
		return "", fmt.Errorf("comando no permitido; usa help para ver los diagnósticos disponibles")
	}

	commandContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(commandContext, fields[0], arguments...).CombinedOutput()
	if len(output) > maxOutputBytes {
		output = append(output[:maxOutputBytes], []byte("\n[salida truncada]\n")...)
	}
	if err != nil {
		return string(output), fmt.Errorf("ejecutar %q: %w", command, err)
	}
	return string(output), nil
}

func sameArguments(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for i := range actual {
		if actual[i] != expected[i] {
			return false
		}
	}
	return true
}
