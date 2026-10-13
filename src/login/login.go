package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

func main() {
	// password encriptada
	const hspasswd string ="caf90169eefa5f807d577486b9f795ab86ae2983c5c20806cff959117e90af18"
	// password: linux
	// reducir el tiempo a medida que aumenta el numero de intentos
	intentos :=1
	for {
		

		fmt.Print(fmt.Sprintf("[intento %d] digite el password:", intentos))
		// var upasswd string 
		// hacer que el password no se muestre en la pantalla

		upasswd, _:= term.ReadPassword(int(os.Stdin.Fd()))
		// encriptar password ingresado
		hupasswd := sha256.Sum256([]byte(upasswd))
		//fmt.Println("hupasswd: %x \n", hupasswd)
		shupasswd := fmt.Sprintf("%x", hupasswd)
		
		if shupasswd == hspasswd {
			fmt.Println("\n**** login CORRECTO ****")
			break;
		}
		
		fmt.Println("\n!!!! login INCORRECTO ¡¡¡¡")
		intentos++
		time.Sleep(time.Duration(intentos) * 500 * time.Millisecond)
			
	}

}
