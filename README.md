# Comunicación entre terminales en Go

Prototipo educativo en Go para explorar la comunicación entre dos equipos mediante sockets TCP. La idea es enviar desde un equipo una instrucción destinada a otro sistema operativo y observar el intercambio entre cliente y servidor.

> **Estado actual:** el cliente envía una línea de texto por TCP y el servidor la recibe y la muestra en pantalla. El servidor todavía **no ejecuta** el comando recibido ni devuelve el resultado. El programa de `shell.go` ejecuta comandos localmente y no está conectado al cliente ni al servidor.

## Objetivo

El proyecto reúne ejemplos iniciales relacionados con sistemas operativos y comunicación entre procesos:

- Establecer una conexión TCP entre un cliente y un servidor.
- Enviar y recibir texto a través de la conexión.
- Leer comandos desde una terminal y ejecutarlos localmente con Bash.
- Probar, por separado, un inicio de sesión sencillo con contraseña.

La intención es servir como punto de partida para estudiar cómo diseñar una solución de administración remota entre sistemas. No es todavía una herramienta de ejecución remota completa.

## Estructura

```text
.
├── src/
│   ├── shell.go                 # Lee y ejecuta comandos en la máquina local
│   ├── saludo.go                # Ejemplo sencillo de entrada y salida
│   ├── sockets/
│   │   ├── serverOper.go        # Escucha una conexión TCP y muestra el texto recibido
│   │   └── clientOper.go        # Se conecta al servidor y envía un texto fijo
│   └── login/
│       ├── go.mod               # Módulo independiente para ejemplos de inicio de sesión
│       ├── login.go
│       └── archivos.go
└── README.md
```

Los programas de `src/` son ejemplos separados y tienen sus propias funciones `main`; por eso se ejecutan individualmente, no como un único programa.

## Requisitos

- Go instalado.
- Linux o un entorno con Bash para ejecutar `shell.go`.
- Para probar cliente y servidor en equipos distintos, conectividad de red entre ellos y acceso al puerto TCP `2710`.

## Probar cliente y servidor

1. En `src/sockets/serverOper.go`, configura la dirección IP del equipo que actuará como servidor. Actualmente el programa intenta escuchar en `10.1.15.66:2710`; esa dirección debe existir en el equipo servidor.
2. En la terminal del equipo servidor, desde la raíz del repositorio, inicia el servidor:

   ```bash
   go run ./src/sockets/serverOper.go
   ```

3. En otra terminal, en el equipo cliente, pasa al programa la dirección IP del servidor:

   ```bash
   go run ./src/sockets/clientOper.go 10.1.15.66
   ```

   Sustituye `10.1.15.66` por la dirección configurada en el servidor.

El cliente envía actualmente el texto fijo `ls -l desde aortiz...`. El servidor lo muestra como texto recibido y cierra la conexión. No se envía una respuesta al cliente.

## Ejecutar el ejemplo de shell local

Para iniciar el lector de comandos local:

```bash
go run ./src/shell.go
```

El programa ejecuta lo que se escriba mediante `bash -c` en **la misma máquina** donde se está ejecutando. Escribe `exit` para salir. Este ejemplo no recibe comandos por la red y no forma parte del flujo TCP.

## Ejemplo de inicio de sesión

El código de `src/login/` es un módulo Go separado. Para probar el ejemplo de contraseña, entra en ese directorio y ejecútalo:

```bash
cd src/login
go run login.go
```

El módulo puede necesitar descargar las dependencias Go declaradas en `go.mod`. Este ejemplo no está integrado con los programas de sockets.

## Limitaciones y seguridad

- La comunicación actual no tiene cifrado ni autenticación entre cliente y servidor.
- El cliente envía un texto fijo; no permite introducir comandos de forma interactiva.
- El servidor solo recibe e imprime ese texto. No lo ejecuta ni devuelve resultados.
- `shell.go` ejecuta comandos locales con Bash. No expongas ese comportamiento a una red ni conectes la recepción de comandos con su ejecución sin diseñar controles de acceso, validación, autorización y transporte seguro.
- El servidor usa una dirección IP fija en el código y acepta una sola conexión antes de finalizar.

Por estas razones, el proyecto debe usarse únicamente en un entorno de laboratorio controlado. No debe considerarse un sistema seguro de administración remota.