package main

import (
	"fyne.io/fyne/v2"
    "fyne.io/fyne/v2/app"
    "fyne.io/fyne/v2/container"
    "fyne.io/fyne/v2/widget"
	"fmt"
	"time"

)
func main() {
	prog := app.New()
	ventPal := prog.NewWindow("Proyecto Sistemas Operativos")

	textComando := widget.NewEntry()
	labelRes := widget.NewLabel("Esperando comando...")
	labelContador := widget.NewLabel("Contador: 0")
	barraDisco := widget.NewProgressBar()
	barraDisco.Max = 100
	show := func(s string) {
		labelRes.SetText("Res: " + s )
	}

	btn := widget.NewButton("Ejecutar", func() {
		show(textComando.Text)	
	})

	ventPal.SetContent(container.NewVBox(
		textComando,
		btn,
		labelRes,
		labelContador,
		barraDisco,
	))
	ventPal.Resize(fyne.NewSize(420, 200))

	go recReporte(labelContador, barraDisco)

	ventPal.ShowAndRun()
}
func recReporte(labelContador *widget.Label, barraDisco *widget.ProgressBar) {
	contador := 0
	for {
		contador++
		time.Sleep(3 * time.Second)
		fyne.Do(func(){
			labelContador.SetText(fmt.Sprintf("Reporte: %d - RAM 34", contador))
		})
		valorBarra := float64(contador % 100)
		barraDisco.SetValue(valorBarra)
	}
}




