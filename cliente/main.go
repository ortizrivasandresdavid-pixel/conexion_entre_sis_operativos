package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"

	"golang.org/x/term"
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

	servidor := bufio.NewReader(conn)
	teclado := bufio.NewReader(os.Stdin)

	// --- autenticación ---
	fmt.Print("Usuario: ")
	usuario, _ := teclado.ReadString('\n')
	usuario = strings.TrimSpace(usuario)

	autenticado := false
	for !autenticado {
		fmt.Print("Password: ")
		pass, _ := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()

		fmt.Fprintf(conn, "AUTH|%s|%s\n", usuario, string(pass))

		resp, err := servidor.ReadString('\n')
		if err != nil {
			fmt.Println("Conexión cerrada por el servidor")
			return
		}
		resp = strings.TrimSpace(resp)
		switch {
		case resp == "AUTH|OK":
			fmt.Println("**** login CORRECTO ****")
			autenticado = true
		case resp == "AUTH|BLOQUEADO":
			fmt.Println("Demasiados intentos fallidos. Conexión cerrada.")
			return
		default:
			fmt.Println("!!!! login INCORRECTO:", resp)
		}
	}

	// Goroutine: escucha lo que llega del servidor
	go func() {
		for {
			linea, err := servidor.ReadString('\n')
			if err != nil {
				fmt.Println("Conexión cerrada por el servidor")
				os.Exit(0)
			}
			linea = strings.TrimRight(linea, "\r\n")
			if strings.HasPrefix(linea, "RESP|") {
				fmt.Println(strings.TrimPrefix(linea, "RESP|"))
			}
			// "FIN|" marca el final de la respuesta; por ahora no se muestra
		}
	}()

	// Goroutine principal: teclado -> servidor
	for {
		texto, _ := teclado.ReadString('\n')
		texto = strings.TrimSpace(texto)
		if texto == "bye" {
			return
		}
		fmt.Fprintf(conn, "%s\n", texto)
	}
}
