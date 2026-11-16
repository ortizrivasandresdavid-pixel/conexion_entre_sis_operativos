# Administración y monitorización de laboratorio en Go

Ejemplo educativo de cliente Windows y servidor Debian/Linux mediante TCP. El cliente ofrece una consola de texto y una interfaz gráfica Fyne; ambas autentican al usuario y permiten ejecutar comandos reales en el servidor, además de mostrar métricas periódicas de CPU, memoria y disco.

## Alcance y seguridad

El servidor **ejecuta cualquier comando de Linux** que envíe un cliente autenticado, igual que una sesión SSH: no hay lista blanca. Cada comando se evalúa con `bash` (o `sh` como alternativa) mediante `bash -c`, de modo que funcionan tuberías, redirecciones, comodines, `&&`, scripts y comandos como `cd`, `tree`, `apt`, `systemctl`, etc.

El directorio de trabajo se conserva durante toda la conexión: si escribes `cd /var/log`, el siguiente comando se ejecuta en `/var/log`. Cada comando se ejecuta en un proceso `bash` nuevo, así que las variables exportadas con `export` no se conservan entre comandos.

Esta funcionalidad es equivalente a dar una shell remota, por lo que debes tratarla con las mismas precauciones que SSH:

- **La conexión TCP no está cifrada.** Los comandos, sus salidas y las métricas pueden ser observados si el tráfico sale de una red confiable. Usa una VPN privada (WireGuard, Tailscale, etc.) entre servidor y clientes y no publiques ni redirijas el puerto TCP a Internet.
- **El servidor ejecuta los comandos con sus propios permisos.** Ejecútalo con una cuenta de laboratorio sin privilegios, nunca como `root` ni con `sudo` permanente. Quien se autentique tendrá los mismos permisos que el proceso del servidor.
- **Autenticación y control de acceso:** desafío aleatorio por intento y prueba basada en SHA-256 (la contraseña no viaja en claro), IP o CIDR autorizado y backoff progresivo por IP tras fallos. El servidor informa el tiempo de espera restante; al agotar los intentos responde `AUTH_LOCKED` y cierra la conexión. Cuando vence la penalización, el contador de esa IP se restablece.

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

## Configuración centralizada y logs

El servidor busca su configuración por defecto en `/etc/configProyecto.conf`. La ruta se puede cambiar con `-config`; `-users` y `-log` permiten sobrescribir `UsersFile` y `LogFile` sin editar el archivo. Las rutas relativas de `UsersFile` y `LogFile` se resuelven respecto del directorio del archivo de configuración; las de `-users` y `-log`, respecto del directorio actual.

Para desarrollo, `servidor/server.conf` sigue siendo válido: si no se indica `-config` y `/etc/configProyecto.conf` no existe, el servidor intenta `servidor/server.conf` desde la raíz del repositorio. Para producción hay plantillas listas para copiar en `deploy/`:

- `deploy/configProyecto.conf` → `/etc/configProyecto.conf`
- `deploy/configProyecto.users` → `/etc/configProyecto.users`
- `deploy/servidor-operativos.service` → `/etc/systemd/system/servidor-operativos.service`

`servidor/server.conf` contiene:

```ini
ListenIP=0.0.0.0
Port=2306
AllowedClientIP=100.66.232.76
MaxAuthAttempts=3
UsersFile=users.conf
DefaultReportPeriodSeconds=5
MaxReportPeriodSeconds=60
LogFile=servidor.log
LogMaxSizeMB=10
LogBackups=3
```

Para acceso por VPN, cambia `AllowedClientIP` por la IP VPN de un cliente o por la subred privada asignada a los clientes autorizados. `ListenIP` es la IP local del servidor; `0.0.0.0` escucha en todas sus interfaces, así que restringe el puerto 2306 con el firewall para permitirlo solo desde la interfaz o subred VPN.

