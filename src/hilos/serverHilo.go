package main

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

func main() {
	fmt.Println("**********************************")
	fmt.Println("*********  SERVER ANDRES  ********")
	fmt.Println("**********************************")

	dirTCP, err := net.ResolveTCPAddr("tcp4", "10.1.15.66:2710")
	if err != nil {
		fmt.Println("Error al resolver la dirección:", err)
		return
	}
	oyente, err := net.ListenTCP("tcp", dirTCP)
	if err != nil {
		fmt.Println("Error al iniciar el servidor:", err)
		return
	}
	defer oyente.Close()
	fmt.Println("Esperando Conexiones...")

	socketS, err := oyente.Accept()
	if err != nil {
		fmt.Println("Error al aceptar la conexión:", err)
		return
	}
	defer socketS.Close()
	fmt.Println("Cliente conectado", socketS.RemoteAddr())

	done := make(chan struct{})
	go recMensajeS(socketS, done)
	go envMensajeS(socketS, done)
	<-done
	fmt.Println("Cerrando el servidor...")
}

func recMensajeS(socketS net.Conn, done chan struct{}) {
	defer close(done)
	bufferC := bufio.NewReader(socketS)
	for {
		msgRec, err := bufferC.ReadString('\n')
		fmt.Println("Comando recibido...", msgRec)
		if strings.TrimSpace(msgRec) == "bye" {
			return
		}
		if err != nil {
			return
		}
	}
}

func envMensajeS(socketS net.Conn, done <-chan struct{}) {
	num := 0
	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()
	bufferC := bufio.NewWriter(socketS)
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
		}
		num++
		_, err := bufferC.WriteString("REPORTE " + strconv.Itoa(num) + " [DD:17%   - CPU:9%  - RAM:34%]\n")
		if err != nil {
			return
		}
		if err := bufferC.Flush(); err != nil {
			return
		}
		fmt.Println("Reporte enviado...")
		fmt.Println("===========================")
		fmt.Println()
	}
}
