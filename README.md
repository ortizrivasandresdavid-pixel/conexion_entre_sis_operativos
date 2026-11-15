# Administración y monitorización de laboratorio en Go

Ejemplo educativo de cliente Windows y servidor Debian/Linux mediante TCP. El cliente ofrece una consola de texto y una interfaz gráfica Fyne; ambas autentican al usuario y permiten ejecutar comandos reales en el servidor, además de mostrar métricas periódicas de CPU, memoria y disco.

## Alcance y seguridad

El servidor **ejecuta cualquier comando de Linux** que envíe un cliente autenticado, igual que una sesión SSH: no hay lista blanca. Cada comando se evalúa con `bash` (o `sh` como alternativa) mediante `bash -c`, de modo que funcionan tuberías, redirecciones, comodines, `&&`, scripts y comandos como `cd`, `tree`, `apt`, `systemctl`, etc.

El directorio de trabajo se conserva durante toda la conexión: si escribes `cd /var/log`, el siguiente comando se ejecuta en `/var/log`. Cada comando se ejecuta en un proceso `bash` nuevo, así que las variables exportadas con `export` no se conservan entre comandos.

Esta funcionalidad es equivalente a dar una shell remota, por lo que debes tratarla con las mismas precauciones que SSH:

- **La conexión TCP no está cifrada.** Los comandos, sus salidas y las métricas pueden ser observados si el tráfico sale de una red confiable. Usa una VPN privada (WireGuard, Tailscale, etc.) entre servidor y clientes y no publiques ni redirijas el puerto TCP a Internet.
- **El servidor ejecuta los comandos con sus propios permisos.** Ejecútalo con una cuenta de laboratorio sin privilegios, nunca como `root` ni con `sudo` permanente. Quien se autentique tendrá los mismos permisos que el proceso del servidor.
- **Autenticación y control de acceso:** desafío aleatorio por intento y prueba basada en SHA-256 (la contraseña no viaja en claro), IP o CIDR autorizado y bloqueo temporal de la IP tras varios fallos.

Límites pensados para no bloquear el protocolo ni la memoria del servidor:

- Cada comando tiene un **tiempo límite de 5 minutos**; al vencer se cancela el grupo de procesos y se devuelve la salida parcial.
- La salida se limita a **2 MiB** por comando. Si se supera, se corta y se añade el aviso `[salida truncada a 2 MiB]`.
- No es una terminal interactiva completa: no hay TTY ni Ctrl+C, así que los programas de pantalla completa (`vim`, `nano`, `top` interactivo) y los que piden contraseña por terminal (`sudo`, `passwd`) no funcionan. Usa variantes no interactivas como `top -b -n 1`.

El servidor verifica la IP o CIDR autorizada y limita los intentos fallidos y el período de reporte. Admite hasta 32 conexiones simultáneas. El cliente no se reconecta automáticamente; para iniciar una nueva sesión, vuelve a ejecutarlo y autentícate de nuevo.

## Requisitos

- Go 1.27.3 o posterior.
- Servidor Debian/Linux (la recolección de CPU, procesos, memoria y disco usa `/proc` y `statfs`, y la ejecución remota requiere `bash` o `sh`).
- Cliente Windows con acceso TCP al servidor.
- Puerto TCP 2306 permitido en el firewall entre ambos equipos.

## Archivos de configuración de ejemplo

`servidor/server.conf` contiene:

```ini
ListenIP=0.0.0.0
Port=2306
AllowedClientIP=127.0.0.1
MaxAuthAttempts=3
UsersFile=users.conf
DefaultReportPeriodSeconds=5
MaxReportPeriodSeconds=60
```

Para acceso por VPN, cambia `AllowedClientIP` por la IP VPN de un cliente o por la subred privada asignada a los clientes autorizados. `ListenIP` es la IP local del servidor; `0.0.0.0` escucha en todas sus interfaces, así que restringe el puerto 2306 con el firewall para permitirlo solo desde la interfaz o subred VPN.

`servidor/users.conf` trae una cuenta de laboratorio:

```text
Usuario: admin
Password: linux
```

El archivo almacena `usuario:SHA256(password)`, no la contraseña. Genera hashes para otras cuentas con:

