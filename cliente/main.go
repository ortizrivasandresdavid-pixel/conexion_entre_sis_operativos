package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"operativos/src/sockets"
)

func main() {
	address, period, err := parseClientArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := sockets.RunClient(ctx, address, period); err != nil {
		fmt.Fprintln(os.Stderr, "Error del cliente:", err)
		os.Exit(1)
	}
}

func parseClientArgs(args []string) (string, int, error) {
	if len(args) < 1 || len(args) > 3 {
		return "", 0, fmt.Errorf("Uso: cliente <IP-servidor> [puerto] [periodo-segundos]")
	}
	if net.ParseIP(args[0]) == nil {
		return "", 0, fmt.Errorf("La IP del servidor no es válida")
	}

	port := 2306
	if len(args) >= 2 {
		value, err := strconv.Atoi(args[1])
		if err != nil || value < 1 || value > 65535 {
			return "", 0, fmt.Errorf("El puerto debe estar entre 1 y 65535")
		}
		port = value
	}

	period := 5
	if len(args) == 3 {
		value, err := strconv.Atoi(args[2])
		if err != nil || value < 1 || value > 3600 {
			return "", 0, fmt.Errorf("El período debe estar entre 1 y 3600 segundos")
		}
		period = value
	}

	return net.JoinHostPort(args[0], strconv.Itoa(port)), period, nil
}