`deploy/configProyecto.conf` ya apunta a las rutas centralizadas:

```ini
ListenIP=0.0.0.0
Port=2306
AllowedClientIP=127.0.0.1
MaxAuthAttempts=3
UsersFile=/etc/configProyecto.users
DefaultReportPeriodSeconds=5
MaxReportPeriodSeconds=60
LogFile=/var/log/proyecto-operativos/servidor.log
LogMaxSizeMB=10
LogBackups=5
```

`servidor/users.conf` y `deploy/configProyecto.users` traen una cuenta de laboratorio:

```text
Usuario: admin
Password: linux
```

El archivo almacena `usuario:SHA256(password)`, no la contraseña. Genera hashes para otras cuentas con:

```bash
printf %s 'TuPassword' | sha256sum
```

Agrega una línea `usuario:hash_de_64_caracteres` al archivo indicado por `UsersFile`. No reutilices la cuenta ni la contraseña de ejemplo.

### Registro persistente y rotación

El servidor escribe cada evento en dos destinos:

- **stdout**, que systemd recoge en journald (`journalctl -u servidor-operativos`).
- **archivo persistente**, indicado por `LogFile` (por defecto `/var/log/proyecto-operativos/servidor.log`).

Cada línea es un registro estructurado de `log/slog`, por ejemplo:

```text
time=2026-11-16T15:00:00.000-05:00 level=INFO msg="comando recibido" ip=100.66.232.76 comando="ls -la"
```

Se registran conexiones entrantes y cerradas, IP no autorizada, autenticaciones correctas y fallidas (usuario, intentos restantes y espera), bloqueos, comandos recibidos y finalizados (código de salida y duración) y el arranque/parada del servidor. La salida de los comandos no se guarda en el log. Como el texto de cada comando sí se registra, evita pasar secretos en la línea de comandos. El archivo rota al superar `LogMaxSizeMB`; se conservan `LogBackups` archivos anteriores como `servidor.log.1`, `servidor.log.2`, etc. Si el archivo no se puede abrir y no se indicó `-log`, el servidor continúa solo con stdout y muestra un aviso.

## Ejecutar en Debian (servidor)

Desde la raíz del proyecto:

```bash
mkdir -p dist/linux
go build -o dist/linux/servidor-operativos ./servidor
./dist/linux/servidor-operativos -config servidor/server.conf
```

También se puede iniciar directamente:

```bash
go run ./servidor -config servidor/server.conf
```

El servidor escucha en `ListenIP:Port` y arranca su shell en el directorio desde el que se lanzó. Para detenerlo, usa `Ctrl+C`.

## Instalar como servicio systemd (Debian)

La unidad incluida usa un usuario dedicado `operativos` (sin shell de inicio de sesión), arranca `/usr/local/bin/servidor-operativos` y lee `/etc/configProyecto.conf`. No se recomienda `nobody`: un usuario dedicado permite dar permisos solo a sus directorios de estado y logs.

### 1. Crear el usuario y los directorios

```bash
getent group operativos >/dev/null || sudo groupadd --system operativos
id -u operativos >/dev/null 2>&1 || sudo useradd --system \
  --gid operativos \
  --home-dir /var/lib/servidor-operativos \
  --no-create-home \
  --shell /usr/sbin/nologin \
  operativos

sudo install -d -o operativos -g operativos -m 0750 \
  /var/lib/servidor-operativos /var/log/proyecto-operativos
```

### 2. Compilar e instalar el binario

```bash
mkdir -p dist/linux
go build -trimpath -ldflags="-s -w" -o dist/linux/servidor-operativos ./servidor
sudo install -o root -g root -m 0755 dist/linux/servidor-operativos /usr/local/bin/servidor-operativos
```

### 3. Instalar la configuración

```bash
sudo install -o root -g operativos -m 0640 deploy/configProyecto.conf /etc/configProyecto.conf
sudo install -o root -g operativos -m 0640 deploy/configProyecto.users /etc/configProyecto.users
sudoedit /etc/configProyecto.conf
```

