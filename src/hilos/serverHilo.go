package main
import (
	"fmt"
	"net"
	"bufio"
	"time"
	"strconv"

)
func main() {
	fmt.Println("**********************************")
	fmt.Println("*********  SERVER ANDRES  ********")
	fmt.Println("**********************************")

	dirTCP, _ := net.ResolveTCPAddr("tcp4", "10.1.15.66:2710")
	oyente, _ := net.ListenTCP("tcp", dirTCP)
	fmt.Println("Esperando Conexiones...")

	var socketS net.Conn
	socketS, _ =oyente.Accept()
	fmt.Println("Cliente conectado", socketS.RemoteAddr())


	go recMensajeS(&socketS)
	go envMensajeS(&socketS)
	for{
	// FALTA__hasta que reciba el comando bye, el servidor no se cierra
	// FALTA__ acomodar el turno de los resportes
	
	}

	socketS.Close()
}

func recMensajeS(socketS *net.Conn){

	for{
		bufferC := bufio.NewReader(*socketS)
		msgRec, _ := bufferC.ReadString('\n')
		fmt.Println("Comando recibido...", msgRec)
	}

}

func envMensajeS(socketS *net.Conn){
	num := 0
	for{
		num++
		time.Sleep(8 * time.Second)
		bufferC := bufio.NewWriter(*socketS)
		bufferC.WriteString("REPORTE " + strconv.Itoa(num) + " [DD:17%   - CPU:9%  - RAM:34%]\n")
		bufferC.Flush()
		fmt.Println("Reporte enviado...")
		fmt.Println("===========================\n")
	}
}