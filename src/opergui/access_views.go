package main

import (
	"errors"
	"fmt"
	"image/color"
	"net"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"operativos/src/sockets"
)

const (
	accessWindowWidth  = 820
	accessWindowHeight = 580

	connectionCardWidth  = 640
	connectionCardHeight = 400

	accessCardWidth  = 720
	accessCardHeight = 460
	accessBodyHeight = 300
)

func monoLabel(text string) *widget.Label {
	label := widget.NewLabel(text)
	label.TextStyle = fyne.TextStyle{Monospace: true}
	return label
}

func titleLabel(text string) *widget.Label {
	label := monoLabel(text)
	label.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	label.Importance = widget.HighImportance
	return label
}

func fieldLabel(text string) *widget.Label {
	return monoLabel(text)
}

func newMonoEntry(placeHolder string, password bool) *widget.Entry {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(placeHolder)
	entry.TextStyle = fyne.TextStyle{Monospace: true}
	entry.Password = password
	return entry
}

func newPrimaryButton(text string, tapped func()) *widget.Button {
	button := widget.NewButton(text, tapped)
	button.Importance = widget.HighImportance
	return button
}

// newOutlineButton imita los botones delineados en verde del diseño: un marco
// redondeado con el botón de baja importancia encima.
func newOutlineButton(text string, tapped func()) fyne.CanvasObject {
	frame := canvas.NewRectangle(color.Transparent)
	frame.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	frame.StrokeWidth = 1
	frame.CornerRadius = 4

	button := widget.NewButton(text, tapped)
	button.Importance = widget.LowImportance
	return container.NewStack(frame, button)
}

// newCard envolve el contenido en una tarjeta oscura de borde verde y tamaño
// fijo, de modo que las pestañas del Paso 2 nunca cambien de tamaño.
func newCard(content fyne.CanvasObject, size fyne.Size) fyne.CanvasObject {
	background := canvas.NewRectangle(theme.Color(theme.ColorNameButton))
	background.CornerRadius = 12
	background.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	background.StrokeWidth = 1.5

	padded := container.New(layout.NewCustomPaddedLayout(20, 20, 26, 26), content)
	card := container.NewStack(background, padded)
	return container.NewGridWrap(size, card)
}

func fixedGap(width float32) fyne.CanvasObject {
	gap := canvas.NewRectangle(color.Transparent)
	gap.SetMinSize(fyne.NewSize(width, 1))
	return gap
}

// connectionView implementa el Paso 1: configuración y validación del socket.
type connectionView struct {
	g       *gui
	root    fyne.CanvasObject
	ip      *widget.Entry
	port    *widget.Entry
	period  *widget.Entry
	status  *widget.Label
	connect *widget.Button
	busy    bool
}

func newConnectionView(g *gui) *connectionView {
	view := &connectionView{g: g}

	view.ip = newMonoEntry("100.83.168.60", false)
	view.ip.SetText(defaultServerIP)
	view.port = newMonoEntry("2306", false)
	view.port.SetText(defaultServerPort)
	view.period = newMonoEntry("5", false)
	view.period.SetText(defaultPeriod)

	view.status = monoLabel("Waiting for connection")
	view.status.Wrapping = fyne.TextWrapWord
	view.connect = newPrimaryButton("Connect to server", view.submit)

	form := container.New(layout.NewFormLayout(),
		fieldLabel("Server IP:"), view.ip,
		fieldLabel("Port:"), view.port,
		fieldLabel("Period (s):"), view.period,
	)
	statusRow := container.NewBorder(nil, nil, fieldLabel("Status:"), view.connect, view.status)

	footer := monoLabel("Fyne Toolkit v2.8.2 - Paso 1 de 2")
	footer.Importance = widget.LowImportance

	// La cabecera queda arriba y el estado con el pie se ancla abajo; el hueco
	// central mantiene la tarjeta proporcionada sin dejar un bloque vacío final.
	content := container.NewBorder(
		container.NewVBox(
			titleLabel("Connection Configuration"),
			widget.NewSeparator(),
			form,
		),
		container.NewVBox(
			widget.NewSeparator(),
			statusRow,
			widget.NewSeparator(),
			footer,
		),
		nil, nil,
		canvas.NewRectangle(color.Transparent),
	)
	view.root = container.NewCenter(newCard(content, fyne.NewSize(connectionCardWidth, connectionCardHeight)))
	return view
}