Cambia `AllowedClientIP` por la IP o red VPN del cliente antes de arrancar el servicio. Mantén `LogFile` dentro de `/var/log/proyecto-operativos`: si lo cambias a otra ruta, añádela a `ReadWritePaths=` en la unidad o el logger solo podrá escribir en journald por `ProtectSystem=strict`.

### 4. Instalar y habilitar la unidad

```bash
sudo install -o root -g root -m 0644 deploy/servidor-operativos.service /etc/systemd/system/servidor-operativos.service
sudo systemd-analyze verify /etc/systemd/system/servidor-operativos.service
sudo systemctl daemon-reload
sudo systemctl enable --now servidor-operativos
```

### 5. Verificar permisos, servicio y logging

Antes de probar el cliente, confirma que el usuario `operativos` puede leer la configuración y escribir solo en sus directorios:

```bash
namei -l /etc/configProyecto.conf
namei -l /etc/configProyecto.users
sudo -u operativos test -r /etc/configProyecto.conf && echo "configuración legible"
sudo -u operativos test -r /etc/configProyecto.users && echo "usuarios legible"
sudo -u operativos test -w /var/lib/servidor-operativos && echo "estado escribible"
sudo -u operativos test -w /var/log/proyecto-operativos && echo "logs escribible"
```

Comprueba el estado real del servicio y del proceso:

```bash
systemctl is-enabled servidor-operativos
systemctl is-active servidor-operativos
systemctl status servidor-operativos --no-pager
systemctl show servidor-operativos -p User -p Group -p ExecStart -p Restart
pgrep -u operativos -af servidor-operativos
sudo ss -ltnp 'sport = :2306'
```

`is-enabled` debe responder `enabled` e `is-active` debe responder `active`. El proceso debe pertenecer a `operativos` y el puerto 2306 debe estar escuchando.

Verifica el logging estructurado en el archivo y en journald:

```bash
sudo stat -c '%U:%G %a %n' /var/log/proyecto-operativos/servidor.log
sudo grep -F 'msg="servidor escuchando"' /var/log/proyecto-operativos/servidor.log
journalctl -u servidor-operativos -b -n 50 --no-pager
```

El archivo debe pertenecer a `operativos:operativos` con modo `0640`, y deben aparecer las líneas de arranque `msg="configuración cargada"` y `msg="servidor escuchando"`. Si UFW está activo, permite TCP 2306 solo desde la IP VPN del cliente:

```bash
sudo ufw status verbose
sudo ufw allow from <IP_VPN_CLIENTE> to any port 2306 proto tcp
```

Para validar los eventos de autenticación y comandos, ejecuta el cliente desde Windows con la IP VPN del servidor y el puerto 2306, introduce primero una contraseña incorrecta, autentícate después y ejecuta `pwd`:

```powershell
.\cliente.exe <IP_VPN_SERVIDOR> 2306 5
```

De vuelta en Debian, comprueba el archivo de log:

```bash
sudo grep -F 'autenticación fallida' /var/log/proyecto-operativos/servidor.log
sudo grep -F 'comando recibido' /var/log/proyecto-operativos/servidor.log
sudo grep -F 'comando finalizado' /var/log/proyecto-operativos/servidor.log
sudo tail -n 30 /var/log/proyecto-operativos/servidor.log
```

### 6. Validar el arranque automático tras reiniciar Debian

```bash
sudo reboot
```

Después de reconectar, sin ejecutar `systemctl start` manualmente:

```bash
systemctl is-enabled servidor-operativos
systemctl is-active servidor-operativos
systemctl status servidor-operativos --no-pager
journalctl -b -u servidor-operativos -n 50 --no-pager
sudo tail -n 30 /var/log/proyecto-operativos/servidor.log
pgrep -u operativos -af servidor-operativos
sudo ss -ltnp 'sport = :2306'
```

