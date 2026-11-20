# Contexto del proyecto

## 1. Propósito general del proyecto

Proyecto universitario de sistemas operativos que implementa un canal cliente-servidor para administrar y monitorizar un equipo Debian/Linux desde un cliente Windows. El servidor escucha conexiones TCP, autoriza la dirección IP del cliente, autentica usuarios mediante un archivo plano de hashes SHA-256 y, tras autenticar, permite ejecutar comandos remotos y recibir reportes periódicos de recursos.

El cliente está disponible como aplicación de consola y como interfaz gráfica para Windows desarrollada con Fyne. La GUI presenta una terminal, el estado de conexión y métricas de CPU, memoria y disco. El intercambio de mensajes ocurre concurrentemente, sin bloquear la interfaz mientras se reciben respuestas o reportes.

El servidor incluye un monitor CLI independiente en `src/servercli`. Sigue los eventos JSONL y recopila las métricas locales sin abrir el puerto de clientes ni controlar el servicio systemd; usa solo el módulo principal y no depende de Fyne ni CGo.

El proyecto tiene fines académicos y no sustituye SSH para administración de sistemas. El protocolo propio usa TCP sin cifrado y el servidor ejecuta comandos reales con los permisos de la cuenta autenticada; por tanto, debe usarse únicamente en una red confiable/VPN y con usuarios Linux de privilegios limitados. El proceso daemon se ejecuta como root para provisionar cuentas y asignar UID/GID a los procesos hijos.

## 2. Estructura de directorios clave

### Raíz

- `README.md`: guía de uso y referencia principal. Su inicio contiene el flujo con ejecutables: preparación Debian/Windows, Tailscale, instalación del servidor, GUI cliente, CLI cliente y monitor CLI del servidor. También documenta configuración, seguridad, compilación, systemd y diagnóstico.
- `GUIA_EXPOSICION.md`: guía didáctica de arquitectura, recorrido por componentes, conceptos de Sistemas Operativos, auditoría técnica y preguntas de sustentación.
- `go.mod` / `go.sum`: módulo principal `operativos`, versión declarada de Go 1.27.3 y dependencias del cliente de consola.
- `instalar-servidor.sh`: instalador automatizado de producción. Desde la raíz prepara directorios, compila con Go, instala los archivos en `/etc/`, provisiona usuarios iniciales como cuentas Linux y habilita/reinicia la unidad systemd. Detiene una instancia activa, por lo que puede cortar conexiones; incluye `trap` de errores y pausa final.
- `iniciar-servidor-debian.sh`: comprueba la versión de Go, elige la configuración de producción o de desarrollo y ejecuta el servidor.
- `iniciar-cli-servidor.sh`: ejecuta el monitor CLI pasivo en una terminal separada; no abre el puerto del servidor ni controla systemd.
- `preparar-debian.sh`: prepara Go/Tailscale en Debian; la autenticación de Tailscale se completa aparte.
- `preparar-windows.bat` / `preparar-windows.ps1`: prepara Go, Tailscale y CGo/GCC, y compila `dist\windows\cliente.exe` y `dist\windows\cliente-gui.exe`.

### `cliente/`

- `main.go`: entrada del cliente de consola. Exige exactamente tres argumentos (`IP`, `puerto`, `período`), valida que el período sea un entero mayor que 0 y que no supere 3600 segundos, muestra el uso y termina con código 1 si faltan o son inválidos; después gestiona señales de terminación y delega la sesión a `src/sockets`.
- `main_test.go`: pruebas del parseo de argumentos, incluyendo faltantes, IP/puerto inválidos, períodos cero/negativos/no numéricos y argumentos adicionales.

### `servidor/`

- `main.go`: entrada del servidor. Lee opciones de línea de comandos, carga y valida el archivo `.conf`, configura el logger y arranca/detiene el servicio según las señales del sistema.
- `server.conf`: configuración de desarrollo; define dirección/puerto, IP autorizada, límites de autenticación, archivo de usuarios, período de métricas y logs.
- `users.conf`: usuarios de ejemplo para la configuración de desarrollo; contiene hashes, no debe tratarse como almacén de credenciales de producción.
- `main_test.go`: pruebas del parseo y resolución de la configuración del servidor.

