package main

import (
	"fmt"
	"os"
	"strings"



)
func main() {
	nomArchivo := "oper.user.db"
	//falta colcar un switch case para ejecutar las funciones
	fmt.Print("ACHIVOs OPER\n1.crear\n2.leer\n3.modificar\nDigite opcion : ")
	var opcion int
	fmt.Scan(&opcion)
	switch opcion {
	case 1:
		crearArchivo(nomArchivo)
	case 2:
		leerArchivo(nomArchivo)
	case 3:
		modiArArchivo(nomArchivo)
	default:
		fmt.Println("opcion inválida")
	}
	//crearArchivo(nomArchivo)
	//leerArchivo(nomArchivo)
	//modiArArchivo(nomArchivo)
}

func crearArchivo(nomArchivo string) {
	credUsuario := "Aortiz:caf90169eefa5f807d577486b9f795ab86ae2983c5c20806cff959117e90af18"
	os.WriteFile(nomArchivo, []byte(credUsuario), 0777)
}

func leerArchivo(nomArchivo string){
	archivo, _ := os.ReadFile(nomArchivo)
	strArchivo := string(archivo)
	//fmt.Println(strArchivo)
	usuarios := strings.Split(strArchivo, "\n")
	for i := 0; i < len(usuarios); i++ {
        fmt.Println("Usuario[",i, "] = ", usuarios[i])
    }

	

}

func modiArArchivo(nomArchivo string){
	//nuevoUsuario := "\nandres:0a92efb1b91ac02c858ab205fbb6baf44d67e8d1e625600a11020cfae50065da"
	archivo, _ := os.OpenFile(nomArchivo, os.O_APPEND|os.O_WRONLY, 0777)
	archivo.WriteString(nuevoUsuario)
	archivo.Close()
} 