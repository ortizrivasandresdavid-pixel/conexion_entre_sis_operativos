# Contexto del proyecto

## 1. Propósito general del proyecto

Proyecto universitario de sistemas operativos que implementa un canal cliente-servidor para administrar y monitorizar un equipo Debian/Linux desde un cliente Windows. El servidor escucha conexiones TCP, autoriza la dirección IP del cliente, autentica usuarios mediante un archivo plano de hashes SHA-256 y, tras autenticar, permite ejecutar comandos remotos y recibir reportes periódicos de recursos.

El cliente está disponible como aplicación de consola y como interfaz gráfica multiplataforma desarrollada con Fyne. La GUI presenta una terminal, el estado de conexión y métricas de CPU, memoria y disco. El intercambio de mensajes ocurre concurrentemente, sin bloquear la interfaz mientras se reciben respuestas o reportes.

El proyecto tiene fines académicos y no sustituye SSH para administración de sistemas. El protocolo propio usa TCP sin cifrado y el servidor ejecuta comandos reales con los permisos de la cuenta que lo ejecuta; por tanto, debe usarse únicamente en una red confiable/VPN y con una cuenta de sistema sin privilegios.

## 2. Estructura de directorios clave

### Raíz

- `README.md`: guía principal; documenta alcance, seguridad, configuración, compilación, ejecución, despliegue en Debian, uso de consola y GUI y conexión por VPN.
- `go.mod` / `go.sum`: módulo principal `operativos`, versión declarada de Go 1.27.3 y dependencias del cliente de consola.
- `iniciar-servidor-debian.sh`: comprueba la versión de Go, elige la configuración de producción o de desarrollo y ejecuta el servidor.
- `preparar-debian.sh` / `preparar-windows.ps1`: scripts de preparación de los entornos indicados por su nombre.
- `oper.user.db`: archivo de credenciales de ejemplo usado por el código legado de `servidor/auth.go`; no es el almacén configurado por el servidor principal.

### `cliente/`

- `main.go`: entrada del cliente de consola. Valida IP, puerto y período de reportes, gestiona señales de terminación y delega la sesión a `src/sockets`.
- `main_test.go`: pruebas de los argumentos de línea de comandos.

### `servidor/`

- `main.go`: entrada del servidor. Lee opciones de línea de comandos, carga y valida el archivo `.conf`, configura el logger y arranca/detiene el servicio según las señales del sistema.
- `auth.go`: autenticación simple de una versión anterior, que compara SHA-256 contra `oper.user.db`. No corresponde al flujo de autenticación por desafío del servidor actual.
- `server.conf`: configuración de desarrollo; define dirección/puerto, IP autorizada, límites de autenticación, archivo de usuarios, período de métricas y logs.
- `users.conf`: usuarios de ejemplo para la configuración de desarrollo; contiene hashes, no debe tratarse como almacén de credenciales de producción.
- `main_test.go`: pruebas del parseo y resolución de la configuración del servidor.

### `deploy/`

- `configProyecto.conf`: plantilla para `/etc/configProyecto.conf`, con rutas centralizadas para usuarios y logs.
- `configProyecto.users`: plantilla de usuarios para el despliegue.
- `servidor-operativos.service`: unidad systemd que ejecuta el servidor como el usuario dedicado `operativos`, configura reinicio y directorios de estado/logs y aplica restricciones de seguridad.

### `src/`

- `sockets/protocol.go`: define el formato JSON de los mensajes de aplicación y sus operaciones de lectura/escritura sobre TCP.
- `sockets/serverOper.go`: servidor TCP; aplica restricción de IP/CIDR, autenticación, límite de conexiones, comandos, período de métricas y cierre ordenado.
- `sockets/clientOper.go`: operaciones de cliente y flujo de autenticación/sesión.
- `sockets/client_session.go`: API concurrente de sesión del cliente; usa goroutine de lectura, canal de eventos, identificadores de solicitudes y escritura sincronizada.
- `sockets/auth_result.go`: interpretación de resultados y errores de autenticación.
- `sockets/*_test.go`: pruebas del protocolo, sesiones y comportamiento del servidor.
- `login/login.go`: generación y verificación de pruebas de autenticación basadas en SHA-256; la contraseña no se envía directamente por el protocolo.
- `login/archivos.go`: carga y validación del archivo plano de usuarios.
- `login/attempts.go`: control de intentos fallidos y espera progresiva por dirección IP.
- `login/*_test.go`: pruebas de autenticación, lectura de usuarios y límites de intentos.
- `shell/shell.go`: ejecución de comandos mediante `bash` o `sh`, conservación del directorio por sesión, límite de duración y tamaño de salida.
- `shell/proc_unix.go` / `shell/proc_other.go`: comportamiento específico de plataforma para gestionar procesos.
- `shell/shell_test.go`: pruebas de ejecución de comandos y límites.
- `metrics/metrics.go`: estructura común del reporte de recursos.
- `metrics/metrics_linux.go`: recopilación en Linux desde `/proc` y `statfs` (CPU, procesos, memoria y disco de `/`).
- `metrics/metrics_other.go`: implementación alternativa para plataformas no Linux.
- `logging/logging.go`: logger estructurado con `log/slog`, salida a consola/archivo y rotación por tamaño.
- `logging/logging_test.go`: pruebas de escritura y rotación de logs.
- `opergui/`: módulo Go separado para evitar incorporar Fyne a las dependencias del servidor o de la consola. `go.mod` declara Fyne v2.8.2 y reemplaza el módulo `operativos` por la raíz del repositorio.
- `opergui/proyOperGUI.go`: aplicación Fyne; formulario de conexión, autenticación, terminal, historial y panel de métricas. La lectura de sesión se realiza concurrentemente y las actualizaciones visuales se envían al hilo de UI mediante `fyne.Do`.
- `opergui/terminal_theme.go`: tema visual de la interfaz gráfica.
- `opergui/go.sum`: sumas de verificación de dependencias del módulo GUI.
- `saludo.go`, `shell.go`, `hilos/` y `opergui/` son componentes auxiliares o de ejercicios previos; las rutas principales de cliente y servidor son `cliente/`, `servidor/` y `src/sockets/`.