### `deploy/`

- `configProyecto.conf`: plantilla para `/etc/configProyecto.conf`, con rutas centralizadas para usuarios y logs.
- `configProyecto.users`: plantilla de usuarios para el despliegue.
- `servidor-operativos.service`: unidad systemd que ejecuta el daemon como root con capacidades acotadas, acceso a `/home`, `NoNewPrivileges=true` y `ProtectSystem=full`. La unidad no declara `ReadWritePaths` para la base de cuentas de `/etc`; ver la limitación en la sección de estado.

### `src/`

- `sockets/protocol.go`: define el formato JSON de los mensajes de aplicación (autenticación, registro, comandos, métricas) y sus operaciones de lectura/escritura sobre TCP.
- `sockets/serverOper.go`: servidor TCP; aplica restricción de IP/CIDR, autenticación y registro de usuarios, límite de conexiones, comandos, período de métricas y cierre ordenado.
- `sockets/gui_events.go`: publica eventos JSONL de sesión, comandos y reportes para que el monitor CLI los lea sin interactuar con el daemon.
- `sockets/clientOper.go`: operaciones del cliente de consola y flujo de autenticación/sesión.
- `sockets/client_session.go`: API concurrente de sesión del cliente; usa goroutine de lectura, canal de eventos, identificadores de solicitudes y escritura sincronizada.
- `sockets/client_access.go`: flujo de acceso en dos pasos para la GUI; `ConnectAccess` valida el socket y la IP, y `AccessConnection.Login`/`Register` completan el acceso antes de entregar la `ClientSession`.
- `sockets/auth_result.go`: interpretación de resultados y errores de autenticación y registro.
- `sockets/*_test.go`: pruebas del protocolo, sesiones, registro y comportamiento del servidor.
- `login/login.go`: generación y verificación de pruebas de autenticación basadas en SHA-256; la contraseña no se envía directamente por el protocolo.
- `login/archivos.go`: carga y validación del archivo plano de usuarios, y `UserStore` con `sync.RWMutex` para verificar credenciales y registrar cuentas nuevas de forma concurrente.
- `login/system_users_linux.go`: valida y provisiona cuentas Linux regulares con `useradd`, UID >= 1000 y home `/home/<usuario>`; `system_users_other.go` informa que la provisión solo está disponible en Linux. El usuario nuevo se intenta crear durante el registro y también se verifica al iniciar sesión.
- `login/attempts.go`: control de intentos fallidos y espera progresiva por dirección IP.
- `login/*_test.go`: pruebas de autenticación, lectura de usuarios y límites de intentos.
- `shell/shell.go`: ejecución de comandos mediante `bash` o `sh` bajo las credenciales de la cuenta autenticada, conservación del directorio por sesión, límite de duración y tamaño de salida.
- `shell/proc_unix.go` / `shell/proc_other.go`: comportamiento específico de plataforma para gestionar procesos y configurar UID/GID.
- `shell/shell_test.go`: pruebas de ejecución de comandos y límites.
- `metrics/metrics.go`: estructura común del reporte de recursos.
- `metrics/metrics_linux.go`: recopilación en Linux de CPU, procesos y memoria desde `/proc`, y del disco exclusivamente en la partición raíz (`/`) mediante `syscall.Statfs_t` dentro de `readRootDiskUsage`.
- `metrics/metrics_linux_test.go`: prueba Linux que valida que la medición de disco de la raíz devuelve tamaños coherentes.
- `metrics/metrics_other.go`: implementación alternativa para plataformas no Linux.
- `logging/logging.go`: logger estructurado con `log/slog`, salida a consola/archivo y rotación por tamaño.
- `logging/logging_test.go`: pruebas de escritura y rotación de logs.
- `servercli/`: monitor de terminal con actualización en vivo de estado, CPU, memoria, disco, procesos, reportes y comandos; `c` limpia las listas visibles y `q` sale. Lee el JSONL local y no detiene/reinicia el daemon.
- `opergui/`: módulo Go separado para evitar incorporar Fyne a las dependencias del servidor o de la consola. `go.mod` declara Fyne v2.8.2 y reemplaza el módulo `operativos` por la raíz del repositorio.
- `opergui/proyOperGUI.go`: aplicación Fyne; flujo de acceso en dos pasos (conexión, login/registro), terminal, historial y panel de métricas. La lectura de sesión se realiza concurrentemente y las actualizaciones visuales se envían al hilo de UI mediante `fyne.Do`.
- `opergui/access_views.go`: vistas del Paso 1 (socket) y del Paso 2 (pestañas `Iniciar sesion`/`Registrarse`) con tarjetas de tamaño fijo y validación de credenciales.
- `opergui/terminal_theme.go`: tema visual de la interfaz gráfica; paleta oscura, acentos verdes y tipografía monoespaciada.
- `opergui/go.sum`: sumas de verificación de dependencias del módulo GUI.
- Se retiraron los ejemplos independientes `saludo.go`, `shell.go` y `hilos/`, además de binarios y archivos de credenciales obsoletos que no forman parte del flujo activo.

