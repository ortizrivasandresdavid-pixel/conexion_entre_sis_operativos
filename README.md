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

## Usar la interfaz del cliente (consola interactiva)

El proyecto no incluye una interfaz gráfica. La interfaz disponible es la consola de Windows: desde allí se inicia la conexión, se ingresan las credenciales, se escriben los diagnósticos y se reciben las métricas.

### 1. Preparar el servidor

En `servidor/server.conf`, configura `AllowedClientIP` con la IP del equipo Windows tal como la ve el servidor Debian. `127.0.0.1` solo sirve si el cliente y el servidor se ejecutan en el mismo equipo. Puedes usar una dirección CIDR para autorizar una red controlada.

`ListenIP=0.0.0.0` permite escuchar en todas las interfaces del servidor; limita el puerto 2306 con el firewall para que solo sea accesible desde la red de laboratorio. Asegúrate también de que la IP de escucha corresponda a una interfaz disponible en Debian.

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

Ejecuta el cliente con la IP de Debian, el puerto y el intervalo de métricas (en segundos):

```powershell
.\cliente.exe 10.1.10.3 2306 5
```

Reemplaza `10.1.10.3` por la IP real del servidor. El intervalo debe ser de 1 a 3600 segundos y no puede superar `MaxReportPeriodSeconds` del servidor.

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
Comandos permitidos: date, df -h, free -h, ps -eo pid,comm,%cpu,%mem --sort=-%cpu, pwd, uname -a, uptime, whoami
df -h
```

Los comandos se ejecutan en Debian, no en Windows. Escribe el comando completo tal como aparece en `help`; no se admiten comandos arbitrarios, tuberías ni argumentos distintos. El cliente muestra las métricas periódicas en la misma consola, incluso mientras escribes. Para cerrar la sesión de forma normal, escribe `bye`.

Si la conexión se interrumpe, revisa la IP, el puerto, el firewall y `AllowedClientIP`; luego vuelve a iniciar el cliente. Un error de autenticación permite reintentar hasta alcanzar `MaxAuthAttempts`.

### Problemas frecuentes

- **No se puede conectar:** verifica que el servidor esté ejecutándose y que el puerto TCP configurado esté permitido en el firewall.
- **IP no autorizada:** `AllowedClientIP` debe coincidir con la IP de origen del cliente que recibe Debian (o con una red CIDR que la incluya).
- **Credenciales incorrectas:** comprueba el usuario y el hash de contraseña en el archivo indicado por `UsersFile`.
- **Comando no permitido:** ejecuta `help` y copia uno de los diagnósticos listados; no agregues argumentos.

Para compilar el ejecutable de Windows desde Debian, si lo necesitas:

```bash
GOOS=windows GOARCH=amd64 go build -o cliente.exe ./cliente
```

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
