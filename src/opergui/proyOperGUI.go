package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"operativos/src/metrics"
	"operativos/src/sockets"
)

const (
	defaultServerIP     = "100.83.168.60"
	defaultServerPort   = "2306"
	defaultPeriod       = "5"
	mainWindowWidth     = 1180
	mainWindowHeight    = 760
	maxReportHistory    = 20
	reportPeriodDefault = 5
)

type gui struct {
	app    fyne.App
	window fyne.Window
	ctx    context.Context
	cancel context.CancelFunc

	connection *connectionView
	access     *accessView
	main       *mainView

	accessConn *sockets.AccessConnection
	session    *sockets.ClientSession
	address    string
	username   string
	period     int
}

func main() {
	program := app.NewWithID("operativos.gui")
	program.Settings().SetTheme(newTerminalTheme())
	window := program.NewWindow("Cliente Operativos - Universidad de los Llanos")
	appContext, cancelApp := context.WithCancel(context.Background())

	gui := &gui{
		app:    program,
		window: window,
		ctx:    appContext,
		cancel: cancelApp,
		period: reportPeriodDefault,
	}
	gui.connection = newConnectionView(gui)
	gui.access = newAccessView(gui)
	gui.main = newMainView(gui)
	gui.showConnection("")

	window.SetOnClosed(func() {
		cancelApp()
		gui.closeTransport()
	})
	window.ShowAndRun()
}

func (g *gui) showConnection(status string) {
	g.closeTransport()
	g.window.SetTitle("System Monitor Client - Step 1: Server Connection")
	g.window.SetContent(g.connection.root)
	g.window.Resize(fyne.NewSize(accessWindowWidth, accessWindowHeight))
	g.window.CenterOnScreen()
	g.connection.setIdle(status)
}

func (g *gui) showAccess(accessConn *sockets.AccessConnection, address string) {
	g.accessConn = accessConn
	g.address = address
	g.access.reset(address)
	g.window.SetTitle("Step 2 User Access")
	g.window.SetContent(g.access.root)
	g.window.Resize(fyne.NewSize(accessWindowWidth, accessWindowHeight))
	g.window.CenterOnScreen()
	g.window.Canvas().Focus(g.access.loginUser)
}

func (g *gui) connect(address string, period int) {
	go func() {
		accessConn, err := sockets.ConnectAccess(g.ctx, address)
		fyne.Do(func() {
			if g.ctx.Err() != nil {
				if accessConn != nil {
					_ = accessConn.Close()
				}
				return
			}
			if err != nil {
				g.connection.setError(err)
				return
			}
			g.period = period
			g.showAccess(accessConn, address)
		})
	}()
}

func (g *gui) login(username, password string) {
	accessConn := g.accessConn
	if accessConn == nil {
		g.access.loginFailed(fmt.Errorf("la conexión con el servidor ya no está activa; cambia de servidor"))
		return
	}
	go func() {
		session, err := accessConn.Login(username, password, g.period)
		fyne.Do(func() {
			if accessConn != g.accessConn {
				if session != nil {
					session.Close()
				}
				return
			}
			if err != nil {
				if errors.Is(err, sockets.ErrAccessClosed) || errors.Is(err, context.Canceled) {
					g.accessConn = nil
					g.showConnection("La conexión con el servidor se cerró. Vuelve a conectar.")
					return
				}
				g.access.loginFailed(err)
				return
			}
			g.accessConn = nil
			g.username = username
			g.startSession(session)
		})
	}()
}

func (g *gui) register(username, password, confirmation string) {
	accessConn := g.accessConn
	if accessConn == nil {
		g.access.registerFailed(fmt.Errorf("la conexión con el servidor ya no está activa; cambia de servidor"))
		return
	}
	go func() {
		session, err := accessConn.Register(username, password, confirmation, g.period)
		fyne.Do(func() {
			if accessConn != g.accessConn {
				if session != nil {
					session.Close()
				}
				return
			}
			if err != nil {
				if errors.Is(err, sockets.ErrAccessClosed) || errors.Is(err, context.Canceled) {
					g.accessConn = nil
					g.showConnection("La conexión con el servidor se cerró. Vuelve a conectar.")
					return
				}
				g.access.registerFailed(err)
				return
			}
			g.accessConn = nil
			g.username = username
			g.startSession(session)
		})
	}()
}

func (g *gui) changeServer() {
	g.closeTransport()
	g.showConnection("Waiting for connection")
}

func (g *gui) startSession(session *sockets.ClientSession) {
	g.session = session
	g.main.reset(g.address, g.username, g.period)
	g.window.SetTitle("Cliente Operativos - Terminal y monitorización")
	g.window.SetContent(g.main.root)
	g.window.Resize(fyne.NewSize(mainWindowWidth, mainWindowHeight))
	g.window.CenterOnScreen()
	g.window.Canvas().Focus(g.main.commandInput)
	g.main.listen(session)
}

