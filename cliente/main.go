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
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "Uso: cliente <IP-servidor> <puerto> <periodo-segundos>")
		os.Exit(2)
	}
	if net.ParseIP(os.Args[1]) == nil {
		fmt.Fprintln(os.Stderr, "La IP del servidor no es válida")
		os.Exit(2)
	}
	port, err := strconv.Atoi(os.Args[2])
	if err != nil || port < 1 || port > 65535 {
		fmt.Fprintln(os.Stderr, "El puerto debe estar entre 1 y 65535")
		os.Exit(2)
	}
	period, err := strconv.Atoi(os.Args[3])
	if err != nil || period < 1 || period > 3600 {
		fmt.Fprintln(os.Stderr, "El período debe estar entre 1 y 3600 segundos")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	address := net.JoinHostPort(os.Args[1], strconv.Itoa(port))
	if err := sockets.RunClient(ctx, address, period); err != nil {
		fmt.Fprintln(os.Stderr, "Error del cliente:", err)
		os.Exit(1)
	}
}
