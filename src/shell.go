package main

import (
	"fmt"
	"bufio"
	"os"
	"os/exec"

	"strings"
)
func main() {
	//var comando string;
	for {
    	
    	fmt.Print("Oper# ")
    	//fmt.Scanf("%s",&comando);
    	lector :=bufio.NewReader(os.Stdin)
		comando, _ := lector.ReadString('\n')
		comando = strings.TrimRight(comando, "\n") // eliminar el salto de linea
		fmt.Println("ejecutando ",comando," ... ");
		/* --- codigo para ejecutar comandos ---  */
		var shell *exec.Cmd
		shell = exec.Command("bash", "-c", comando)
		res, err := shell.Output()
		if comando == "exit" {
			fmt.Println("Saliendo del shell ... ");
			break;
		}
		/* --- comando ejecutado ---    */
		// res := "ejecutado OK"
		fmt.Println("Oper# ",string(res),"\n ERROR ",err);
	}
}