func (g *gui) sessionEnded(session *sockets.ClientSession, message string) {
	if g.session != session {
		return
	}
	g.session = nil
	session.Close()
	g.showConnection(message)
}

func (g *gui) closeTransport() {
	if g.session != nil {
		g.session.Close()
		g.session = nil
	}
	if g.accessConn != nil {
		_ = g.accessConn.Close()
		g.accessConn = nil
	}
}

type mainView struct {
	g              *gui
	root           fyne.CanvasObject
	status         *widget.Label
	changeServer   *widget.Button
	terminal       *widget.Label
	terminalScroll *container.Scroll
	history        *widget.Label
	historyScroll  *container.Scroll
	commandInput   *widget.Entry
	executeButton  *widget.Button
	cpuLabel       *widget.Label
	cpuBar         *widget.ProgressBar
	memoryLabel    *widget.Label
	memoryBar      *widget.ProgressBar
	diskLabel      *widget.Label
	diskBar        *widget.ProgressBar
	reportCount    *widget.Label

	pendingCommandID uint64
	pendingCommand   string
	currentDir       string
	reportTotal      int
	reportHistory    []string
	terminalOutput   string
}

func newMainView(g *gui) *mainView {
	m := &mainView{g: g}

	m.status = monoLabel("ONLINE")
	m.status.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	m.status.Importance = widget.HighImportance
	m.changeServer = widget.NewButton("Cambiar servidor", g.changeServer)

	m.terminal = monoLabel("")
	m.terminal.Wrapping = fyne.TextWrapWord
	m.terminalScroll = container.NewVScroll(m.terminal)
	m.history = monoLabel("")
	m.history.Wrapping = fyne.TextWrapWord
	m.historyScroll = container.NewVScroll(m.history)

	m.cpuLabel = monoLabel("CPU: --")
	m.cpuBar = widget.NewProgressBar()
	m.cpuBar.Max = 100
	m.memoryLabel = monoLabel("Memoria: --")
	m.memoryBar = widget.NewProgressBar()
	m.memoryBar.Max = 100
	m.diskLabel = monoLabel("Disco: --")
	m.diskBar = widget.NewProgressBar()
	m.diskBar.Max = 100
	m.reportCount = monoLabel("Reportes recibidos: 0")

	m.commandInput = newMonoEntry("Escribe un comando de Linux y presiona Enter (p. ej. ls -la)", false)
	m.commandInput.Disable()
	m.commandInput.OnSubmitted = func(string) { m.sendCommand() }
	m.executeButton = widget.NewButton("Ejecutar", m.sendCommand)
	m.executeButton.Disable()

	topBar := container.NewPadded(container.NewBorder(
		nil, nil,
		m.status,
		m.changeServer,
	))
	terminalPanel := container.NewBorder(titleLabel("Terminal"), nil, nil, nil, m.terminalScroll)
	resourceHeader := container.NewVBox(
		titleLabel("MONITORIZACIÓN DE RECURSOS (Servidor Linux)"),
		m.cpuLabel,
		m.cpuBar,
		m.memoryLabel,
		m.memoryBar,
		m.diskLabel,
		m.diskBar,
		widget.NewSeparator(),
		m.reportCount,
		monoLabel("Historial de reportes"),
	)
	resourcePanel := container.NewBorder(resourceHeader, nil, nil, nil, m.historyScroll)
	split := container.NewHSplit(terminalPanel, resourcePanel)
	split.SetOffset(0.7)

	commandBar := container.NewBorder(nil, nil, nil, m.executeButton, m.commandInput)
	body := container.NewBorder(nil, commandBar, nil, nil, split)
	m.root = container.NewBorder(topBar, nil, nil, nil, body)
	return m
}

func (m *mainView) reset(address, username string, period int) {
	m.terminalOutput = ""
	m.reportHistory = nil
	m.reportTotal = 0
	m.pendingCommandID = 0
	m.pendingCommand = ""
	m.currentDir = ""
	m.terminal.SetText("")
	m.history.SetText("")
	m.reportCount.SetText("Reportes recibidos: 0")
	m.cpuLabel.SetText("CPU: --")
	m.cpuBar.SetValue(0)
	m.memoryLabel.SetText("Memoria: --")
	m.memoryBar.SetValue(0)
	m.diskLabel.SetText("Disco: --")
	m.diskBar.SetValue(0)
	m.commandInput.SetText("")
	m.commandInput.Enable()
	m.executeButton.Enable()
	m.status.SetText(fmt.Sprintf("ONLINE | %s@%s | período: %ds", username, address, period))
	m.appendTerminal("Autenticación correcta. Escribe cualquier comando de Linux; 'bye' para salir.")
}

