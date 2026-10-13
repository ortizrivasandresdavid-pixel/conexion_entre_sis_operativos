package main

import (
	"fmt"
	"os"
)
func main() 
{
	nomArchivo = "oper.user.db"
	creaArchivo(nomArchivo)
	//leerArchivo(nomArchivo)
	//modiArArchivo(nomArchivo)
}

func crearArchivo(nombre string) 
{
	credUsuario := "Aortiz:caf90169eefa5f807d577486b9f795ab86ae2983c5c20806cff959117e90af18"
	os.WriteFile(nomArchivo, []byte(credUsuario),8777)
}
func leerArchivo(nombre string)
{

}
func modiArArchivo(nombre string)
{

}