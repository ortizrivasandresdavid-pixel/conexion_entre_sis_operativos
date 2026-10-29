package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	if len(os.Args) != 3 { //[1] = IP, [2] = Puerto, esperamos 3 argumentos
		fmt.Println("Uso: cliente <IP> <Puerto>")
		return
	}

	conn, err := net.Dial("tcp", os.Args[1]+":"+os.Args[2]) //une ip y puerto ip:puerto, abre conexion
	if err != nil {
		fmt.Println("No se pudo conectar:", err)
		return
	}
	defer conn.Close()
	fmt.Println("Conectado a", conn.RemoteAddr())

	// Goroutine 1: escucha lo que llega del servidor
	go func() {
		lector := bufio.NewReader(conn) //lee lo que manda el servidor
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
	teclado := bufio.NewReader(os.Stdin) //lee lo que escribe el usuario
	for {
		texto, _ := teclado.ReadString('\n')
		texto = strings.TrimSpace(texto) //elimina el salto de linea
		if texto == "bye" {
			return
		}
		fmt.Fprintf(conn, "%s\n", texto) //envia al servidor lo que escribio el usuario
	}
}

//los dos goroutines corren en paralelo, uno escucha lo que llega del servidor y el otro lee lo que escribe el usuario y lo envia al servidor.
