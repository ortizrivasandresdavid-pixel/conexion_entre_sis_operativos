# Administración y monitorización de laboratorio en Go

Ejemplo educativo de cliente Windows y servidor Debian/Linux mediante TCP. El cliente autentica al usuario, solicita diagnósticos permitidos y muestra métricas periódicas sin detener la entrada interactiva.

## Alcance y seguridad

Por seguridad, el servidor **no ejecuta órdenes arbitrarias ni usa `bash -c`**. Los únicos diagnósticos remotos permitidos son:

```text
date
df -h
free -h
ps -eo pid,comm,%cpu,%mem --sort=-%cpu
pwd
uname -a
uptime
whoami
help
bye
```

Los comandos se ejecutan sin shell y sin argumentos proporcionados por el cliente. `help` lista las opciones y `bye` cierra la sesión. `src/shell.go` reutiliza el mismo allowlist para el ejemplo de consola local.

La autenticación usa un desafío aleatorio por intento y una prueba basada en SHA-256; la contraseña no se envía en claro. El máximo de intentos se aplica a cada conexión TCP. **TCP no está cifrado**: las órdenes, sus salidas y métricas sí pueden ser observadas en la red. Usa únicamente una red aislada de laboratorio. No expongas el puerto a Internet; para administración real, utiliza SSH o un transporte TLS autenticado.

El servidor verifica la IP o CIDR autorizada y limita los intentos fallidos y el período de reporte. El cliente no se reconecta automáticamente; para iniciar una nueva sesión, vuelve a ejecutarlo y autentícate de nuevo.

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

`AllowedClientIP` debe cambiarse por la IP del cliente Windows (o una red CIDR controlada). `ListenIP` es la IP local del servidor; `0.0.0.0` escucha en todas sus interfaces, así que restringe el tráfico también con el firewall.

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

## Compilar y ejecutar en Windows (cliente)

Desde la raíz del proyecto en PowerShell:

```powershell
go build -o cliente.exe .\cliente
.\cliente.exe 10.1.10.3 2306 5
```

Reemplaza `10.1.10.3` por la IP del servidor Debian. El último argumento define el período de métricas en segundos (1–3600); el servidor lo limita con `MaxReportPeriodSeconds`. Si se compila en Debian para distribuir el ejecutable Windows:

```bash
GOOS=windows GOARCH=amd64 go build -o cliente.exe ./cliente
```

El cliente solicita usuario y contraseña (la contraseña no se muestra), después acepta órdenes de diagnóstico. Las métricas se imprimen a medida que llegan, incluso mientras se escribe en la consola. Escribe `bye` para cerrar limpiamente la conexión. Un fallo de conexión se informa; inicia el cliente otra vez para reconectar.

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