func (v *connectionView) submit() {
	if v.busy {
		return
	}
	ip := strings.TrimSpace(v.ip.Text)
	if net.ParseIP(ip) == nil {
		v.setError(fmt.Errorf("ingresa una dirección IP válida (por ejemplo 100.83.168.60)"))
		return
	}
	port, err := strconv.Atoi(strings.TrimSpace(v.port.Text))
	if err != nil || port < 1 || port > 65535 {
		v.setError(fmt.Errorf("el puerto debe ser un número entre 1 y 65535"))
		return
	}
	period, err := strconv.Atoi(strings.TrimSpace(v.period.Text))
	if err != nil || period < 1 || period > 3600 {
		v.setError(fmt.Errorf("el período debe estar entre 1 y 3600 segundos"))
		return
	}

	address := net.JoinHostPort(ip, strconv.Itoa(port))
	v.busy = true
	v.connect.Disable()
	v.status.Importance = widget.MediumImportance
	v.status.SetText("Connecting to " + address + "...")
	v.g.connect(address, period)
}

func (v *connectionView) setIdle(message string) {
	v.busy = false
	v.connect.Enable()
	if message == "" {
		message = "Waiting for connection"
	}
	v.status.Importance = widget.MediumImportance
	v.status.SetText(message)
}

func (v *connectionView) setError(err error) {
	v.busy = false
	v.connect.Enable()
	v.status.Importance = widget.DangerImportance
	v.status.SetText(err.Error())
}

// tabButton es una pestaña con marco verde; la activa usa el relleno verde del
// botón primario para distinguirse sin cambiar el tamaño del encabezado.
type tabButton struct {
	button *widget.Button
	frame  *canvas.Rectangle
	object fyne.CanvasObject
}

func newTabButton(text string, tapped func()) *tabButton {
	frame := canvas.NewRectangle(color.Transparent)
	frame.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	frame.StrokeWidth = 1
	frame.CornerRadius = 4

	button := widget.NewButton(text, tapped)
	button.Importance = widget.LowImportance
	return &tabButton{button: button, frame: frame, object: container.NewStack(frame, button)}
}

func (t *tabButton) setActive(active bool) {
	if active {
		t.button.Importance = widget.HighImportance
		t.frame.StrokeColor = theme.Color(theme.ColorNamePrimary)
	} else {
		t.button.Importance = widget.LowImportance
		t.frame.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	}
	t.button.Refresh()
	t.frame.Refresh()
}

// accessView implementa el Paso 2. Ambas pestañas comparten la misma tarjeta de
// tamaño fijo y el mismo cuerpo, por lo que cambiar de pestaña no redimensiona
// la ventana ni mueve los botones inferiores.
type accessView struct {
	g           *gui
	root        fyne.CanvasObject
	body        *fyne.Container
	loginTab    *tabButton
	registerTab *tabButton

	connected *widget.Label

	loginUser     *widget.Entry
	loginPassword *widget.Entry
	loginStatus   *widget.Label
	loginButton   *widget.Button
	loginForm     fyne.CanvasObject

	registerUser         *widget.Entry
	registerPassword     *widget.Entry
	registerConfirmation *widget.Entry
	registerStatus       *widget.Label
	registerButton       *widget.Button
	registerForm         fyne.CanvasObject

	activeTab int
	busy      bool
}