```bash
printf %s 'TuPassword' | sha256sum
```

Agrega una línea `usuario:hash_de_64_caracteres` a `users.conf`. No reutilices la cuenta ni la contraseña de ejemplo.

## Ejecutar en Debian (servidor)

Desde la raíz del proyecto:

```bash
go build -o servidor ./servidor
./servidor -config servidor/server.conf
```

También se puede iniciar directamente:

```bash
go run ./servidor -config servidor/server.conf
```

El servidor escucha en `ListenIP:Port` y arranca su shell en el directorio desde el que se lanzó. Para detenerlo, usa `Ctrl+C`.

## Usar el cliente de consola

La consola de Windows permite iniciar la conexión, ingresar las credenciales, ejecutar comandos y recibir métricas.

### 1. Preparar el servidor

Para conectar clientes desde distintas redes físicas, primero instala y configura una VPN privada (por ejemplo, WireGuard o Tailscale) en Debian y en cada Windows. Asegúrate de que ambos estén conectados a la misma VPN y que Windows tenga una ruta hacia la IP VPN de Debian. La VPN proporciona el acceso cifrado entre redes; esta aplicación no lo hace por sí sola.

En `servidor/server.conf`, configura `AllowedClientIP` con la IP VPN del cliente o con la subred VPN que quieras autorizar. `127.0.0.1` solo sirve cuando cliente y servidor se ejecutan en el mismo equipo. Mantén `ListenIP=0.0.0.0` o usa la IP VPN estable del servidor. Configura el firewall de Debian para permitir TCP 2306 solo desde la VPN; no redirijas ese puerto en el router hacia Internet. La VPN debe estar conectada antes de probar la aplicación.

Inicia el servidor desde la raíz del proyecto:

```bash
go run ./servidor -config servidor/server.conf
```

Déjalo ejecutándose mientras uses el cliente. Si cambias `UsersFile`, recuerda que su ruta relativa se resuelve respecto de la carpeta `servidor`.

### 2. Iniciar el cliente en Windows

Abre PowerShell en la raíz del proyecto y compila el cliente:

```powershell
go build -o cliente.exe .\cliente
```

Ejecuta el cliente con la IP VPN del servidor:

```powershell
.\cliente.exe 10.8.0.1
```

Reemplaza `10.8.0.1` por la IP VPN real de Debian. Al indicar solo la IP, el cliente usa el puerto 2306 y recibe métricas cada 5 segundos. Si necesitas otros valores, puedes indicarlos opcionalmente: `.\cliente.exe <IP-VPN-servidor> <puerto> <periodo-segundos>`. El período debe ser de 1 a 3600 segundos y no puede superar `MaxReportPeriodSeconds` del servidor.

### 3. Autenticarse y usar la terminal

El cliente solicita las credenciales en este orden:

```text
Usuario: admin
Password:
```

Escribe la contraseña y presiona Enter; no se mostrará mientras la escribes. Para probar en un laboratorio recién clonado, el archivo de ejemplo incluye `admin` / `linux`. Cambia esa cuenta antes de usarlo fuera de una práctica local.

Después de autenticarte puedes escribir cualquier comando de Linux. Por ejemplo:

```text
Autenticación correcta. Escribe cualquier comando de Linux; 'bye' para salir.
Recibirás métricas cada 5 segundos.
ls -la
cd /var/log
pwd
tail -n 20 syslog
```

`cd`, `pwd`, `tree` y el resto de comandos se ejecutan en Debian, no en Windows. Como el directorio de trabajo se conserva, tras `cd /var/log` el siguiente `pwd` responde `/var/log`. Si un comando devuelve un código de salida distinto de cero, el cliente muestra `[código de salida: N]` sin cortar la sesión. El cliente muestra las métricas periódicas en la misma consola, incluso mientras escribes. Para cerrar la sesión de forma normal, escribe `bye`.

Si la conexión se interrumpe, confirma que la VPN esté conectada en ambos equipos y revisa la IP VPN, el puerto, el firewall y `AllowedClientIP`; luego vuelve a iniciar el cliente. Los fallos de autenticación persisten por IP incluso al abrir otra conexión y el cliente informa la espera y los intentos restantes.

### Problemas frecuentes

