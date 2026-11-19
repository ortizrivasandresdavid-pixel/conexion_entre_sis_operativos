#!/bin/bash
# Script automatizado de despliegue para el servidor de sistemas operativos

# Función para mantener la ventana abierta al terminar
pausar() {
    echo ""
    echo "=================================================="
    read -p "Proceso finalizado. Presiona [ENTER] para cerrar esta ventana..."
}

# Si ocurre algún error en medio del script, atrapalo y avísale al usuario sin cerrar la terminal
trap 'echo ""; echo "❌ Error: Algo falló durante la instalación."; pausar; exit 1' ERR

set -e
echo "=== Iniciando instalacion formal del servidor en Debian ==="

# 1. Crear usuario y grupo system dedicado
getent group operativos >/dev/null || sudo groupadd --system operativos
id -u operativos >/dev/null 2>&1 || sudo useradd --system \
  --gid operativos \
  --home-dir /var/lib/servidor-operativos \
  --no-create-home \
  --shell /usr/sbin/nologin \
  operativos

# 2. Crear directorios de estado y logs
sudo install -d -o operativos -g operativos -m 0750 \
  /var/lib/servidor-operativos /var/log/proyecto-operativos

# 3. Compilar el binario del servidor
echo "Compilando binario de Go..."
mkdir -p dist/linux
/usr/local/go/bin/go build -trimpath -ldflags="-s -w" -o dist/linux/servidor-operativos ./servidor
sudo install -o root -g root -m 0755 dist/linux/servidor-operativos /usr/local/bin/servidor-operativos

# 4. Instalar archivos de configuracion
echo "Instalando configuracion en /etc/..."
sudo install -o root -g operativos -m 0640 deploy/configProyecto.conf /etc/configProyecto.conf
# El archivo de usuarios debe ser escribible por el grupo operativos para que
# la GUI pueda registrar cuentas. Si ya existe se conserva: reinstalar no debe
# borrar los usuarios creados desde la aplicacion.
if [ -f /etc/configProyecto.users ]; then
  sudo chown root:operativos /etc/configProyecto.users
  sudo chmod 0660 /etc/configProyecto.users
else
  sudo install -o root -g operativos -m 0660 deploy/configProyecto.users /etc/configProyecto.users
fi

# 5. Instalar unidad de systemd
echo "Configurando servicio systemd..."
sudo install -o root -g root -m 0644 deploy/servidor-operativos.service /etc/systemd/system/servidor-operativos.service

# 6. Recargar systemd y arrancar servicio
sudo systemctl daemon-reload
sudo systemctl enable --now servidor-operativos

echo "=== ¡Instalacion y arranque completados con exito! ==="
sudo systemctl status servidor-operativos --no-pager

# Llamar a la pausa para que la ventana permanezca abierta
pausar