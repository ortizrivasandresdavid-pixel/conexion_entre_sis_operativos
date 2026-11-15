# Administración y monitorización de laboratorio en Go

Ejemplo educativo de cliente Windows y servidor Debian/Linux mediante TCP. El cliente ofrece una consola de texto y una interfaz gráfica Fyne; ambas autentican al usuario, solicitan diagnósticos permitidos y muestran métricas periódicas.

## Alcance y seguridad

Por seguridad, el servidor **no ejecuta órdenes arbitrarias ni usa `bash -c`**. Los únicos diagnósticos remotos permitidos son:

```text
date
df -h
free -h
ls -l
ps -eo pid,comm,%cpu,%mem --sort=-%cpu
pwd
uname -a
uptime
whoami
help
bye
```

Los comandos se ejecutan sin shell y sin argumentos proporcionados por el cliente. `help` lista las opciones y `bye` cierra la sesión. `src/shell.go` reutiliza el mismo allowlist para el ejemplo de consola local.

La autenticación usa un desafío aleatorio por intento y una prueba basada en SHA-256; la contraseña no se envía en claro. El máximo de intentos se aplica a cada conexión TCP. **La conexión TCP de esta aplicación no está cifrada**: las órdenes, sus salidas y métricas pueden ser observadas si el tráfico sale de una red confiable. Para conectar desde otras redes, usa una VPN privada entre servidor y clientes; no publiques ni redirijas el puerto TCP de la aplicación directamente a Internet.

El servidor verifica la IP o CIDR autorizada y limita los intentos fallidos y el período de reporte. Admite hasta 32 conexiones simultáneas. El cliente no se reconecta automáticamente; para iniciar una nueva sesión, vuelve a ejecutarlo y autentícate de nuevo.

## Requisitos

- Go 1.27.3 o posterior.
- Servidor Debian/Linux (la recolección de CPU, procesos, memoria y disco usa `/proc` y `statfs`).
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

El servidor escucha en `ListenIP:Port`. Para detenerlo, usa `Ctrl+C`.

## Usar el cliente de consola

La consola de Windows permite iniciar la conexión, ingresar las credenciales, escribir diagnósticos y recibir métricas.

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

### 3. Autenticarse y usar la consola

El cliente solicita las credenciales en este orden:

```text
Usuario: admin
Password:
```

Escribe la contraseña y presiona Enter; no se mostrará mientras la escribes. Para probar en un laboratorio recién clonado, el archivo de ejemplo incluye `admin` / `linux`. Cambia esa cuenta antes de usarlo fuera de una práctica local.

Después de autenticarte, puedes escribir `help` para ver la lista de diagnósticos admitidos. Por ejemplo:

```text
Autenticación correcta. Escribe help para ver los diagnósticos permitidos; bye para salir.
Recibirás métricas cada 5 segundos.
help
Comandos permitidos: date, df -h, free -h, ls -l, ps -eo pid,comm,%cpu,%mem --sort=-%cpu, pwd, uname -a, uptime, whoami
df -h
```

Los comandos se ejecutan en Debian, no en Windows. Escribe el comando completo tal como aparece en `help`; no se admiten comandos arbitrarios, tuberías ni argumentos distintos. El cliente muestra las métricas periódicas en la misma consola, incluso mientras escribes. Para cerrar la sesión de forma normal, escribe `bye`.

Si la conexión se interrumpe, confirma que la VPN esté conectada en ambos equipos y revisa la IP VPN, el puerto, el firewall y `AllowedClientIP`; luego vuelve a iniciar el cliente. Un error de autenticación permite reintentar hasta alcanzar `MaxAuthAttempts`.

### Problemas frecuentes

- **No se puede conectar:** verifica que la VPN esté activa en ambos equipos, que haya una ruta entre sus IP VPN, que el servidor esté ejecutándose y que el firewall permita TCP 2306 desde la VPN.
- **IP no autorizada:** `AllowedClientIP` debe coincidir con la IP VPN del cliente (o con la subred VPN que la incluya).
- **Credenciales incorrectas:** comprueba el usuario y el hash de contraseña en el archivo indicado por `UsersFile`.
- **Comando no permitido:** ejecuta `help` y copia uno de los diagnósticos listados; no agregues argumentos.

Para compilar el ejecutable de Windows desde Debian, si lo necesitas:

```bash
GOOS=windows GOARCH=amd64 go build -o cliente.exe ./cliente
```

## Usar una terminal Debian completa por SSH

La aplicación de sockets mantiene una lista cerrada de diagnósticos; `ls -l` lista el directorio de trabajo, pero no acepta rutas ni opciones adicionales. Para ejecutar cualquier comando normal de Debian, usa OpenSSH desde Windows Terminal o PowerShell, no el puerto de la aplicación.

En Debian, si OpenSSH Server aún no está instalado, instálalo e inicia el servicio. Configura autenticación por clave y limita el puerto SSH (22) a la VPN en el firewall; no lo publiques directamente en Internet.

```bash
sudo apt install openssh-server
sudo systemctl enable --now ssh
```

Desde PowerShell, conecta usando una cuenta normal de Debian y la IP VPN del servidor:

```powershell
ssh usuario@10.8.0.1
```

Reemplaza `usuario` y `10.8.0.1` por la cuenta y la IP VPN reales. Ya dentro de la sesión SSH, dispondrás de la terminal Debian completa.

## Usar la interfaz gráfica Fyne

La aplicación gráfica tiene apariencia de terminal e incluye campos de IP, puerto, período, usuario y contraseña; una salida de comandos, un campo para enviar diagnósticos, barras de CPU/memoria/disco y un contador e historial de reportes. La autenticación y el intercambio de mensajes reutilizan el protocolo TCP de `src/sockets`; los reportes se procesan concurrentemente y se reflejan en la ventana sin bloquearla.

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

En la ventana, ingresa la IP del servidor, el puerto (por defecto `2306`), el período de reportes (por defecto `5` segundos) y las credenciales; luego selecciona **Conectar**. Tras autenticarte, escribe un comando permitido como `help` o `df -h` y presiona Enter o **Ejecutar**. Solo se permite un comando pendiente a la vez; los reportes periódicos siguen actualizando el panel lateral. Para cerrar la sesión, envía `bye`.

La GUI y el cliente de consola son ejecutables separados. Los argumentos de consola siguen siendo compatibles (`cliente <IP-servidor> [puerto] [periodo-segundos]`); para la GUI, esos valores se ingresan en los campos de la ventana.

## Ejecutar el shell de diagnóstico local

En Linux, desde la raíz:

```bash
go run ./src/shell.go
```

Escribe `help` para ver los diagnósticos permitidos y `exit` o `bye` para salir. El shell local tampoco ejecuta órdenes fuera de la lista.

## Pruebas

```bash
go test ./cliente ./servidor ./src/login ./src/metrics ./src/shell ./src/sockets
```