## 3. Stack tecnológico y decisiones de diseño

- **Go**: lenguaje de implementación y concurrencia. El servidor, el cliente de consola y el monitor CLI pertenecen al módulo principal `operativos`; la GUI cliente se mantiene en un módulo separado. `go.mod` declara Go 1.27.3.
- **Fyne**: toolkit usado exclusivamente en `src/opergui`; no forma parte del servidor ni del monitor CLI. En Windows, la compilación de la GUI cliente requiere el entorno CGo indicado en el `README.md`.
- **Sockets TCP**: transporte de cliente a servidor. Los mensajes de aplicación se codifican como JSON, uno por línea. El protocolo permite autenticación, ejecución de comandos, ajuste del intervalo y envío de métricas. TCP no cifra el tráfico.
- **SHA-256**: se almacena el hash de la contraseña en el archivo plano de usuarios. En autenticación, el servidor emite un nonce y cliente y servidor calculan/verifican una prueba SHA-256 ligada al usuario y al desafío; se compara en tiempo constante. Este esquema no reemplaza TLS ni un algoritmo especializado de almacenamiento de contraseñas.
- **Goroutines y canales**: el servidor procesa conexiones concurrentes con un límite de 32; cada sesión intercala solicitudes, resultados y ticks de métricas. El cliente tiene una goroutine de lectura que publica eventos por un canal, mientras la GUI aplica cambios en el hilo de interfaz.
- **Configuración y despliegue**: los parámetros se leen desde `.conf`; `instalar-servidor.sh` automatiza el despliegue en Debian y systemd gestiona el ciclo de vida. La configuración de producción vive en `/etc/configProyecto.conf`; `AllowedClientIP` debe corresponder a la IP VPN de origen del cliente autorizado, mientras que la IP de destino que introduce el cliente es la IP VPN del servidor.
- **Métricas**: se recopilan periódicamente en el servidor Linux y se transmiten al cliente; la GUI presenta porcentajes de CPU, memoria y disco. El disco se consulta siempre en la partición raíz (`/`) con `syscall.Statfs_t`, sin depender del directorio de trabajo del proceso.
- **Ejecución normal**: se instala el servicio una vez y systemd lo mantiene activo; abrir o cerrar `iniciar-cli-servidor.sh` no afecta a las sesiones. En Windows, `cliente-gui.exe` se abre con doble clic; `cliente.exe` requiere PowerShell/consola y los argumentos IP, puerto y período.

## 4. Estado actual del desarrollo

