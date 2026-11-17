#!/bin/bash
# Script automatizado de despliegue para el servidor de sistemas operativos

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
go build -trimpath -ldflags="-s -w" -o dist/linux/servidor-operativos ./servidor
sudo install -o root -g root -m 0755 dist/linux/servidor-operativos /usr/local/bin/servidor-operativos

# 4. Instalar archivos de configuracion
echo "Instalando configuracion en /etc/..."
sudo install -o root -g operativos -m 0640 deploy/configProyecto.conf /etc/configProyecto.conf
sudo install -o root -g operativos -m 0640 deploy/configProyecto.users /etc/configProyecto.users

# 5. Instalar unidad de systemd
echo "Configurando servicio systemd..."
sudo install -o root -g root -m 0644 deploy/servidor-operativos.service /etc/systemd/system/servidor-operativos.service

# 6. Recargar systemd y arrancar servicio
sudo systemctl daemon-reload
sudo systemctl enable --now servidor-operativos

echo "=== ¡Instalacion y arranque completados con exito! ==="
sudo systemctl status servidor-operativos --no-pager