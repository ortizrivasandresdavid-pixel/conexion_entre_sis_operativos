package main

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// ejecutarComando corre el comando (igual que shell.go) y envia la salida por el socket
func ejecutarComando(conn net.Conn, comando string) {
	res, err := exec.Command("bash", "-c", comando).Output()

	if len(res) > 0 {
		lineas := strings.Split(strings.TrimRight(string(res), "\n"), "\n")
		for i := 0; i < len(lineas); i++ {
			fmt.Fprintf(conn, "RESP|%s\n", lineas[i])
		}
	}
	if err != nil {
		fmt.Fprintf(conn, "RESP|ERROR: %s\n", err)
	}
	fmt.Fprintf(conn, "FIN|\n")
}