Si algo falla, revisa primero el error de configuración o de permisos:

```bash
systemctl --failed
journalctl -b -u servidor-operativos -e --no-pager
sudo systemd-analyze verify /etc/systemd/system/servidor-operativos.service
```

### 7. Operación diaria y diagnóstico

```bash
sudo systemctl restart servidor-operativos   # tras editar /etc/configProyecto.conf o users
sudo systemctl stop servidor-operativos
sudo systemctl disable --now servidor-operativos
sudo systemd-analyze security servidor-operativos
```

La unidad aplica `ProtectSystem=strict`, `NoNewPrivileges=true`, `CapabilityBoundingSet=` vacío y `PrivateTmp=true`. Los comandos remotos se ejecutan como `operativos` y solo pueden escribir en `/var/lib/servidor-operativos`, en su `/tmp` privado y en `/var/log/proyecto-operativos`; el resto del sistema queda en solo lectura. Si un comando necesita otra ruta, añádela de forma consciente con `ReadWritePaths=` en la unidad. El servidor nunca debe ejecutarse como `root`: quien se autentique tendría los mismos permisos que el proceso.

## Usar el cliente de consola

La consola de Windows permite iniciar la conexión, ingresar las credenciales, ejecutar comandos y recibir métricas.

### 1. Preparar el servidor

Para conectar clientes desde distintas redes físicas, primero instala y configura una VPN privada (por ejemplo, WireGuard o Tailscale) en Debian y en cada Windows. Asegúrate de que ambos estén conectados a la misma VPN y que Windows tenga una ruta hacia la IP VPN de Debian. La VPN proporciona el acceso cifrado entre redes; esta aplicación no lo hace por sí sola.

En `servidor/server.conf`, configura `AllowedClientIP` con la IP VPN del cliente o con la subred VPN que quieras autorizar. `127.0.0.1` solo sirve cuando cliente y servidor se ejecutan en el mismo equipo. Mantén `ListenIP=0.0.0.0` o usa la IP VPN estable del servidor. Configura el firewall de Debian para permitir TCP 2306 solo desde la VPN; no redirijas ese puerto en el router hacia Internet. La VPN debe estar conectada antes de probar la aplicación.

Inicia el servidor desde la raíz del proyecto:

```bash
go run ./servidor -config servidor/server.conf
```

Déjalo ejecutándose mientras uses el cliente. Si cambias `UsersFile`, recuerda que su ruta relativa se resuelve respecto del archivo de configuración.

### 2. Iniciar el cliente en Windows

Abre PowerShell en la raíz del proyecto y compila el cliente:

```powershell
go build -o cliente.exe .\cliente
```

Ejecuta el cliente con la IP VPN del servidor, el puerto y el período de reporte:

```powershell
.\cliente.exe 10.8.0.1 2306 5
```

Los tres argumentos son obligatorios: `<IP-servidor>`, `<puerto>` y `<periodo-segundos>`. Reemplaza `10.8.0.1` por la IP VPN real de Debian. El puerto debe estar entre 1 y 65535; el período debe ser un entero mayor que 0, no puede superar 3600 segundos ni `MaxReportPeriodSeconds` del servidor. Si falta algún argumento o alguno no es válido, el cliente muestra el uso correcto y termina con código de salida 1.

### 3. Autenticarse y usar la terminal

El cliente solicita las credenciales en este orden:

```text
Usuario: admin
Password:
```

Escribe la contraseña y presiona Enter; no se mostrará mientras la escribes. Para probar en un laboratorio recién clonado, el archivo de ejemplo incluye `admin` / `linux`. Cambia esa cuenta antes de usarlo fuera de una práctica local.

Después de autenticarte puedes escribir cualquier comando de Linux. El historial se mantiene en memoria por conexión: `history` muestra los comandos de la sesión actual (no el `.bash_history` del sistema) y `history -c` lo limpia. Por ejemplo:

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

