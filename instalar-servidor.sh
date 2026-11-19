#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

# 1. Detener servicio systemd si estaba activo para evitar conflictos de puerto o bloqueos
if systemctl is-active --quiet servidor-operativos 2>/dev/null; then
    sudo systemctl stop servidor-operativos || true
fi

# 2. Preparar directorios de configuración y logs con permisos del usuario actual
sudo mkdir -p /var/log/proyecto-operativos /etc/proyecto-operativos
sudo chown -R "$USER:$USER" /var/log/proyecto-operativos /etc/proyecto-operativos

# Copiar configuración si existe en el proyecto y aún no existe en /etc/configProyecto.conf
if [[ -f "$ROOT_DIR/servidor/server.conf" && ! -f /etc/configProyecto.conf ]]; then
    sudo cp "$ROOT_DIR/servidor/server.conf" /etc/configProyecto.conf
    sudo chown "$USER:$USER" /etc/configProyecto.conf
fi

# 3. Detectar versión mínima requerida de Go desde go.mod
MIN_GO_VERSION="$(awk '$1 == "go" { print $2; exit }' "$ROOT_DIR/go.mod")"
if [[ "$MIN_GO_VERSION" =~ ^[0-9]+\.[0-9]+$ ]]; then
    MIN_GO_VERSION="${MIN_GO_VERSION}.0"
fi

GO_CANDIDATES=()
if GO_ON_PATH="$(command -v go 2>/dev/null)"; then
    GO_CANDIDATES+=("$GO_ON_PATH")
fi
[[ -x /usr/local/go/bin/go ]] && GO_CANDIDATES+=(/usr/local/go/bin/go)
for candidate in /usr/local/lib/go*/bin/go; do
    [[ -x "$candidate" ]] && GO_CANDIDATES+=("$candidate")
done

GO_BIN=""
for candidate in "${GO_CANDIDATES[@]}"; do
    candidate_version="$("$candidate" version | sed -E 's/^go version go([0-9]+\.[0-9]+(\.[0-9]+)?).*/\1/')"
    [[ "$candidate_version" =~ ^[0-9]+\.[0-9]+$ ]] && candidate_version="${candidate_version}.0"
    if [[ -n "$candidate_version" ]] &&
        dpkg --compare-versions "$candidate_version" ge "$MIN_GO_VERSION"; then
        GO_BIN="$candidate"
        break
    fi
done

if [[ -z "$GO_BIN" ]]; then
    echo "No se encontró Go $MIN_GO_VERSION o posterior. Ejecuta primero preparar-debian.sh." >&2
    exit 1
fi

# 4. Operar siempre en la raíz del proyecto y resolver configuración
cd "$ROOT_DIR"

CONFIG_PATH="${OPERATIVOS_CONFIG:-/etc/configProyecto.conf}"
if [[ ! -r "$CONFIG_PATH" ]]; then
    CONFIG_PATH="$ROOT_DIR/servidor/server.conf"
    echo "Usando configuración de desarrollo: $CONFIG_PATH"
fi

# 5. Detener instancia previa del usuario si existe e iniciar en segundo plano desde ROOT_DIR
pkill -f "$GO_BIN run ./servidor -config" 2>/dev/null || true
nohup "$GO_BIN" run ./servidor -config "$CONFIG_PATH" "$@" > /var/log/proyecto-operativos/servidor.log 2>&1 &
sleep 1

echo "Instalación completada y servidor iniciado desde: $ROOT_DIR"
echo "Configuración activa: $CONFIG_PATH"
echo "Logs en: /var/log/proyecto-operativos/servidor.log"