> Resumen del código y configuración versionados. No garantiza que la máquina desplegada use la misma unidad, configuración o binario; para confirmar el estado real hay que consultar systemd y los archivos instalados. No se encontró un PDF de referencia en los archivos del repositorio.

| Área | Estado verificable |
|---|---|
| Servidor TCP y configuración `.conf` | **Implementado**: carga parámetros, valida valores y acepta configuración por argumentos de línea de comandos. |
| Autenticación | **Implementada**: archivo plano de hashes SHA-256, desafío aleatorio, verificación de prueba, autorización por IP/CIDR y limitación de intentos con espera progresiva. |
| Registro de usuarios | **Implementado en el código**: la GUI valida el nombre con `^[a-z_][a-z0-9_-]{0,31}$`, el servidor intenta crear una cuenta Linux regular con `/home/<usuario>` y persiste `usuario:hash`; el archivo de producción es `root:root 0600`. La unidad actual puede impedir la escritura de la base de cuentas durante el registro en vivo. |
| Ejecución remota | **Implementada**: los comandos corren con el UID/GID de la cuenta autenticada, desde su home y con las variables de entorno de usuario; hay timeout y límite de salida. No hay lista blanca de comandos. |
| Cliente de consola | **Implementado**: exige IP, puerto y período, valida argumentos, muestra el uso ante valores inválidos y conecta con el servidor. |
| Monitor CLI del servidor | **Implementado**: terminal en vivo con recursos y actividad JSONL; no depende de Fyne ni controla el daemon. |
| GUI Fyne | **Implementada en módulo separado**: interfaz de conexión, terminal, estado, historial y panel de métricas. La preparación de Windows la compila como `cliente-gui.exe`. |
| Concurrencia | **Implementada**: goroutines, canales, sincronización de escritura y límite de conexiones. |
| Métricas | **Implementadas en el código Linux**: reportes de CPU, procesos, memoria y disco de la raíz (`/`) con `syscall.Statfs_t`. |
| Pruebas automatizadas | Hay pruebas para argumentos, configuración, autenticación, sockets, shell, sesiones, logging, métricas y monitor CLI. Ejecutar los comandos descritos en el README para verificar el árbol actual; el módulo Fyne debe compilarse por separado desde `src/opergui`. |
| Instalación automatizada | **Implementada** mediante `instalar-servidor.sh`: permisos de archivos, compilación, provisión de usuarios iniciales, unidad systemd, habilitación y reinicio. El script detiene una instancia existente, captura errores con `trap` y solicita Enter al finalizar. |
| systemd | **Configuración con discrepancia conocida**: daemon root, capacidades limitadas, `ProtectSystem=full`, acceso a `/home` y `NoNewPrivileges=true`. `ProtectSystem=full` deja `/etc` de solo lectura y la plantilla no declara `ReadWritePaths` para `/etc/passwd`, `/etc/shadow`, `/etc/group` y archivos relacionados; el registro de cuentas desde el servicio puede fallar. El instalador crea las cuentas iniciales fuera del sandbox. |
| Logs | **Implementados en el código**: registros estructurados a stdout/journald y archivo con rotación configurable. |
| Seguridad de red | **Documentada**: se recomienda que cliente y servidor usen una VPN privada; `AllowedClientIP` debe ser la IP VPN del cliente. El protocolo de aplicación no cifra TCP por sí mismo. |
| Paper/documento académico | **Pendiente de confirmar**: no se encontró un paper o manuscrito en el repositorio. Debe elaborarse o actualizarse según los requisitos de la asignatura. |
| PDF de seguimiento | **No disponible** en los archivos inspeccionados; no es posible atribuirle hitos o pendientes específicos sin incorporarlo. |

### Pendientes de entrega

1. Verificar la unidad systemd efectivamente instalada y resolver, antes de depender del registro en vivo, la falta de rutas escribibles para la base de cuentas.
2. Completar o actualizar el paper/documento académico según los requisitos de la asignatura.
3. Incorporar el PDF de seguimiento, si se requiere mantener en este repositorio la trazabilidad de hitos académicos.