- **No se puede conectar:** verifica que la VPN esté activa en ambos equipos, que haya una ruta entre sus IP VPN, que el servidor esté ejecutándose y que el firewall permita TCP 2306 desde la VPN.
- **IP no autorizada:** `AllowedClientIP` debe coincidir con la IP VPN del cliente (o con la subred VPN que la incluya).
- **Credenciales incorrectas:** comprueba el usuario y el hash de contraseña en el archivo indicado por `UsersFile`.
- **`tree: command not found`:** instálalo en el servidor con `sudo apt install tree`.
- **Un comando se queda colgado:** espera a que venza el límite de 5 minutos; los programas interactivos o que esperan un TTY (por ejemplo `sudo` o `vim`) no funcionan por este canal.
- **Autenticación bloqueada:** espera el tiempo indicado antes de intentar de nuevo. El bloqueo se aplica a la IP cliente.

Para compilar el ejecutable de Windows desde Debian, si lo necesitas:

```bash
GOOS=windows GOARCH=amd64 go build -o cliente.exe ./cliente
```

## Diferencia con SSH

Esta aplicación usa su propio protocolo TCP sin cifrar y sin TTY; SSH sigue siendo la opción recomendada para administración interactiva en producción. El proyecto implementa su propio canal para cumplir el objetivo académico de sockets, autenticación y métricas. Si usas OpenSSH en paralelo, instálalo y limita el puerto 22 a la VPN:

```bash
sudo apt install openssh-server
sudo systemctl enable --now ssh
```

```powershell
ssh usuario@10.8.0.1
```

## Usar la interfaz gráfica Fyne

La aplicación gráfica tiene fondo negro, texto verde monoespaciado en la terminal y el historial de reportes. Incluye campos de IP, puerto, período, usuario y contraseña; salida de comandos, barras de CPU/memoria/disco, contador e historial con desplazamiento automático. La autenticación y el intercambio de mensajes reutilizan el protocolo TCP de `src/sockets`; los reportes se procesan concurrentemente y se reflejan en la ventana sin bloquearla.

La GUI está en su propio módulo para que Fyne no cambie las dependencias del cliente de consola ni del servidor. En Windows, Fyne requiere CGo y un compilador C. Si usas MSYS2 en `C:\msys64`, configura MinGW64 en la misma ventana de PowerShell antes de ejecutar o compilar:

```powershell
$env:CC = 'C:\msys64\mingw64\bin\gcc.exe'
$env:PATH = "C:\msys64\mingw64\bin;$env:PATH"
$env:CGO_ENABLED = '1'
```

Desde la raíz del proyecto, ejecútala en desarrollo:

```powershell
go -C .\src\opergui run .
```

O compílala en Windows:

```powershell
go -C .\src\opergui build -o ..\..\cliente-gui.exe .
.\cliente-gui.exe
```

En la ventana, ingresa la IP del servidor, el puerto (por defecto `2306`), el período de reportes (por defecto `5` segundos) y las credenciales; luego selecciona **Conectar**. Tras autenticarte, escribe cualquier comando de Linux (`ls -la`, `df -h`, `cd /tmp`, `pwd`...) y presiona Enter o **Ejecutar**. El indicador antes de cada comando muestra el directorio de trabajo actual en el servidor, y las salidas con código distinto de cero se marcan con `[código de salida: N]`. Solo se permite un comando pendiente a la vez; los reportes periódicos siguen actualizando el panel lateral. Para cerrar la sesión, envía `bye`.

La GUI y el cliente de consola son ejecutables separados. Los argumentos de consola siguen siendo compatibles (`cliente <IP-servidor> [puerto] [periodo-segundos]`); para la GUI, esos valores se ingresan en los campos de la ventana.

## Ejecutar el shell local

En Linux, desde la raíz:

```bash
go run ./src/shell.go
```

También ejecuta cualquier comando de tu propia shell y conserva el directorio de trabajo entre comandos. Escribe `exit` o `bye` para salir.

## Pruebas

```bash
go test ./cliente ./servidor ./src/login ./src/metrics ./src/shell ./src/sockets
```

Las pruebas de `src/shell` ejecutan `bash`/`sh`, así que se omiten en Windows o donde no haya una shell POSIX disponible.