func newAccessView(g *gui) *accessView {
	view := &accessView{g: g}

	view.loginTab = newTabButton("Iniciar sesion", func() { view.switchTab(0) })
	view.registerTab = newTabButton("Registrarse", func() { view.switchTab(1) })
	view.loginTab.setActive(true)
	view.registerTab.setActive(false)

	view.connected = monoLabel("Connected to")
	view.connected.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	dot := canvas.NewCircle(theme.Color(theme.ColorNameSuccess))
	dotWrap := container.NewGridWrap(fyne.NewSize(10, 10), dot)
	connectedRow := container.NewHBox(view.connected, dotWrap, layout.NewSpacer())

	// Pestaña Iniciar sesion.
	view.loginUser = newMonoEntry("Usuario", false)
	view.loginUser.SetText("admin")
	view.loginPassword = newMonoEntry("Contraseña", true)
	view.loginStatus = monoLabel("Quedan 3 intentos de autenticacion")
	view.loginStatus.Wrapping = fyne.TextWrapWord
	loginHint := monoLabel("Autenticación por desafío SHA-256; la contraseña no se envía directamente.")
	loginHint.Wrapping = fyne.TextWrapWord
	loginHint.Importance = widget.LowImportance
	view.loginButton = newPrimaryButton("Ingresar", view.submitLogin)
	loginButtons := container.NewHBox(
		layout.NewSpacer(),
		newOutlineButton("Cambiar servidor", g.changeServer),
		fixedGap(10),
		view.loginButton,
	)
	loginContent := container.NewVBox(
		fieldLabel("Usuario"), view.loginUser,
		fieldLabel("Contraseña"), view.loginPassword,
		view.loginStatus,
		loginHint,
	)
	view.loginForm = container.NewBorder(nil, loginButtons, nil, nil, loginContent)

	// Pestaña Registrarse, con el mismo cuerpo y la misma barra de botones.
	view.registerUser = newMonoEntry("Nuevo usuario", false)
	view.registerPassword = newMonoEntry("Contraseña", true)
	view.registerConfirmation = newMonoEntry("Confirmar clave", true)
	view.registerStatus = monoLabel("Completa los campos para crear la cuenta.")
	view.registerStatus.Wrapping = fyne.TextWrapWord
	view.registerStatus.Importance = widget.LowImportance
	registerNote := monoLabel("Nota: la contraseña se hashea con SHA-256 (64 hex) antes de enviarse por el canal TCP sin cifrar.")
	registerNote.Wrapping = fyne.TextWrapWord
	registerNote.Importance = widget.LowImportance
	view.registerButton = newPrimaryButton("Crear cuenta y entrar", view.submitRegister)
	registerButtons := container.NewHBox(
		layout.NewSpacer(),
		newOutlineButton("Cambiar servidor", g.changeServer),
		fixedGap(10),
		view.registerButton,
	)
	registerContent := container.NewVBox(
		fieldLabel("Nuevo usuario"), view.registerUser,
		fieldLabel("Contraseña"), view.registerPassword,
		fieldLabel("Confirmar clave"), view.registerConfirmation,
		view.registerStatus,
		registerNote,
	)
	view.registerForm = container.NewBorder(nil, registerButtons, nil, nil, registerContent)

	// El espaciador fija la altura del cuerpo; las dos pestañas se superponen y
	// solo se muestra la activa, conservando el tamaño total de la tarjeta.
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(1, accessBodyHeight))
	view.registerForm.Hide()
	view.body = container.NewStack(view.loginForm, view.registerForm, spacer)

	tabs := container.NewGridWithColumns(2, view.loginTab.object, view.registerTab.object)
	header := container.NewVBox(tabs, widget.NewSeparator(), connectedRow)
	cardContent := container.NewBorder(header, nil, nil, nil, view.body)
	view.root = container.NewCenter(newCard(cardContent, fyne.NewSize(accessCardWidth, accessCardHeight)))

	view.switchTab(0)
	return view
}

func (v *accessView) reset(address string) {
	v.connected.SetText("Connected to " + address)
	v.busy = false

	v.loginUser.SetText("admin")
	v.loginPassword.SetText("")
	v.loginStatus.Importance = widget.MediumImportance
	v.loginStatus.SetText("Quedan 3 intentos de autenticacion")
	v.loginButton.Enable()

	v.registerUser.SetText("")
	v.registerPassword.SetText("")
	v.registerConfirmation.SetText("")
	v.registerStatus.Importance = widget.LowImportance
	v.registerStatus.SetText("Completa los campos para crear la cuenta.")
	v.registerButton.Enable()

	v.switchTab(0)
}

