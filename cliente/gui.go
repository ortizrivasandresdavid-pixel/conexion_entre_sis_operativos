package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"operativos/src/sockets"
)

func runGUI() {
	program := app.New()
	program.Settings().SetTheme(theme.DarkTheme())
	window := program.NewWindow("Cliente Operativos - Universidad de los Llanos")
	window.Resize(fyne.NewSize(1100, 700))

	appCtx, cancelApp := context.WithCancel(context.Background())
	var client *sockets.Client
	connecting := false
	reportCount := 0
	terminalContent := "Cliente listo. Conéctate al servidor y escribe help para ver los comandos permitidos.\n"
	historyContent := ""

	ipEntry := widget.NewEntry()
	ipEntry.SetPlaceHolder("IP del servidor")
	ipEntry.SetText("127.0.0.1")
	portEntry := widget.NewEntry()
	portEntry.SetPlaceHolder("Puerto")
	portEntry.SetText("2306")
	periodEntry := widget.NewEntry()
	periodEntry.SetPlaceHolder("Período (s)")
	periodEntry.SetText("5")
	usernameEntry := widget.NewEntry()
	usernameEntry.SetPlaceHolder("Usuario")
	passwordEntry := widget.NewEntry()
	passwordEntry.SetPlaceHolder("Contraseña")
	passwordEntry.Password = true

	statusLabel := widget.NewLabel("DESCONECTADO")
	reportCountLabel := widget.NewLabel("Reportes recibidos: 0")
	cpuLabel := widget.NewLabel("CPU: --")
	memoryLabel := widget.NewLabel("Memoria: --")
	diskLabel := widget.NewLabel("Disco: --")
	cpuBar := widget.NewProgressBar()
	memoryBar := widget.NewProgressBar()
	diskBar := widget.NewProgressBar()
	terminal := widget.NewTextGrid()
	terminal.SetText(terminalContent)
	terminalScroll := container.NewScroll(terminal)
	history := widget.NewTextGrid()
	history.SetText(historyContent)
	historyScroll := container.NewScroll(history)

	appendTerminal := func(text string) {
		terminalContent += text
		lines := strings.Split(terminalContent, "\n")
		if len(lines) > 500 {
			terminalContent = strings.Join(lines[len(lines)-500:], "\n")
		}
		terminal.SetText(terminalContent)
		terminalScroll.ScrollToBottom()
	}
	appendHistory := func(text string) {
		historyContent += text + "\n"
		lines := strings.Split(historyContent, "\n")
		if len(lines) > 51 {
			historyContent = strings.Join(lines[len(lines)-51:], "\n")
		}
		history.SetText(historyContent)
		historyScroll.ScrollToBottom()
	}

	connectButton := widget.NewButton("Conectar", nil)
	disconnectButton := widget.NewButton("Desconectar", nil)
	disconnectButton.Disable()
	commandEntry := widget.NewEntry()
	commandEntry.SetPlaceHolder("Digite un comando de diagnóstico (ej. df -h)")
	executeButton := widget.NewButton("Ejecutar", nil)
	executeButton.Disable()

	setDisconnected := func(message string) {
		statusLabel.SetText("DESCONECTADO")
		connectButton.Enable()
		disconnectButton.Disable()
		executeButton.Disable()
		if message != "" {
			appendTerminal("\n" + message + "\n")
		}
	}

	connectButton.OnTapped = func() {
		if connecting || client != nil {
			return
		}
		ip := strings.TrimSpace(ipEntry.Text)
		if net.ParseIP(ip) == nil {
			appendTerminal("Error: la IP del servidor no es válida.\n")
			return
		}
		port, err := strconv.Atoi(strings.TrimSpace(portEntry.Text))
		if err != nil || port < 1 || port > 65535 {
			appendTerminal("Error: el puerto debe estar entre 1 y 65535.\n")
			return
		}
		period, err := strconv.Atoi(strings.TrimSpace(periodEntry.Text))
		if err != nil || period < 1 || period > 3600 {
			appendTerminal("Error: el período debe estar entre 1 y 3600 segundos.\n")
			return
		}
		username, password := strings.TrimSpace(usernameEntry.Text), passwordEntry.Text
		if username == "" || password == "" {
			appendTerminal("Error: ingresa el usuario y la contraseña.\n")
			return
		}

		connecting = true
		connectButton.Disable()
		statusLabel.SetText("CONECTANDO...")
		appendTerminal(fmt.Sprintf("Conectando a %s:%d...\n", ip, port))
		address := net.JoinHostPort(ip, strconv.Itoa(port))
		go func() {
			session, connectErr := sockets.ConnectClient(appCtx, address, period, username, password)
			fyne.Do(func() {
				connecting = false
				passwordEntry.SetText("")
				if appCtx.Err() != nil {
					if session != nil {
						session.Close()
					}
					return
				}
				if connectErr != nil {
					setDisconnected("Error de conexión: " + connectErr.Error())
					return
				}
				client = session
				statusLabel.SetText(fmt.Sprintf("CONECTADO: %s | Período: %ds", address, period))
				connectButton.Disable()
				disconnectButton.Enable()
				executeButton.Enable()
				appendTerminal("Autenticación correcta. Escribe help para ver los diagnósticos permitidos.\n")
				go receiveGUIEvents(session, func(event sockets.ClientEvent) {
					fyne.Do(func() {
						if client != session {
							return
						}
						if event.Err != nil {
							client = nil
							setDisconnected("Error de conexión: " + event.Err.Error())
							return
						}
						switch event.Message.Type {
						case "metric":
							if event.Message.Error != "" {
								appendTerminal("Error al recibir métricas: " + event.Message.Error + "\n")
								return
							}
							if event.Message.Metrics == nil {
								appendTerminal("Error: el servidor envió un reporte de métricas vacío.\n")
								return
							}
							report := event.Message.Metrics
							reportCount++
							cpuPercent := report.CPUPercent
							memoryPercent := percentage(report.MemoryUsedMB, report.MemoryTotalMB)
							diskPercent := percentage(report.DiskUsedGB, report.DiskTotalGB)
							cpuLabel.SetText(fmt.Sprintf("CPU: %.1f%% | Procesos: %d", cpuPercent, report.ActiveProcesses))
							memoryLabel.SetText(fmt.Sprintf("Memoria: %d / %d MB (%.1f%%)", report.MemoryUsedMB, report.MemoryTotalMB, memoryPercent))
							diskLabel.SetText(fmt.Sprintf("Disco: %d / %d GB (%.1f%% usado)", report.DiskUsedGB, report.DiskTotalGB, diskPercent))
							cpuBar.SetValue(clampPercent(cpuPercent))
							memoryBar.SetValue(clampPercent(memoryPercent))
							diskBar.SetValue(clampPercent(diskPercent))
							reportCountLabel.SetText(fmt.Sprintf("Reportes recibidos: %d", reportCount))
							reportTime := report.Timestamp
							if parsed, err := time.Parse(time.RFC3339, report.Timestamp); err == nil {
								reportTime = parsed.Local().Format("15:04:05")
							}
							appendHistory(fmt.Sprintf("[%s] Reporte #%d recibido", reportTime, reportCount))
						case "command_result":
							if event.Message.Output != "" {
								output := event.Message.Output
								if !strings.HasSuffix(output, "\n") {
									output += "\n"
								}
								appendTerminal(output)
							}
							if event.Message.Error != "" {
								appendTerminal("Error: " + event.Message.Error + "\n")
							}
							if event.Message.Output == "Sesión cerrada" && event.Message.OK {
								client.Close()
								client = nil
								setDisconnected("Sesión cerrada.")
							}
						default:
							appendTerminal(fmt.Sprintf("Mensaje del servidor no reconocido: %s\n", event.Message.Type))
						}
					})
				})
			})
		}()
	}

	disconnectButton.OnTapped = func() {
		if client != nil {
			client.Close()
			client = nil
		}
		setDisconnected("Sesión desconectada.")
	}

	execute := func() {
		command := strings.TrimSpace(commandEntry.Text)
		if command == "" {
			return
		}
		session := client
		if session == nil {
			appendTerminal("Error: primero conéctate al servidor.\n")
			return
		}
		appendTerminal("$ " + command + "\n")
		commandEntry.SetText("")
		go func() {
			if err := session.SendCommand(command); err != nil {
				fyne.Do(func() {
					if client == session {
						appendTerminal("Error al enviar el comando: " + err.Error() + "\n")
					}
				})
			}
		}()
	}
	executeButton.OnTapped = execute
	commandEntry.OnSubmitted = func(string) { execute() }

	connectionForm := container.NewGridWithColumns(7,
		ipEntry, portEntry, periodEntry, usernameEntry, passwordEntry, connectButton, disconnectButton,
	)
	header := container.NewHBox(
		widget.NewLabel("Cliente TCP de Sistemas Operativos"),
		widget.NewLabel(" | "),
		statusLabel,
		widget.NewLabel(" | "),
		reportCountLabel,
	)
	resourcePanel := container.NewVBox(
		widget.NewLabel("MONITORIZACIÓN DE RECURSOS (Servidor Linux)"),
		cpuLabel, cpuBar,
		memoryLabel, memoryBar,
		diskLabel, diskBar,
		widget.NewSeparator(),
		widget.NewLabel("Historial de reportes"),
		historyScroll,
	)
	mainArea := container.NewHSplit(terminalScroll, resourcePanel)
	mainArea.SetOffset(0.68)
	commandBar := container.NewBorder(nil, nil, nil, executeButton, commandEntry)
	window.SetContent(container.NewBorder(
		container.NewVBox(header, connectionForm, widget.NewSeparator()),
		commandBar,
		nil, nil,
		mainArea,
	))
	window.SetOnClosed(func() {
		cancelApp()
		if client != nil {
			client.Close()
		}
	})
	window.ShowAndRun()
}

func receiveGUIEvents(client *sockets.Client, handle func(sockets.ClientEvent)) {
	for event := range client.Events() {
		handle(event)
	}
}

func percentage(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) / float64(total) * 100
}

func clampPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
