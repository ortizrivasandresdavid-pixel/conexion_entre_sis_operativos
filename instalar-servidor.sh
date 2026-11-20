#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

SERVICE_NAME="servidor-operativos"
SERVICE_USER="operativos"
SERVICE_GROUP="operativos"

BIN_PATH="/usr/local/bin/servidor-operativos"
CONF_DEST="/etc/configProyecto.conf"
USERS_DEST="/etc/configProyecto.users"
UNIT_DEST="/etc/systemd/system/${SERVICE_NAME}.service"
STATE_DIR="/var/lib/servidor-operativos"
LOG_DIR="/var/log/proyecto-operativos"

pause_on_exit() {
    local exit_code=$?
    if [[ -t 0 && -t 1 ]]; then
        echo ""
        if [[ $exit_code -ne 0 ]]; then
            echo "[ERROR] La instalación falló con código de salida: $exit_code"
        else
            echo "[OK] Proceso finalizado."
        fi
        read -r -p "Presiona Enter para cerrar..." _ || true
    fi
}
trap pause_on_exit EXIT

run_root() {
    if [[ "$(id -u)" -eq 0 ]]; then
        "$@"
    else
        sudo "$@"
    fi
}

echo "=== Instalador de ${SERVICE_NAME} ==="

# 1. Detener servicio systemd y cualquier instancia previa en ejecución
echo "[1/7] Deteniendo instancias previas..."
if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
    run_root systemctl stop "$SERVICE_NAME" || true
fi
run_root pkill -f "go run ./servidor" 2>/dev/null || true
run_root pkill -x "servidor" 2>/dev/null || true
run_root pkill -x "servidor-operativos" 2>/dev/null || true
sleep 1

# 2. Localizar versión de Go compatible con go.mod
echo "[2/7] Verificando instalación de Go..."
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
    echo "Error: No se encontró Go >= $MIN_GO_VERSION. Ejecuta primero preparar-debian.sh." >&2
    exit 1
fi

# 3. Crear grupo y usuario de sistema sin acceso interactivo
echo "[3/7] Configurando usuario y directorios del sistema..."
if ! getent group "$SERVICE_GROUP" >/dev/null 2>&1; then
    run_root groupadd --system "$SERVICE_GROUP"
fi
if ! id -u "$SERVICE_USER" >/dev/null 2>&1; then
    run_root useradd --system --gid "$SERVICE_GROUP" \
        --home-dir "$STATE_DIR" --no-create-home \
        --shell /usr/sbin/nologin "$SERVICE_USER"
fi

run_root mkdir -p "$STATE_DIR" "$LOG_DIR"
run_root chown -R "${SERVICE_USER}:${SERVICE_GROUP}" "$STATE_DIR" "$LOG_DIR"
run_root chmod 0750 "$STATE_DIR" "$LOG_DIR"

# 4. Compilar el binario de producción e instalarlo en /usr/local/bin
echo "[4/7] Compilando binario del servidor..."
TMP_BIN="$(mktemp /tmp/servidor-operativos.XXXXXX)"
cd "$ROOT_DIR"
"$GO_BIN" build -trimpath -ldflags="-s -w" -o "$TMP_BIN" ./servidor
run_root install -o root -g root -m 0755 "$TMP_BIN" "$BIN_PATH"
rm -f "$TMP_BIN"

# 5. Instalar archivos de configuración y base de usuarios en /etc
echo "[5/7] Instalando configuración en /etc..."
CONF_SRC="$ROOT_DIR/deploy/configProyecto.conf"
[[ ! -f "$CONF_SRC" ]] && CONF_SRC="$ROOT_DIR/servidor/server.conf"

USERS_SRC="$ROOT_DIR/deploy/configProyecto.users"
[[ ! -f "$USERS_SRC" ]] && USERS_SRC="$ROOT_DIR/servidor/users.conf"

if [[ ! -f "$CONF_DEST" ]]; then
    run_root install -o root -g "$SERVICE_GROUP" -m 0640 "$CONF_SRC" "$CONF_DEST"
    # Asegurar que UsersFile y LogFile apunten a las rutas del sistema
    run_root sed -i "s|^UsersFile=.*|UsersFile=${USERS_DEST}|" "$CONF_DEST"
    run_root sed -i "s|^LogFile=.*|LogFile=${LOG_DIR}/servidor.log|" "$CONF_DEST"
else
    echo "      ${CONF_DEST} ya existe; se conserva la configuración actual."
fi

if [[ ! -f "$USERS_DEST" ]]; then
    run_root install -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0640 "$USERS_SRC" "$USERS_DEST"
else
    run_root chown "${SERVICE_USER}:${SERVICE_GROUP}" "$USERS_DEST"
    run_root chmod 0640 "$USERS_DEST"
    echo "      ${USERS_DEST} ya existe; se conservan los usuarios registrados."
fi

# 6. Instalar unidad systemd (permitiendo escritura controlada en /etc/configProyecto.users)
echo "[6/7] Configurando servicio systemd..."
UNIT_SRC="$ROOT_DIR/deploy/${SERVICE_NAME}.service"
if [[ -f "$UNIT_SRC" ]]; then
    run_root install -o root -g root -m 0644 "$UNIT_SRC" "$UNIT_DEST"
    # Garantizar que ProtectSystem=strict permita registrar nuevos usuarios en USERS_DEST
    if run_root grep -q "^ProtectSystem=strict" "$UNIT_DEST" && ! run_root grep -q "^ReadWritePaths=" "$UNIT_DEST"; then
        run_root sed -i "/^ProtectSystem=strict/a ReadWritePaths=${USERS_DEST}" "$UNIT_DEST"
    fi
else
    echo "Error: No se encontró la plantilla ${UNIT_SRC}." >&2
    exit 1
fi

# 7. Habilitar, iniciar y verificar el servicio en segundo plano
echo "[7/7] Iniciando servicio en segundo plano..."
run_root systemctl daemon-reload
run_root systemctl reset-failed "$SERVICE_NAME" 2>/dev/null || true
run_root systemctl enable "$SERVICE_NAME"
run_root systemctl restart "$SERVICE_NAME"
sleep 2

if ! systemctl is-active --quiet "$SERVICE_NAME"; then
    echo "Error: El servicio ${SERVICE_NAME} no logró mantenerse activo." >&2
    run_root systemctl status "$SERVICE_NAME" --no-pager >&2 || true
    exit 1
fi

echo ""
echo "Instalación completada correctamente."
echo "  - Binario:       $BIN_PATH"
echo "  - Configuración: $CONF_DEST"
echo "  - Usuarios:      $USERS_DEST"
echo "  - Logs:          ${LOG_DIR}/servidor.log"
echo "  - Servicio:      $SERVICE_NAME (activo y habilitado al arranque)"