func (v *accessView) switchTab(index int) {
	v.activeTab = index
	if index == 1 {
		v.loginForm.Hide()
		v.registerForm.Show()
		v.loginTab.setActive(false)
		v.registerTab.setActive(true)
	} else {
		v.registerForm.Hide()
		v.loginForm.Show()
		v.registerTab.setActive(false)
		v.loginTab.setActive(true)
	}
	v.body.Refresh()
}

func (v *accessView) setBusy(busy bool) {
	v.busy = busy
	if busy {
		v.loginButton.Disable()
		v.registerButton.Disable()
		return
	}
	v.loginButton.Enable()
	v.registerButton.Enable()
}

func (v *accessView) submitLogin() {
	if v.busy {
		return
	}
	username := strings.TrimSpace(v.loginUser.Text)
	if username == "" {
		v.loginStatus.Importance = widget.DangerImportance
		v.loginStatus.SetText("Ingresa el usuario.")
		return
	}
	if v.loginPassword.Text == "" {
		v.loginStatus.Importance = widget.DangerImportance
		v.loginStatus.SetText("Ingresa la contraseña.")
		return
	}
	v.setBusy(true)
	v.loginStatus.Importance = widget.MediumImportance
	v.loginStatus.SetText("Autenticando...")
	v.g.login(username, v.loginPassword.Text)
}

func (v *accessView) submitRegister() {
	if v.busy {
		return
	}
	username := strings.TrimSpace(v.registerUser.Text)
	switch {
	case username == "":
		v.showRegisterError("Ingresa el nombre del nuevo usuario.")
		return
	case strings.Contains(username, ":"):
		v.showRegisterError("El usuario no puede contener ':'.")
		return
	case v.registerPassword.Text == "":
		v.showRegisterError("Ingresa una contraseña.")
		return
	case v.registerPassword.Text != v.registerConfirmation.Text:
		v.showRegisterError("Las contraseñas no coinciden.")
		return
	}
	v.setBusy(true)
	v.registerStatus.Importance = widget.MediumImportance
	v.registerStatus.SetText("Creando cuenta y validando registro...")
	v.g.register(username, v.registerPassword.Text, v.registerConfirmation.Text)
}

func (v *accessView) showRegisterError(message string) {
	v.registerStatus.Importance = widget.DangerImportance
	v.registerStatus.SetText(message)
}

func (v *accessView) loginFailed(err error) {
	v.setBusy(false)
	v.loginPassword.SetText("")

	var authError *sockets.AuthResultError
	if errors.As(err, &authError) {
		if authError.Code == "AUTH_LOCKED" || sockets.IsAuthLocked(err) {
			v.loginButton.Disable()
			v.loginStatus.Importance = widget.DangerImportance
			v.loginStatus.SetText("Acceso bloqueado: se agotaron los intentos. Cambia de servidor o inténtalo más tarde.")
			return
		}
		message := fmt.Sprintf("Credenciales incorrectas. Quedan %d intentos de autenticacion", authError.AttemptsRemaining)
		if authError.RetryAfterSeconds > 0 {
			message += fmt.Sprintf(". Espera %d s antes de reintentar.", authError.RetryAfterSeconds)
		} else {
			message += "."
		}
		v.loginStatus.Importance = widget.DangerImportance
		v.loginStatus.SetText(message)
		return
	}
	v.loginStatus.Importance = widget.DangerImportance
	v.loginStatus.SetText(err.Error())
}

func (v *accessView) registerFailed(err error) {
	v.setBusy(false)
	v.registerStatus.Importance = widget.DangerImportance
	if sockets.IsUserExists(err) {
		v.registerStatus.SetText("Ese usuario ya existe. Elige otro nombre.")
		return
	}
	v.registerStatus.SetText(err.Error())
}
