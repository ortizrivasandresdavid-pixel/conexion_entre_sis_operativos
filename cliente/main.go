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

const clientUsage = "Uso: cliente <IP-servidor> <puerto> <periodo-segundos>\n" +
	"Ejemplo: cliente 10.8.0.1 2306 5"

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "Error: faltan argumentos.")
		fmt.Fprintln(os.Stderr, clientUsage)
		os.Exit(1)
	}

	address, period, err := parseClientArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		fmt.Fprintln(os.Stderr, clientUsage)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := sockets.RunClient(ctx, address, period); err != nil {
		fmt.Fprintln(os.Stderr, "Error del cliente:", err)
		os.Exit(1)
	}
}

func parseClientArgs(args []string) (string, int, error) {
	if len(args) != 3 {
		return "", 0, fmt.Errorf("se requieren exactamente 3 argumentos")
	}

	ip := args[0]
	if net.ParseIP(ip) == nil {
		return "", 0, fmt.Errorf("la IP del servidor no es válida")
	}

	port, err := strconv.Atoi(args[1])
	if err != nil || port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("el puerto debe ser un entero entre 1 y 65535")
	}

	period, err := strconv.Atoi(args[2])
	if err != nil || period <= 0 {
		return "", 0, fmt.Errorf("el período debe ser un entero mayor que 0")
	}
	if period > 3600 {
		return "", 0, fmt.Errorf("el período no puede superar 3600 segundos")
	}

	return net.JoinHostPort(ip, strconv.Itoa(port)), period, nil
}