## 3. Stack tecnológico y decisiones de diseño

- **Go**: lenguaje de implementación y concurrencia. El servidor y cliente de consola pertenecen al módulo principal `operativos`; la GUI se mantiene en un módulo separado. `go.mod` declara Go 1.27.3.
- **Fyne**: toolkit de interfaz gráfica usado en `src/opergui`. La separación de módulos reduce el impacto de sus dependencias sobre el servidor y el cliente de consola. En Windows, la compilación requiere el entorno CGo indicado en el `README.md`.
- **Sockets TCP**: transporte de cliente a servidor. Los mensajes de aplicación se codifican como JSON, uno por línea. El protocolo permite autenticación, ejecución de comandos, ajuste del intervalo y envío de métricas. TCP no cifra el tráfico.
- **SHA-256**: se almacena el hash de la contraseña en el archivo plano de usuarios. En autenticación, el servidor emite un nonce y cliente y servidor calculan/verifican una prueba SHA-256 ligada al usuario y al desafío; se compara en tiempo constante. Este esquema no reemplaza TLS ni un algoritmo especializado de almacenamiento de contraseñas.
- **Goroutines y canales**: el servidor procesa conexiones concurrentes con un límite de 32; cada sesión intercala solicitudes, resultados y ticks de métricas. El cliente tiene una goroutine de lectura que publica eventos por un canal, mientras la GUI aplica cambios en el hilo de interfaz.
- **Configuración y despliegue**: los parámetros se leen desde `.conf`; existen una configuración de desarrollo y plantillas de instalación para `/etc`. En Debian, systemd gestiona el ciclo de vida del servidor.
- **Métricas**: se recopilan periódicamente en el servidor Linux y se transmiten al cliente; la GUI presenta porcentajes de CPU, memoria y disco.

## 4. Estado actual del desarrollo

> No se encontró un PDF de referencia en los archivos disponibles del repositorio. Por ello, este estado se basa en el código, las pruebas y la documentación presentes; las marcas “implementado” no implican validación en una instalación Debian/Windows concreta.

| Área | Estado verificable |
|---|---|
| Servidor TCP y configuración `.conf` | **Implementado**: carga parámetros, valida valores y acepta configuración por argumentos de línea de comandos. |
| Autenticación | **Implementada**: archivo plano de hashes SHA-256, desafío aleatorio, verificación de prueba, autorización por IP/CIDR y limitación de intentos con espera progresiva. |
| Ejecución remota | **Implementada**: comandos mediante `bash`/`sh`, directorio de trabajo persistente por sesión, timeout y límite de salida. No hay lista blanca de comandos. |
| Métricas | **Implementadas**: reportes de CPU, procesos, memoria y disco raíz en Linux; envío periódico y consumo en el cliente. |
| Cliente de consola | **Implementado**: validación de parámetros y conexión al servidor. |
| GUI Fyne | **Implementada**: interfaz de conexión, terminal, estado, historial y panel de métricas; se distribuye como módulo separado. |
| Concurrencia | **Implementada**: goroutines, canales, sincronización de escritura y límite de conexiones. |
| Pruebas automatizadas | **Hay pruebas** para configuración, autenticación, sockets, shell, sesiones y logging. La existencia de tests no acredita por sí sola que se hayan ejecutado en todos los entornos objetivo. |
| systemd | **Artefacto implementado**: existe `deploy/servidor-operativos.service` y el README documenta instalación y endurecimiento. **Pendiente de validar** en el Debian de destino: instalación, permisos, arranque, reinicio y acceso efectivo a logs. |
| Logs | **Implementados en el código**: registros estructurados a stdout y archivo, con rotación configurable; systemd puede enviar stdout a journald. **Pendiente de validar** en despliegue: permisos, creación de directorios, rotación y consulta con journald. |
| Paper/documento académico | **Pendiente de confirmar**: no se encontró un paper o manuscrito en el repositorio. Debe elaborarse o actualizarse según los requisitos de la asignatura. |
| PDF de seguimiento | **No disponible** en los archivos inspeccionados; no es posible atribuirle hitos o pendientes específicos sin incorporarlo. |

### Riesgos y validaciones pendientes

1. Validar en un Debian limpio el servicio systemd con usuario dedicado y rutas de configuración, usuarios y logs.
2. Verificar conectividad y reglas de firewall/VPN con las IP configuradas para cliente y servidor.
3. Probar la GUI en Windows con Fyne/CGo y confirmar la experiencia de ejecución en la plataforma objetivo.
4. Documentar explícitamente en la entrega académica que el protocolo no cifra tráfico y que una cuenta autenticada obtiene los permisos del proceso servidor.
5. Completar el paper y comparar el estado anterior con el PDF de avance cuando este esté disponible.

