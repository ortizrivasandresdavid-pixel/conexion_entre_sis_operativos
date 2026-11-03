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
	
	var socketC net.Conn
	socketC, _ = net.DialTCP("tcp", nil, dirserverTCP)
	fmt.Println("Cliente conectado ", socketC.RemoteAddr())

	for{
		envMensajeC(&socketC)
		recMensajeC(&socketC)
	}
	socketC.Close()
}


func recMensajeC(socketC *net.Conn){
	bufferC := bufio.NewReader(*socketC)
	msjRec, _ := bufferC.ReadString('\n')
	fmt.Println("Reporte recibido...", msjRec)
	fmt.Println("==================================\n")
}
   
func envMensajeC(socketC *net.Conn){

	fmt.Print("Digite el comando a enviar: ")
    //fmt.Scanf("%s",&comando);
    lector :=bufio.NewReader(os.Stdin)
	comando, _ := lector.ReadString('\n')
	//comando = strings.TrimRight(comando, "\n")

	bufferC := bufio.NewWriter(*socketC)
	bufferC.WriteString(comando)
	bufferC.Flush()
	fmt.Println("Comando enviado...")
}