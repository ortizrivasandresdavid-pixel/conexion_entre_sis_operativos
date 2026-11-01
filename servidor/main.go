package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"
)

const maxIntentos = 3 // luego vendrá del .conf

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

	// --- autenticación ---
	for intentos := 1; ; intentos++ {
		linea, err := lector.ReadString('\n')
		if err != nil {
			fmt.Println("Cliente desconectado")
			return
		}
		partes := strings.SplitN(strings.TrimSpace(linea), "|", 3)
		if len(partes) == 3 && partes[0] == "AUTH" && validarUsuario(partes[1], partes[2]) {
			fmt.Println("Usuario autenticado:", partes[1])
			fmt.Fprintf(conn, "AUTH|OK\n")
			break
		}
		fmt.Println("Login incorrecto, intento", intentos)
		if intentos == maxIntentos {
			fmt.Fprintf(conn, "AUTH|BLOQUEADO\n")
			return
		}
		fmt.Fprintf(conn, "AUTH|ERROR|intento %d de %d\n", intentos, maxIntentos)
		time.Sleep(time.Duration(intentos) * 500 * time.Millisecond)
	}

	// --- sesión (por ahora eco; en el paso 3 será ejecución de comandos) ---
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
