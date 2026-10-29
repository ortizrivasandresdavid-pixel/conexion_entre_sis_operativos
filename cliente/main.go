package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Uso: cliente <IP> <Puerto>")
		return
	}

	conn, err := net.Dial("tcp", os.Args[1]+":"+os.Args[2])
	if err != nil {
		fmt.Println("No se pudo conectar:", err)
		return
	}
	defer conn.Close()
	fmt.Println("Conectado a", conn.RemoteAddr())

	// Goroutine 1: escucha lo que llega del servidor
	go func() {
		lector := bufio.NewReader(conn)
		for {
			linea, err := lector.ReadString('\n')
			if err != nil {
				fmt.Println("Conexión cerrada por el servidor")
				os.Exit(0)
			}
			fmt.Print("<< ", linea)
		}
	}()

	// Goroutine principal: lee el teclado y envía
	teclado := bufio.NewReader(os.Stdin)
	for {
		texto, _ := teclado.ReadString('\n')
		texto = strings.TrimSpace(texto)
		if texto == "bye" {
			return
		}
		fmt.Fprintf(conn, "%s\n", texto)
	}
}
