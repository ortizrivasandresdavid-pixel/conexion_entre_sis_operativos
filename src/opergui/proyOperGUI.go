package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"operativos/src/metrics"
	"operativos/src/sockets"
)

func main() {
	program := app.NewWithID("operativos.gui")
	program.Settings().SetTheme(newTerminalTheme())
	window := program.NewWindow("Cliente Operativos - Universidad de los Llanos")
	appContext, cancelApp := context.WithCancel(context.Background())

	serverIP := widget.NewEntry()
	serverIP.SetPlaceHolder("IP del servidor")
	portInput := widget.NewEntry()
	portInput.SetText("2306")
	periodInput := widget.NewEntry()
	periodInput.SetText("5")
	usernameInput := widget.NewEntry()
	usernameInput.SetPlaceHolder("Usuario")
	passwordInput := widget.NewEntry()
	passwordInput.SetPlaceHolder("Contraseña")
	passwordInput.Password = true

	connectionStatus := widget.NewLabel("DESCONECTADO")
	reportCount := widget.NewLabel("Reportes recibidos: 0")
	terminal := widget.NewLabel("")
	terminal.Wrapping = fyne.TextWrapWord
	terminal.TextStyle = fyne.TextStyle{Monospace: true}
	history := widget.NewLabel("")
	history.Wrapping = fyne.TextWrapWord
	history.TextStyle = fyne.TextStyle{Monospace: true}
	terminalScroll := container.NewVScroll(terminal)
	historyScroll := container.NewVScroll(history)

	cpuLabel := widget.NewLabel("CPU: --")
	cpuBar := widget.NewProgressBar()
	cpuBar.Max = 100
	memoryLabel := widget.NewLabel("Memoria: --")
	memoryBar := widget.NewProgressBar()
	memoryBar.Max = 100
	diskLabel := widget.NewLabel("Disco: --")
	diskBar := widget.NewProgressBar()
	diskBar.Max = 100

	commandInput := widget.NewEntry()
	commandInput.SetPlaceHolder("Escribe un comando de Linux y presiona Enter (p. ej. ls -la)")
	commandInput.Disable()
	executeButton := widget.NewButton("Ejecutar", nil)
	executeButton.Disable()
	connectButton := widget.NewButton("Conectar", nil)

	var activeSession *sockets.ClientSession
	var pendingCommandID uint64
	var pendingCommand string
	var currentDir string
	var reportTotal int
	var reportHistory []string
	var terminalOutput string

	appendReport := func(line string) {
		reportHistory = append(reportHistory, line)
		if len(reportHistory) > 20 {
			reportHistory = reportHistory[len(reportHistory)-20:]
		}
		history.SetText(strings.Join(reportHistory, "\n"))
		historyScroll.ScrollToBottom()
	}

	appendTerminal := func(line string) {
		fyne.Do(func() {
			if terminalOutput != "" && !strings.HasSuffix(terminalOutput, "\n") {
				terminalOutput += "\n"
			}
			terminalOutput += line
			terminal.SetText(terminalOutput)
			terminalScroll.ScrollToBottom()
		})
	}

	setDisconnected := func(message string) {
		if activeSession != nil {
			activeSession.Close()
			activeSession = nil
		}
		pendingCommandID = 0
		pendingCommand = ""
		currentDir = ""
		commandInput.Disable()
		executeButton.Disable()
		serverIP.Enable()
		portInput.Enable()
		periodInput.Enable()
		usernameInput.Enable()
		passwordInput.Enable()
		connectButton.Enable()
		connectionStatus.SetText(message)
	}

	var sendCommand func()
	sendCommand = func() {
		if activeSession == nil || pendingCommandID != 0 {
			return
		}
		command := strings.TrimSpace(commandInput.Text)
		if command == "" {
			return
		}
		id, err := activeSession.SendCommand(command)
		if err != nil {
			appendTerminal("Error al enviar el comando: " + err.Error())
			setDisconnected("DESCONECTADO")
			return
		}
		prompt := "$"
		if currentDir != "" {
			prompt = currentDir + " $"
		}
		appendTerminal(prompt + " " + command)
		pendingCommandID = id
		pendingCommand = command
		commandInput.SetText("")
		commandInput.Disable()
		executeButton.Disable()
	}
	executeButton.OnTapped = sendCommand
	commandInput.OnSubmitted = func(string) {
		sendCommand()
	}

	applyMetrics := func(snapshot *metrics.Snapshot) {
		if snapshot == nil {
			return
		}
		cpu := clampPercent(snapshot.CPUPercent)
		memoryPercent := percent(snapshot.MemoryUsedMB, snapshot.MemoryTotalMB)
		diskPercent := percent(snapshot.DiskUsedGB, snapshot.DiskTotalGB)
		cpuLabel.SetText(fmt.Sprintf("CPU: %.1f%%", cpu))
		cpuBar.SetValue(cpu)
		memoryLabel.SetText(fmt.Sprintf(
			"Memoria: %.1f%% (%d / %d MB)",
			memoryPercent,
			snapshot.MemoryUsedMB,
			snapshot.MemoryTotalMB,
		))
		memoryBar.SetValue(memoryPercent)
		diskLabel.SetText(fmt.Sprintf(
			"Disco: %.1f%% usado (%d / %d GB)",
			diskPercent,
			snapshot.DiskUsedGB,
			snapshot.DiskTotalGB,
		))
		diskBar.SetValue(diskPercent)

		reportTotal++
		reportCount.SetText(fmt.Sprintf("Reportes recibidos: %d", reportTotal))
		timestamp := snapshot.Timestamp
		if timestamp == "" {
			timestamp = time.Now().Format("15:04:05")
		}
		appendReport(fmt.Sprintf("[%s] Reporte #%d recibido", timestamp, reportTotal))
	}

	readSession := func(session *sockets.ClientSession) {
		go func() {
			for event := range session.Events() {
				currentEvent := event
				fyne.Do(func() {
					if activeSession != session {
						return
					}
					if currentEvent.Err != nil {
						if errors.Is(currentEvent.Err, io.EOF) {
							setDisconnected("DESCONECTADO")
						} else {
							appendTerminal("Conexión cerrada: " + currentEvent.Err.Error())
							setDisconnected("DESCONECTADO")
						}
						return
					}
					message := currentEvent.Message
					switch message.Type {
					case "metric":
						if message.Error != "" {
							timestamp := time.Now().Format("15:04:05")
							appendReport(fmt.Sprintf("[%s] Error al leer métricas: %s", timestamp, message.Error))
						} else {
							applyMetrics(message.Metrics)
						}
					case "command_result":
						if message.Output != "" {
							appendTerminal(strings.TrimRight(message.Output, "\n"))
						}
						if message.ExitCode != 0 {
							appendTerminal(fmt.Sprintf("[código de salida: %d]", message.ExitCode))
						}
						if message.Error != "" {
							appendTerminal("Error del servidor: " + message.Error)
						}
						if message.Dir != "" {
							currentDir = message.Dir
						}
						if message.ID == pendingCommandID {
							wasBye := pendingCommand == "bye"
							pendingCommandID = 0
							pendingCommand = ""
							if wasBye {
								setDisconnected("SESIÓN CERRADA")
								return
							}
							commandInput.Enable()
							executeButton.Enable()
							window.Canvas().Focus(commandInput)
						}
					default:
						appendTerminal("Mensaje no reconocido del servidor: " + message.Type)
					}
				})
			}
			fyne.Do(func() {
				if activeSession == session && appContext.Err() == nil {
					setDisconnected("DESCONECTADO")
				}
			})
		}()
	}

	connectButton.OnTapped = func() {
		ip := strings.TrimSpace(serverIP.Text)
		if net.ParseIP(ip) == nil {
			dialog.ShowError(fmt.Errorf("ingresa una dirección IP válida del servidor"), window)
			return
		}
		port, err := strconv.Atoi(strings.TrimSpace(portInput.Text))
		if err != nil || port < 1 || port > 65535 {
			dialog.ShowError(fmt.Errorf("el puerto debe estar entre 1 y 65535"), window)
			return
		}
		period, err := strconv.Atoi(strings.TrimSpace(periodInput.Text))
		if err != nil || period < 1 || period > 3600 {
			dialog.ShowError(fmt.Errorf("el período debe estar entre 1 y 3600 segundos"), window)
			return
		}
		username := strings.TrimSpace(usernameInput.Text)
		if username == "" {
			dialog.ShowError(fmt.Errorf("ingresa el usuario"), window)
			return
		}

		address := net.JoinHostPort(ip, strconv.Itoa(port))
		password := passwordInput.Text
		connectButton.Disable()
		connectionStatus.SetText("CONECTANDO...")
		appendTerminal("Conectando a " + address + "...")
		go func() {
			session, err := sockets.ConnectClient(appContext, address, period, username, password)
			if appContext.Err() != nil {
				if session != nil {
					session.Close()
				}
				return
			}
			fyne.Do(func() {
				if err != nil {
					if sockets.IsAuthLocked(err) {
						serverIP.Disable()
						portInput.Disable()
						periodInput.Disable()
						usernameInput.Disable()
						passwordInput.Disable()
						connectButton.Disable()
						commandInput.Disable()
						executeButton.Disable()
						connectionStatus.SetText("ACCESO BLOQUEADO")
						notice := dialog.NewInformation(
							"Acceso denegado",
							"Acceso denegado: se ha agotado el número máximo de intentos permitidos. La aplicación se cerrará.",
							window,
						)
						notice.SetOnClosed(program.Quit)
						notice.Show()
						return
					}
					appendTerminal("Error de conexión: " + err.Error())
					setDisconnected("DESCONECTADO")
					dialog.ShowError(err, window)
					return
				}
				activeSession = session
				passwordInput.SetText("")
				serverIP.Disable()
				portInput.Disable()
				periodInput.Disable()
				usernameInput.Disable()
				passwordInput.Disable()
				connectButton.Disable()
				commandInput.Enable()
				executeButton.Enable()
				connectionStatus.SetText(fmt.Sprintf("ONLINE | %s | período: %ds", address, period))
				appendTerminal("Autenticación correcta. Escribe cualquier comando de Linux; 'bye' para salir.")
				window.Canvas().Focus(commandInput)
				readSession(session)
			})
		}()
	}

	connectionFields := container.NewGridWithColumns(3,
		container.NewVBox(widget.NewLabel("IP del servidor"), serverIP),
		container.NewVBox(widget.NewLabel("Puerto"), portInput),
		container.NewVBox(widget.NewLabel("Período (s)"), periodInput),
	)
	credentialFields := container.NewGridWithColumns(3,
		container.NewVBox(widget.NewLabel("Usuario"), usernameInput),
		container.NewVBox(widget.NewLabel("Contraseña"), passwordInput),
		container.NewVBox(widget.NewLabel(""), connectButton),
	)
	connectionPanel := container.NewVBox(connectionFields, credentialFields, connectionStatus)

	resourceHeader := container.NewVBox(
		widget.NewLabel("MONITORIZACIÓN DE RECURSOS (Servidor Linux)"),
		cpuLabel,
		cpuBar,
		memoryLabel,
		memoryBar,
		diskLabel,
		diskBar,
		widget.NewSeparator(),
		reportCount,
		widget.NewLabel("Historial de reportes"),
	)
	resourcePanel := container.NewBorder(resourceHeader, nil, nil, nil, historyScroll)
	split := container.NewHSplit(
		container.NewBorder(widget.NewLabel("Terminal"), nil, nil, nil, terminalScroll),
		resourcePanel,
	)
	split.SetOffset(0.7)
	commandBar := container.NewBorder(nil, nil, nil, executeButton, commandInput)
	window.SetContent(container.NewBorder(connectionPanel, commandBar, nil, nil, split))
	window.Resize(fyne.NewSize(1100, 720))
	window.SetOnClosed(func() {
		cancelApp()
		if activeSession != nil {
			activeSession.Close()
		}
	})
	window.ShowAndRun()
}

func percent(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return clampPercent(float64(used) * 100 / float64(total))
}

func clampPercent(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