func (m *mainView) listen(session *sockets.ClientSession) {
	go func() {
		for event := range session.Events() {
			current := event
			fyne.Do(func() {
				m.handleEvent(session, current)
			})
		}
		fyne.Do(func() {
			m.g.sessionEnded(session, "La conexión con el servidor se cerró.")
		})
	}()
}

func (m *mainView) handleEvent(session *sockets.ClientSession, event sockets.ClientEvent) {
	if m.g.session != session {
		return
	}
	if event.Err != nil {
		if errors.Is(event.Err, io.EOF) {
			m.g.sessionEnded(session, "El servidor cerró la sesión.")
		} else {
			m.g.sessionEnded(session, "Conexión cerrada: "+event.Err.Error())
		}
		return
	}

	message := event.Message
	switch message.Type {
	case "metric":
		if message.Error != "" {
			timestamp := time.Now().Format("15:04:05")
			m.appendReport(fmt.Sprintf("[%s] Error al leer métricas: %s", timestamp, message.Error))
			return
		}
		m.applyMetrics(message.Metrics)
	case "command_result":
		if message.Output != "" {
			m.appendTerminal(strings.TrimRight(message.Output, "\n"))
		}
		if message.ExitCode != 0 {
			m.appendTerminal(fmt.Sprintf("[código de salida: %d]", message.ExitCode))
		}
		if message.Error != "" {
			m.appendTerminal("Error del servidor: " + message.Error)
		}
		if message.Dir != "" {
			m.currentDir = message.Dir
		}
		if message.ID != m.pendingCommandID {
			return
		}
		wasBye := m.pendingCommand == "bye"
		m.pendingCommandID = 0
		m.pendingCommand = ""
		if wasBye {
			m.g.sessionEnded(session, "SESIÓN CERRADA")
			return
		}
		m.commandInput.Enable()
		m.executeButton.Enable()
		m.g.window.Canvas().Focus(m.commandInput)
	default:
		m.appendTerminal("Mensaje no reconocido del servidor: " + message.Type)
	}
}

func (m *mainView) sendCommand() {
	if m.g.session == nil || m.pendingCommandID != 0 {
		return
	}
	command := strings.TrimSpace(m.commandInput.Text)
	if command == "" {
		return
	}
	id, err := m.g.session.SendCommand(command)
	if err != nil {
		m.appendTerminal("Error al enviar el comando: " + err.Error())
		m.g.sessionEnded(m.g.session, "DESCONECTADO")
		return
	}
	prompt := "$"
	if m.currentDir != "" {
		prompt = m.currentDir + " $"
	}
	m.appendTerminal(prompt + " " + command)
	m.pendingCommandID = id
	m.pendingCommand = command
	m.commandInput.SetText("")
	m.commandInput.Disable()
	m.executeButton.Disable()
}

func (m *mainView) appendTerminal(line string) {
	if m.terminalOutput != "" && !strings.HasSuffix(m.terminalOutput, "\n") {
		m.terminalOutput += "\n"
	}
	m.terminalOutput += line
	m.terminal.SetText(m.terminalOutput)
	m.terminalScroll.ScrollToBottom()
}

func (m *mainView) appendReport(line string) {
	m.reportHistory = append(m.reportHistory, line)
	if len(m.reportHistory) > maxReportHistory {
		m.reportHistory = m.reportHistory[len(m.reportHistory)-maxReportHistory:]
	}
	m.history.SetText(strings.Join(m.reportHistory, "\n"))
	m.historyScroll.ScrollToBottom()
}

func (m *mainView) applyMetrics(snapshot *metrics.Snapshot) {
	if snapshot == nil {
		return
	}
	cpu := clampPercent(snapshot.CPUPercent)
	memoryPercent := percent(snapshot.MemoryUsedMB, snapshot.MemoryTotalMB)
	diskPercent := percent(snapshot.DiskUsedGB, snapshot.DiskTotalGB)
	m.cpuLabel.SetText(fmt.Sprintf("CPU: %.1f%%", cpu))
	m.cpuBar.SetValue(cpu)
	m.memoryLabel.SetText(fmt.Sprintf(
		"Memoria: %.1f%% (%d / %d MB)",
		memoryPercent,
		snapshot.MemoryUsedMB,
		snapshot.MemoryTotalMB,
	))
	m.memoryBar.SetValue(memoryPercent)
	m.diskLabel.SetText(fmt.Sprintf(
		"Disco: %.1f%% usado (%d / %d GB)",
		diskPercent,
		snapshot.DiskUsedGB,
		snapshot.DiskTotalGB,
	))
	m.diskBar.SetValue(diskPercent)

	m.reportTotal++
	m.reportCount.SetText(fmt.Sprintf("Reportes recibidos: %d", m.reportTotal))
	timestamp := snapshot.Timestamp
	if timestamp == "" {
		timestamp = time.Now().Format("15:04:05")
	}
	m.appendReport(fmt.Sprintf("[%s] Reporte #%d recibido", timestamp, m.reportTotal))
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
