# Administración y monitorización de sistemas en Go

Proyecto académico multiplataforma: servidor en Debian/Linux y clientes para Windows (consola y GUI Fyne). El sistema permite autenticarse, ejecutar comandos remotos y recibir métricas periódicas del servidor.

> **Uso restringido a entornos controlados.** El servidor ejecuta comandos arbitrarios con los permisos de su proceso. El protocolo TCP de la aplicación no está cifrado: para conectar equipos distintos, utiliza una VPN privada y no expongas el puerto del servicio a Internet.

## Tabla de contenidos

- [Resumen del proyecto y arquitectura rápida](#resumen-arquitectura)
  - [Arquitectura](#arquitectura)
  - [Seguridad y límites operativos](#seguridad-limites)
- [Requisitos del sistema](#requisitos)
- [Guía de inicio rápido (desarrollo local)](#inicio-rapido)
- [Despliegue en producción](#despliegue-produccion)
  - [Red privada y autorización del cliente](#red-privada)
  - [Instalación automatizada con systemd](#instalacion-systemd)
  - [Monitor CLI pasivo del servidor](#monitor-servidor)
  - [Verificación y operación](#verificacion-operacion)
- [Uso del cliente](#uso-cliente)
  - [Consola](#cliente-consola)
  - [Interfaz gráfica Fyne](#cliente-fyne)
- [Solución de problemas frecuentes](#solucion-problemas)

<a id="resumen-arquitectura"></a>
## Resumen del proyecto y arquitectura rápida

El servidor escucha en TCP, autoriza la IP/CIDR del cliente, autentica usuarios con un archivo plano de hashes SHA-256, permite registrar cuentas nuevas desde la GUI y administra una sesión de comandos. Durante la sesión reporta CPU, memoria, procesos y espacio usado en la partición raíz (`/`). Los mensajes de aplicación son JSON delimitado por líneas.

El flujo técnico —despliegue systemd, logs estructurados, métricas, cliente de consola y GUI Fyne— está implementado, probado y verificado en un entorno real con Debian y Windows.

<a id="arquitectura"></a>
### Arquitectura

```text
┌────────────────────────────── Windows ──────────────────────────────┐
│  Cliente de consola                 Cliente GUI (Fyne)               │
│  cliente/main.go                    src/opergui/                     │
└───────────────────────┬──────────────────────────┬──────────────────┘
                        └──── TCP / JSON-lines ────┘
                                   │
                           VPN privada recomendada
                                   │
┌──────────────────────────── Debian/Linux ────────────────────────────┐
│  servidor/ → src/sockets/ → autenticación, shell y métricas          │
│                         ↘ logs y eventos JSONL                       │
│  systemd provisiona cuentas; cada shell baja al UID/GID autenticado  │
│  src/servercli/ observa eventos y recursos sin controlar el servicio  │
└───────────────────────────────────────────────────────────────────────┘
```

<a id="seguridad-limites"></a>
### Seguridad y límites operativos

- La conexión TCP de la aplicación no cifra comandos, respuestas ni métricas. Para conectar equipos distintos, utiliza una VPN privada (por ejemplo, Tailscale o WireGuard); no publiques ni redirijas el puerto `2306` a Internet.
- El servidor no aplica una lista blanca: un usuario autenticado puede ejecutar comandos con los permisos de su cuenta Linux. El daemon requiere root y capacidades acotadas para crear usuarios y cambiar credenciales; cada comando se ejecuta con el UID/GID autenticado, no como root. `NoNewPrivileges=true` impide que los shells ganen privilegios mediante ejecutables SUID.
- La autenticación usa un desafío aleatorio y una prueba basada en SHA-256. La contraseña no se envía directamente; este mecanismo no sustituye TLS ni un algoritmo especializado de almacenamiento de contraseñas.
- El monitor CLI del servidor lee un flujo JSONL local (`0644`); incluye comandos recibidos, usuarios e IP de cliente. No envíes secretos en los comandos ni expongas ese archivo a usuarios no confiables.
- El registro desde la GUI permite crear cuentas de aplicación y cuentas Linux regulares con `/home/<usuario>`; la contraseña de aplicación no se asigna a la cuenta Linux. El nombre debe cumplir `^[a-z_][a-z0-9_-]{0,31}$`. El hash SHA-256 de la contraseña viaja por el canal TCP; en un canal sin cifrar equivale a la credencial. Restringe `AllowedClientIP` y usa una VPN privada.
- Una sesión admite un comando a la vez; cada comando tiene un límite de cinco minutos y una salida máxima de 2 MiB. No hay TTY ni ejecución interactiva para programas como `vim`, `top` interactivo o `sudo` que soliciten contraseña.
- El servidor admite hasta 32 conexiones concurrentes y limita los intentos fallidos por IP. El cliente no se reconecta automáticamente.

<a id="requisitos"></a>
## Requisitos del sistema

| Componente | Requisito |
|---|---|
| Servidor | Debian/Linux, Go 1.27.3 o posterior, `bash` o `sh`; systemd para el despliegue de producción. |
| Monitor CLI del servidor | Debian/Linux y Go 1.27.3 o posterior; no requiere Fyne, CGo ni entorno gráfico. |
| Cliente de consola | Windows y Go 1.27.3 o posterior para compilar desde el código fuente. |
| Cliente gráfico | Windows, Go, CGo habilitado y un compilador C compatible con Fyne (por ejemplo, GCC de MSYS2). |
| Red entre equipos | VPN privada activa, IP del cliente autorizada en `AllowedClientIP` y TCP `2306` permitido solo por la red privada. |

El recolector Linux lee CPU, procesos y memoria desde `/proc`, y mide el uso del sistema de archivos raíz con `syscall.Statfs_t`.

<details>
<summary>Preparar dependencias en los equipos</summary>

En Debian, el script de preparación instala o verifica Go y Tailscale. La autenticación de Tailscale debe completarse manualmente:

```bash
bash ./preparar-debian.sh
```

En Windows, abre PowerShell como administrador desde la raíz del repositorio:

```powershell
.\preparar-windows.ps1
```

Si la política de PowerShell bloquea el script, permite su ejecución solo en la sesión actual y vuelve a ejecutarlo:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
.\preparar-windows.ps1
```

El preparador de Windows instala Go, Tailscale y las herramientas de compilación de Fyne, y genera `dist\windows\cliente.exe` y `dist\windows\cliente-gui.exe`.

</details>

<a id="inicio-rapido"></a>
## Guía de inicio rápido (desarrollo local)

Ejecuta estos pasos desde la raíz del repositorio. Antes de conectar equipos distintos, configura la VPN y la IP permitida siguiendo [Red privada y autorización del cliente](#red-privada).

<details>
<summary>1. Configurar y ejecutar el servidor de desarrollo</summary>

Edita `servidor/server.conf` y establece `AllowedClientIP` con la IP del cliente que se conectará (o con una subred CIDR autorizada). El archivo de usuarios se indica en `UsersFile`; cada entrada debe tener el formato `usuario:hash_sha256`.

Para generar un hash de contraseña en Debian:

```bash
printf %s 'ReemplazaEstaContraseña' | sha256sum
```

Añade el usuario y el hash de 64 caracteres al archivo configurado en `UsersFile`. No reutilices credenciales de ejemplo.

En desarrollo, el usuario indicado también debe existir en Linux y tener home `/home/<usuario>` (o ejecuta el servidor con privilegios para que pueda provisionarse). El instalador Debian prepara el servicio y las cuentas iniciales automáticamente.

Arranca el servidor desde la raíz del proyecto:

```bash
go run ./servidor -config servidor/server.conf
```

El proceso permanece en primer plano hasta recibir `Ctrl+C`. El período predeterminado de métricas y sus límites se configuran en `servidor/server.conf`.

</details>

<details>
<summary>2. Compilar y conectar el cliente de consola desde Windows</summary>

En PowerShell, desde la raíz del repositorio:

```powershell
go build -o cliente.exe .\cliente
.\cliente.exe <IP_VPN_SERVIDOR> 2306 5
```

El cliente requiere exactamente tres argumentos: IP del servidor, puerto y período en segundos. El puerto debe ser `1..65535`; el período debe ser mayor que cero, no puede superar 3600 y tampoco puede exceder `MaxReportPeriodSeconds` del servidor.

</details>

<details>
<summary>3. Ejecutar las pruebas automatizadas</summary>

```bash
go build ./...
go test ./cliente ./servidor ./src/logging ./src/login ./src/metrics ./src/shell ./src/sockets
go test ./src/servercli
(cd src/opergui && go build ./...)
```

Las pruebas de `src/shell` necesitan `bash` o `sh` y se omiten en Windows o en entornos sin una shell POSIX.

</details>

<a id="despliegue-produccion"></a>
## Despliegue en producción

El despliegue Debian utiliza `instalar-servidor.sh` y la unidad `deploy/servidor-operativos.service`. El instalador prepara los directorios, compila el servidor, instala configuración y usuarios bajo `/etc/`, provisiona las cuentas iniciales como usuarios Linux regulares y habilita e inicia el servicio.

<a id="red-privada"></a>
### Red privada y autorización del cliente

La VPN proporciona el cifrado del trayecto; el protocolo de la aplicación no cifra TCP. Mantén Debian y Windows conectados a la misma VPN y no abras el puerto `2306` hacia Internet. `AllowedClientIP` es la IP **de origen del cliente Windows**, no la dirección del servidor.

Con Tailscale, consulta las direcciones con `tailscale ip -4` en ambos equipos. Antes de instalar, edita `deploy/configProyecto.conf` y sustituye el valor de `AllowedClientIP` por la IP VPN actual del cliente. El instalador copia ese archivo a `/etc/configProyecto.conf`. Si cambia la IP del cliente, actualiza el archivo de producción y reinicia el servicio:

```bash
sudoedit /etc/configProyecto.conf
sudo systemctl restart servidor-operativos
```

`AllowedClientIP` admite una IP individual o un bloque CIDR. `ListenIP=0.0.0.0` escucha en todas las interfaces; por eso limita el firewall a la VPN.

<details>
<summary>Parámetros principales de configuración</summary>

| Parámetro | Uso |
|---|---|
| `ListenIP` / `Port` | Dirección y puerto TCP del servidor (puerto habitual: `2306`). |
| `AllowedClientIP` | IP o CIDR de los clientes autorizados; para este despliegue, usa la IP VPN de Windows. |
| `MaxAuthAttempts` | Intentos de autenticación permitidos antes de aplicar el bloqueo temporal. |
| `UsersFile` | Archivo plano de usuarios y hashes; en producción, `/etc/configProyecto.users`. |
| `DefaultReportPeriodSeconds` | Período inicial de envío de métricas. |
| `MaxReportPeriodSeconds` | Período máximo que el cliente puede solicitar (hasta 3600 segundos). |
| `LogFile` | Archivo persistente; la ruta de producción predeterminada es `/var/log/proyecto-operativos/servidor.log`. |
| `EventFile` | Flujo JSONL local para el monitor del servidor; por defecto, `eventos-gui.jsonl` junto a `LogFile`. |
| `LogMaxSizeMB` / `LogBackups` | Tamaño antes de rotar y cantidad de archivos anteriores a conservar. |

Las plantillas para desarrollo y producción están en `servidor/server.conf` y `deploy/configProyecto.conf`, respectivamente. Las rutas relativas de archivos de configuración se resuelven desde el directorio del propio `.conf`.

</details>

<details>
<summary>Pasos de conexión de Tailscale</summary>

1. Instala Tailscale en Debian y Windows y autentica ambos dispositivos en la misma red privada.
2. En cada dispositivo, consulta su dirección:

   ```bash
   tailscale ip -4
   ```

3. Usa la dirección mostrada en Windows como `AllowedClientIP` en el servidor. Usa la dirección mostrada en Debian como IP destino del cliente.
4. Si UFW está activo, permite el puerto solo desde el cliente:

   ```bash
   sudo ufw allow from <IP_VPN_CLIENTE> to any port 2306 proto tcp
   ```

Sustituye el marcador por la IP real. No crees una regla de redirección del puerto en el router. En Tailscale, verifica también que las políticas de acceso permitan al cliente alcanzar el servidor.

</details>

<a id="instalacion-systemd"></a>
### Instalación automatizada con systemd

El instalador usa `/usr/local/go/bin/go` y rutas relativas al repositorio. Comprueba esa ruta y ejecuta el script desde la raíz del proyecto:

```bash
/usr/local/go/bin/go version
chmod +x ./instalar-servidor.sh
./instalar-servidor.sh
```

También se puede ejecutar con Bash:

```bash
bash ./instalar-servidor.sh
```

El usuario actual debe poder usar `sudo`. El script instala la configuración, legible por el monitor CLI (`root:root 0644`), el archivo de usuarios (`root:root 0600`), el binario y la unidad systemd. Crea cada usuario inicial que falte con `useradd -m -d /home/<usuario> -s /bin/bash`, verifica que sea una cuenta regular y asegura que su home pertenezca a ese usuario. Al final imprime el estado y siempre solicita Enter; desde el administrador de archivos, selecciona **Ejecutar en un terminal** para conservar la ventana abierta. Si `/etc/configProyecto.users` ya existe, conserva su contenido y corrige propietario y permisos.

> Revisa `deploy/configProyecto.conf` antes de instalar: el valor de `AllowedClientIP` debe ser la IP VPN del cliente autorizado. Si editas el archivo de `/etc/` después de la instalación, reinicia systemd como se indica arriba.

<details>
<summary>Configuración de usuarios y archivos instalados</summary>

El servicio carga la configuración desde `/etc/configProyecto.conf` y las credenciales desde la ruta indicada por `UsersFile` (por defecto, `/etc/configProyecto.users`). Las entradas de usuario tienen el formato:

```text
usuario:hash_sha256_de_64_caracteres
```

Genera un hash con:

```bash
printf %s 'ReemplazaEstaContraseña' | sha256sum
```

Reemplaza cualquier credencial de ejemplo antes de utilizar el sistema fuera de una práctica local. Restringe el acceso a los archivos de credenciales; no almacenes contraseñas en texto claro.

La GUI puede crear cuentas desde la pestaña **Registrarse** del Paso 2. El servidor valida el nombre y el hash, crea primero la cuenta Linux regular con shell `/bin/bash` y home `/home/<usuario>`, persiste `usuario:hash_sha256` y confirma el alta. Si falla la provisión, el registro se rechaza; el servidor no permite iniciar sesión con una cuenta cuyo UID sea menor que 1000 o cuyo home no sea `/home/<usuario>`. Las cuentas de Linux se crean sin contraseña de acceso del sistema: la autenticación pertenece a la aplicación. El archivo se instala como `root:root 0600`; el servicio necesita root para `useradd` y para cambiar las credenciales de los procesos hijos.

</details>

<a id="monitor-servidor"></a>
### Monitor CLI pasivo del servidor

El monitor es una aplicación de terminal en `src/servercli` y no depende de Fyne, CGo ni paquetes gráficos. Se ejecuta como proceso independiente del daemon: lee `/var/log/proyecto-operativos/eventos-gui.jsonl` y consulta `/proc`/el recolector local de recursos, pero no abre el puerto de clientes, controla systemd ni modifica las conexiones. Iniciarlo o salir de él no detiene ni reinicia el servidor.

Desde la raíz del repositorio, ejecútalo en una terminal interactiva:

```bash
bash ./iniciar-cli-servidor.sh
```

El lanzador compila y ejecuta solo el monitor, conserva la terminal abierta al terminar o fallar y no necesita privilegios de root. Usa `c` + Enter para limpiar las listas visibles, `q` + Enter o `Ctrl+C` para salir. Si se configura `EventFile` en `/etc/configProyecto.conf`, el monitor utiliza esa ruta; también acepta `-config` o `-events`. El archivo conserva modo `0644` para permitir la lectura al usuario que ejecuta el monitor y contiene comandos, nombres de usuario e IP; no envíes secretos y protege el archivo frente a usuarios no confiables.

<a id="verificacion-operacion"></a>
### Verificación y operación

<details>
<summary>Verificar servicio, permisos, puerto y logs</summary>

Estado del servicio, usuario y socket:

```bash
systemctl is-enabled servidor-operativos
systemctl is-active servidor-operativos
systemctl status servidor-operativos --no-pager
systemctl show servidor-operativos -p User -p Group -p ExecStart -p Restart
ps -C servidor-operativos -o user,group,pid,args
sudo ss -ltnp 'sport = :2306'
```

El servicio debe estar `enabled` y `active`, ejecutarse como `root` con la unidad endurecida y escuchar en el puerto configurado. Tras iniciar sesión desde el cliente, comprueba `whoami`, `pwd` y `printf '%s\n' "$HOME"`: deben indicar el nombre autenticado y `/home/<usuario>`.

Permisos de configuración y directorios:

```bash
namei -l /etc/configProyecto.conf
namei -l /etc/configProyecto.users
sudo test -r /etc/configProyecto.conf && echo "configuración legible"
sudo test -r /etc/configProyecto.users && echo "usuarios legible"
sudo test -w /etc/configProyecto.users && echo "usuarios escribible"
sudo test -w /var/lib/servidor-operativos && echo "estado escribible"
sudo test -w /var/log/proyecto-operativos && echo "logs escribible"
```

Logs estructurados: se escriben en journald y en `/var/log/proyecto-operativos/servidor.log`; la rotación se configura con `LogMaxSizeMB` y `LogBackups`. Se registran conexiones, autenticaciones y comandos (no sus salidas). Como sí se registra el texto del comando, no incluyas secretos en los comandos enviados.

El flujo para el monitor se encuentra en `/var/log/proyecto-operativos/eventos-gui.jsonl` (o en la ruta configurada con `EventFile`). El CLI lo sigue en vivo y no altera el proceso servidor:

```bash
stat -c '%U:%G %a %n' /var/log/proyecto-operativos/eventos-gui.jsonl
tail -n 20 /var/log/proyecto-operativos/eventos-gui.jsonl
```

```bash
journalctl -u servidor-operativos -b -n 50 --no-pager
sudo stat -c '%U:%G %a %n' /var/log/proyecto-operativos/servidor.log
sudo tail -n 50 /var/log/proyecto-operativos/servidor.log
```

La unidad aplica `ProtectSystem=full`, permite acceso a `/home`, protege `/usr` y `/boot` contra escritura, y restringe las capacidades a las necesarias para `useradd`, `chown`, el cambio de UID/GID y cancelar procesos hijos. Mantiene `NoNewPrivileges=true` para que una sesión no pueda elevar privilegios ejecutando binarios SUID.

</details>

<details>
<summary>Administración y diagnóstico de systemd</summary>

```bash
sudo systemctl restart servidor-operativos
sudo systemctl stop servidor-operativos
sudo systemctl disable --now servidor-operativos
systemctl --failed
journalctl -b -u servidor-operativos -e --no-pager
sudo systemd-analyze verify /etc/systemd/system/servidor-operativos.service
sudo systemd-analyze security servidor-operativos
```

Para comprobar el arranque automático, reinicia Debian y vuelve a consultar `systemctl is-active servidor-operativos` y `systemctl is-enabled servidor-operativos`.

</details>

<a id="uso-cliente"></a>
## Uso del cliente

Para equipos distintos, conecta primero a la VPN y usa como destino la IP privada del servidor. La IP que debe aparecer en `AllowedClientIP` es la del cliente Windows.

<a id="cliente-consola"></a>
### Consola

Desde PowerShell, en la raíz del repositorio, compila y ejecuta con IP, puerto y período:

```powershell
go build -o cliente.exe .\cliente
.\cliente.exe <IP_VPN_SERVIDOR> 2306 5
```

El programa valida los tres argumentos y termina con un mensaje de uso si falta alguno o no es válido. Tras autenticarte, escribe comandos Linux; se ejecutan en Debian, no en Windows. El directorio de trabajo se conserva durante la sesión. Usa `history` para ver el historial de la sesión, `history -c` para limpiarlo y `bye` para cerrar la conexión.

<a id="cliente-fyne"></a>
### Interfaz gráfica Fyne

La GUI presenta un flujo en dos pasos antes de la terminal remota y el panel de CPU, memoria y disco:

1. **Conexión al servidor:** campos `Server IP`, `Port` y `Period (s)` con los valores por defecto `100.83.168.60`, `2306` y `5`. El botón **Connect to server** abre el socket TCP y valida la conectividad y la autorización por IP del servidor; si falla, el error aparece en la misma tarjeta sin avanzar.
2. **Acceso de usuario:** dos pestañas del mismo tamaño, **Iniciar sesion** y **Registrarse**, comparten el indicador `Connected to <IP>:<Puerto>`. El login usa la autenticación por desafío y SHA-256 e informa los intentos restantes. El registro valida el nombre (sin `:`), exige que las contraseñas coincidan, calcula el SHA-256 y lo envía al servidor. **Cambiar servidor** cierra el socket y vuelve al Paso 1.

Tras autenticarse o registrarse, la ventana muestra la terminal, el historial de reportes y las métricas. Reutiliza el protocolo de `src/sockets`; la recepción de mensajes ocurre concurrentemente. Solo permite un comando pendiente a la vez.

<details>
<summary>Ejecutar o compilar la GUI en Windows</summary>

Fyne requiere CGo y un compilador C. Si GCC UCRT64 está instalado en `C:\msys64`, configura estas variables en la misma ventana de PowerShell:

```powershell
$env:CC = 'C:\msys64\ucrt64\bin\gcc.exe'
$env:PATH = "C:\msys64\ucrt64\bin;$env:PATH"
$env:CGO_ENABLED = '1'
```

Ejecuta la GUI desde la raíz:

```powershell
go -C .\src\opergui run .
```

Para crear un ejecutable:

```powershell
go -C .\src\opergui build -o ..\..\cliente-gui.exe .
.\cliente-gui.exe
```

En la ventana, introduce la IP VPN del servidor, el puerto `2306` y el período; selecciona **Connect to server**, elige **Iniciar sesion** o **Registrarse** en el Paso 2 y completa las credenciales. Si el servidor autoriza la conexión y el acceso, se abre la terminal y el panel de métricas.

</details>

<a id="solucion-problemas"></a>
## Solución de problemas frecuentes

| Síntoma | Comprobaciones y solución |
|---|---|
| El cliente no conecta | Comprueba que el servicio esté activo, ambos equipos estén conectados a la VPN, el destino sea la IP privada de Debian y TCP `2306` esté permitido por el firewall. |
| `IP no autorizada` | `AllowedClientIP` debe coincidir con la IP VPN de origen del cliente Windows, o incluirla en una red CIDR. Después de editar `/etc/configProyecto.conf`, reinicia el servicio. |
| systemd no inicia el servicio | Revisa `systemctl status servidor-operativos --no-pager`, `journalctl -u servidor-operativos -b -e --no-pager`, los permisos de archivos y `systemd-analyze verify`. |
| No aparecen logs en archivo | Comprueba `LogFile`, el propietario y permisos de `/var/log/proyecto-operativos`; consulta también `journalctl -u servidor-operativos`. |
| El instalador no encuentra Go | Confirma `/usr/local/go/bin/go version`; el instalador requiere el ejecutable en esa ruta absoluta y debe ejecutarse desde la raíz del repositorio. |
| Argumentos del cliente rechazados | Usa exactamente `cliente <IP-servidor> <puerto> <periodo-segundos>`; la IP debe ser válida, el puerto `1..65535` y el período `1..3600` y no mayor que el máximo del servidor. |
| Fyne no compila en Windows | Verifica CGo y GCC, configura `CC`, `PATH` y `CGO_ENABLED` en la misma ventana y vuelve a ejecutar `go -C .\src\opergui build .`. |
| Un comando se queda esperando o no abre una TUI | La sesión no proporciona TTY. Los programas interactivos o que solicitan contraseña por terminal no son compatibles; cada comando se cancela tras cinco minutos. |
| Autenticación temporalmente bloqueada | Respeta el tiempo de espera que informa el cliente. El control de intentos se aplica por IP cliente. |
| Falta `tree` u otra utilidad | La utilidad debe estar instalada en Debian; por ejemplo, un administrador puede instalar `tree` con `sudo apt install tree`. |

Para administración interactiva en producción, utiliza SSH en lugar de esta shell educativa. La aplicación no tiene TTY y no cifra el protocolo TCP.
