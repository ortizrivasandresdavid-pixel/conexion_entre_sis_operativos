package main
import (
	"fmt"
	"net"
	"bufio"

)
func main() {
	fmt.Println("**********************************")
	fmt.Println("*********  SERVER ANDRES  ********")
	fmt.Println("**********************************")

	dirTCP, _ := net.ResolveTCPAddr("tcp4", "10.1.15.66:2710")
	oyente, _ := net.ListenTCP("tcp", dirTCP)
	fmt.Println("Esperando Conexiones...")

	socketS, _ :=oyente.Accept()
	fmt.Println("Cliente conectado", socketS.RemoteAddr())

	bufferC := bufio.NewReader(socketS)
	msgRec, _ := bufferC.ReadString('\n')
	fmt.Println("Comando recibido...", msgRec)
	socketS.Close()
}