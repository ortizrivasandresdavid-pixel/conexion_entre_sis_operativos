// basicamente es un servidor que escucha en el puerto 2710 y cuando un cliente se conecta, lee lo que el cliente envia y lo devuelve con un prefijo "ECO|". Si el cliente se desconecta, el servidor imprime un mensaje y termina la conexion.
package main

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

func main() {
	oyente, err := net.Listen("tcp", ":2710") //reserva el puerto y espera a un oyente
	if err != nil {
		fmt.Println("Error al abrir el puerto:", err) //nil es sin error osea si no hay espacio lo muestra y chao
		return
	}
	defer oyente.Close() //cerrar el puerto al final del programa
	fmt.Println("Esperando conexiones...")

	conn, err := oyente.Accept() //espera conexion y cuando ya, devuelve conn, osea la conexion del cliente, y ahi se lee y escribe
	if err != nil {
		fmt.Println("Error al aceptar:", err)
		return
	}
	defer conn.Close()
	fmt.Println("Cliente conectado:", conn.RemoteAddr())

	lector := bufio.NewReader(conn) //lee lo que manda el cliente
	for {
		linea, err := lector.ReadString('\n') //lee hasta que haya un salto de linea
		if err != nil {
			fmt.Println("Cliente desconectado")
			return
		}
		linea = strings.TrimSpace(linea) //elimina el salto de linea
		fmt.Println("Recibido:", linea)
		fmt.Fprintf(conn, "ECO|%s\n", linea) //envia de vuelta al cliente lo que recibio
	}
}