La terminal de la GUI sigue automáticamente la salida nueva y el panel de reportes mantiene su propio desplazamiento, de modo que recibir métricas no mueve la terminal. Si se agotan los intentos de autenticación, se deshabilita el formulario y aparece un aviso modal; al cerrarlo, la aplicación termina.

La GUI está en su propio módulo para que Fyne no cambie las dependencias del cliente de consola ni del servidor. En Windows, Fyne requiere CGo y un compilador C. Si usas MSYS2 en `C:\msys64`, configura UCRT64 en la misma ventana de PowerShell antes de ejecutar o compilar:

```powershell
$env:CC = 'C:\msys64\ucrt64\bin\gcc.exe'
$env:PATH = "C:\msys64\ucrt64\bin;$env:PATH"
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

La GUI y el cliente de consola son ejecutables separados. El cliente de consola exige los tres argumentos (`cliente <IP-servidor> <puerto> <periodo-segundos>`); para la GUI, esos valores se ingresan en los campos de la ventana.

## Conexión remota desde otra red (Cliente Externo)

Si ejecutas el cliente desde otra casa o red distinta a la del servidor Debian, **no expongas ni redirijas el puerto TCP 2306 directamente a Internet**. El protocolo de esta aplicación no cifra el tráfico, por lo que los comandos, las respuestas y las métricas podrían ser observados. Usa una VPN privada como Tailscale para conectar ambos equipos; la VPN cifra el trayecto entre ellos.

### 1. Instalar y conectar Tailscale en ambos equipos

1. En Debian, instala Tailscale siguiendo la [guía oficial para Linux](https://tailscale.com/kb/1031/install-linux) y conéctalo a tu tailnet:

   ```bash
   sudo tailscale up
   ```

   Completa el inicio de sesión indicado. En Windows, instala Tailscale desde su [página oficial](https://tailscale.com/download/windows), inicia sesión con la misma cuenta/tailnet y confirma que ambos dispositivos aparezcan conectados.
2. En Debian, consulta la IP de Tailscale del **servidor**:

   ```bash
   tailscale ip -4
   ```

   También puedes revisar el estado de los dispositivos con `tailscale status`.
3. En PowerShell de Windows, consulta la IP de Tailscale del **cliente**:

   ```powershell
   tailscale ip -4
   ```

   Mantén la VPN conectada en ambos equipos mientras uses la aplicación.

### 2. Autorizar el cliente en Debian

Edita `servidor/server.conf` en el servidor y configura `AllowedClientIP` con la IP de Tailscale del **cliente Windows** obtenida en el paso anterior. Este valor autoriza el origen de la conexión; no es la IP del servidor que usarás para conectarte. Por ejemplo:

```ini
ListenIP=0.0.0.0
Port=2306
AllowedClientIP=100.83.168.60
MaxAuthAttempts=3
UsersFile=users.conf
DefaultReportPeriodSeconds=5
MaxReportPeriodSeconds=60
```

Reemplaza `100.83.168.60` por la IP real del cliente. También puedes autorizar una subred Tailscale usando su notación CIDR, si necesitas permitir varios clientes. Conserva los demás parámetros requeridos que ya tenga tu archivo. `ListenIP=0.0.0.0` permite escuchar en las interfaces del servidor; configura el firewall de Debian para permitir TCP 2306 solo desde la VPN y no crees una regla de redirección del puerto en el router. Si el servidor ya estaba ejecutándose, detenlo con `Ctrl+C` y vuelve a iniciarlo desde la raíz del proyecto para que lea la nueva configuración:

```bash
go run ./servidor -config servidor/server.conf
```

### 3. Conectar desde Windows

Desde PowerShell, en la raíz del proyecto, compila el cliente de consola:

```powershell
go build -o cliente.exe .\cliente
```

Conéctate usando la IP de Tailscale del **servidor Debian**, el puerto y el período:

```powershell
.\cliente.exe <IP_TAILSCALE_SERVIDOR> 2306 5
```

Sustituye el marcador por la IP real, por ejemplo `.\cliente.exe 100.83.168.60 2306 5`. Los tres argumentos son obligatorios: IP, puerto y período en segundos.

Para iniciar la interfaz gráfica Fyne desde la raíz del proyecto:

```powershell
go -C .\src\opergui run .
```

En la ventana, ingresa la IP de Tailscale del servidor y el puerto `2306`. Si vas a compilar o ejecutar la GUI localmente en Windows, configura primero `CC`, `PATH` y `CGO_ENABLED` para MSYS2 UCRT64 como se indica en la sección **Usar la interfaz gráfica Fyne**. En las opciones de Tailscale de la organización, asegúrate también de que las reglas de acceso permitan que el cliente alcance el servidor.

## Ejecutar el shell local

En Linux, desde la raíz:

```bash
go run ./src/shell.go
```

También ejecuta cualquier comando de tu propia shell y conserva el directorio de trabajo entre comandos. Escribe `exit` o `bye` para salir.

## Pruebas

```bash
go test ./cliente ./servidor ./src/logging ./src/login ./src/metrics ./src/shell ./src/sockets
```

Las pruebas de `src/shell` ejecutan `bash`/`sh`, así que se omiten en Windows o donde no haya una shell POSIX disponible.

## Preparar dependencias y ejecutables

Los scripts de preparación instalan Go y Tailscale si hacen falta, descargan las dependencias de Go y compilan los ejecutables. La conexión de Tailscale requiere iniciar sesión manualmente en cada equipo; el script no cambia la configuración de red ni abre puertos.

### Windows: cliente de consola y GUI

Abre PowerShell como administrador desde la raíz del proyecto y ejecuta:

```powershell
.\preparar-windows.ps1
```

Si PowerShell bloquea la ejecución del archivo, permite scripts solo en esa ventana y vuelve a ejecutarlo:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\preparar-windows.ps1
```

