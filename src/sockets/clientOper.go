package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
)

func main() {
	fmt.Println("==========================================")
	fmt.Println("========  CLIENTE SERVER ANDRES  =========")
	fmt.Println("==========================================")

	IPserver := os.Args[1]
	dirserverTCP, _ := net.ResolveTCPAddr("tcp4", IPserver+":2710")
	SocketC, _ := net.DialTCP("tcp", nil, dirserverTCP)


	fmt.Println("Cliente conectado : ", SocketC.RemoteAddr())

	bufferC := bufio.NewWriter(SocketC)
	bufferC.WriteString("ls -l desde aortiz...\n")
	bufferC.Flush()
	fmt.Println("Comando enviado...")
	
}
