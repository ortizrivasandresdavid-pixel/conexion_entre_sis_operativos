package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

func main() {
	oyente, err := net.Listen("tcp", ":2710")
	if err != nil {
		fmt.Println("Error al abrir el puerto:", err)
		return
	}
	defer oyente.Close()
	fmt.Println("Esperando conexiones...")

	conn, err := oyente.Accept()
	if err != nil {
		fmt.Println("Error al aceptar:", err)
		return
	}
	defer conn.Close()
	fmt.Println("Cliente conectado:", conn.RemoteAddr())

	lector := bufio.NewReader(conn)
	for {
		linea, err := lector.ReadString('\n')
		if err != nil {
			fmt.Println("Cliente desconectado")
			return
		}
		linea = strings.TrimSpace(linea)
		fmt.Println("Recibido:", linea)
		fmt.Fprintf(conn, "ECO|%s\n", linea)
	}
}
