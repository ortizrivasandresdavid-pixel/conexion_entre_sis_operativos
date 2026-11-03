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

	IPserver := os.Args[1]
	dirserverTCP, _ := net.ResolveTCPAddr("tcp4", IPserver+":2710")
	
	var socketC net.Conn
	socketC, _ = net.DialTCP("tcp", nil, dirserverTCP)
	fmt.Println("Cliente conectado ", socketC.RemoteAddr())

	go envMensajeC(&socketC)
	go recMensajeC(&socketC)
	for{
		
	}
	socketC.Close()
}


func recMensajeC(socketC *net.Conn){
	for{
		bufferC := bufio.NewReader(*socketC)
		msjRec, _ := bufferC.ReadString('\n')
		fmt.Println("Reporte recibido...", msjRec)
		fmt.Println("==================================\n")
		time.Sleep(8 * time.Second)
	}
}
   
func envMensajeC(socketC *net.Conn){

	for{
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
}