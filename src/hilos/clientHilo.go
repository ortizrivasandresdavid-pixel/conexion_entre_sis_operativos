package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	fmt.Println("==========================================")
	fmt.Println("========  CLIENTE SERVER ANDRES  =========")
	fmt.Println("==========================================")

	if len(os.Args) < 2 {
		fmt.Println("Uso: cliente <IP del servidor>")
		return
	}
	IPserver := os.Args[1]
	dirserverTCP, err := net.ResolveTCPAddr("tcp4", IPserver+":2710")
	if err != nil {
		fmt.Println("Dirección del servidor inválida:", err)
		return
	}

	socketC, err := net.DialTCP("tcp", nil, dirserverTCP)
	if err != nil {
		fmt.Println("No se pudo conectar al servidor:", err)
		return
	}
	defer socketC.Close()
	fmt.Println("Cliente conectado ", socketC.RemoteAddr())

	done := make(chan struct{})
	go envMensajeC(socketC)
	go recMensajeC(socketC, done)
	<-done
}

func recMensajeC(socketC net.Conn, done chan struct{}) {
	defer close(done)
	bufferC := bufio.NewReader(socketC)
	for {
		msjRec, err := bufferC.ReadString('\n')
		if err != nil {
			if msjRec != "" {
				fmt.Println("Reporte recibido...", msjRec)
			}
			return
		}
		fmt.Println("Reporte recibido...", msjRec)
		fmt.Println("==================================")
		fmt.Println()
		time.Sleep(8 * time.Second)
	}
}

func envMensajeC(socketC net.Conn) {
	lector := bufio.NewReader(os.Stdin)
	bufferC := bufio.NewWriter(socketC)

	for {
		fmt.Print("Digite el comando a enviar: ")
		comando, _ := lector.ReadString('\n')
		bufferC.WriteString(comando)
		bufferC.Flush()
		fmt.Println("Comando enviado...")
	}
}