El script instala Go, Tailscale y MSYS2 con GCC UCRT64, configura `CC`, `PATH` y `CGO_ENABLED` durante la compilación y crea:

- `dist\windows\cliente.exe`
- `dist\windows\cliente-gui.exe`

Si Windows solicita reiniciar o cerrar sesión después de instalar una herramienta, hazlo y vuelve a ejecutar el script.

### Debian: servidor

Ejecuta el instalador de dependencias:

```bash
bash ./preparar-debian.sh
```

También puedes usar **Ejecutar como un programa** desde el administrador de archivos. El script abrirá una terminal y solicitará autorización de administrador mediante `pkexec`; no hace falta agregar tu usuario al grupo `sudo`. Si no aparece la solicitud de autorización, ejecútalo desde una terminal o pide al administrador que habilite PolicyKit. Este script solo instala/verifica dependencias; no compila ni inicia el servidor.

Configura `servidor/server.conf` (o `/etc/configProyecto.conf` si ya instalaste el servicio) para autorizar la IP VPN del cliente y luego inicia el servidor con el lanzador:

```bash
bash ./iniciar-servidor-debian.sh
```

El lanzador usa `/etc/configProyecto.conf` si existe; si no, cae en `servidor/server.conf`. Puedes forzar otra ruta con `OPERATIVOS_CONFIG=/ruta/config.conf bash ./iniciar-servidor-debian.sh`. Ejecuta `go run ./servidor` en una terminal y mantiene el servidor activo hasta detenerlo con `Ctrl+C`. También puedes usar **Ejecutar como un programa** sobre el lanzador. Inicia sesión en Tailscale antes de conectar el cliente.

El cliente y el servidor siguen necesitando conectividad por la VPN. No publiques el puerto TCP 2306 directamente en Internet.
