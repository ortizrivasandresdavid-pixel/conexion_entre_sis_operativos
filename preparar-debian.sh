#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

trap 'echo "❌ Error en la línea $LINENO. Revisa el mensaje anterior." >&2' ERR

# Si se ejecuta sin terminal, abrir una terminal visible.
if [[ ! -t 0 || ! -t 1 ]]; then
    SCRIPT_PATH="$(readlink -f -- "${BASH_SOURCE[0]}")"
    TERMINAL_COMMAND='
        script=$1
        shift
        "$script" "$@"
        status=$?
        echo
        if [[ "$status" -eq 0 ]]; then
            read -r -p "Proceso terminado. Presiona Enter para cerrar esta ventana..."
        else
            read -r -p "El proceso falló (código $status). Presiona Enter para cerrar esta ventana..."
        fi
        exit "$status"
    '
    if command -v x-terminal-emulator >/dev/null 2>&1; then
        exec x-terminal-emulator -e bash -c "$TERMINAL_COMMAND" bash "$SCRIPT_PATH" "$@"
    elif command -v gnome-terminal >/dev/null 2>&1; then
        exec gnome-terminal --wait -- bash -c "$TERMINAL_COMMAND" bash "$SCRIPT_PATH" "$@"
    else
        echo "Abre una terminal en $ROOT_DIR y ejecuta: bash ./preparar-debian.sh" >&2
        exit 1
    fi
fi

# Elevar privilegios con sudo dentro de la misma terminal
if [[ "$EUID" -ne 0 ]]; then
    echo "Solicitando permisos de administrador (sudo)..."
    exec sudo -E bash "$(readlink -f -- "${BASH_SOURCE[0]}")" "$@"
fi

# Detectar la versión mínima de Go desde go.mod.
MIN_GO_VERSION="1.22.0"
if [[ -f "$ROOT_DIR/go.mod" ]]; then
    MOD_GO="$(awk '$1 == "go" { print $2; exit }' "$ROOT_DIR/go.mod")"
    if [[ -n "$MOD_GO" ]]; then
        if [[ "$MOD_GO" =~ ^[0-9]+\.[0-9]+$ ]]; then
            MIN_GO_VERSION="${MOD_GO}.0"
        else
            MIN_GO_VERSION="$MOD_GO"
        fi
    fi
fi

ARCH="$(dpkg --print-architecture)"
case "$ARCH" in
    amd64) GO_ARCH="amd64" ;;
    arm64) GO_ARCH="arm64" ;;
    *)
        echo "Arquitectura Debian no soportada: $ARCH (se admite amd64 o arm64)." >&2
        exit 1
        ;;
esac

echo "[1/4] Verificando paquetes base del sistema..."
MISSING_PACKAGES=()
for pkg in ca-certificates curl python3; do
    if ! dpkg -s "$pkg" >/dev/null 2>&1; then
        MISSING_PACKAGES+=("$pkg")
    fi
done
if (( ${#MISSING_PACKAGES[@]} > 0 )); then
    apt-get update
    apt-get install -y "${MISSING_PACKAGES[@]}"
fi

TMP_DIR="$(mktemp -d)"
cleanup() {
    rm -f -- "$TMP_DIR/go.tar.gz" "$TMP_DIR/tailscale-install.sh"
    rmdir -- "$TMP_DIR"
}
trap cleanup EXIT

echo "[2/4] Verificando Go (mínimo requerido: $MIN_GO_VERSION)..."
GO_BIN="$(command -v go || true)"
GO_VERSION=""
if [[ -n "$GO_BIN" ]]; then
    GO_VERSION="$("$GO_BIN" version | sed -E 's/^go version go([0-9]+\.[0-9]+(\.[0-9]+)?).*/\1/')"
    [[ "$GO_VERSION" =~ ^[0-9]+\.[0-9]+$ ]] && GO_VERSION="${GO_VERSION}.0"
fi

if [[ -z "$GO_VERSION" ]] || dpkg --compare-versions "$GO_VERSION" lt "$MIN_GO_VERSION"; then
    for candidate in /usr/local/lib/go*/bin/go; do
        [[ -x "$candidate" ]] || continue
        candidate_version="$("$candidate" version | sed -E 's/^go version go([0-9]+\.[0-9]+(\.[0-9]+)?).*/\1/')"
        [[ "$candidate_version" =~ ^[0-9]+\.[0-9]+$ ]] && candidate_version="${candidate_version}.0"
        if [[ -n "$candidate_version" ]] &&
            dpkg --compare-versions "$candidate_version" ge "$MIN_GO_VERSION"; then
            GO_BIN="$candidate"
            GO_VERSION="$candidate_version"
            break
        fi
    done
fi

if [[ -z "$GO_VERSION" ]] || dpkg --compare-versions "$GO_VERSION" lt "$MIN_GO_VERSION"; then
    echo "Descargando Go compatible para linux-$GO_ARCH..."
    RELEASES="$(curl -fsSL 'https://go.dev/dl/?mode=json')"
    GO_INFO="$(
        python3 -c '
import json, sys
arch = sys.argv[1]
minimum = tuple(map(int, sys.argv[2].split(".")))
releases = json.load(sys.stdin)
for release in releases:
    if not release.get("stable"):
        continue
    version = release["version"].removeprefix("go")
    if tuple(map(int, version.split("."))) < minimum:
        continue
    for item in release["files"]:
        if item["os"] == "linux" and item["arch"] == arch and item["kind"] == "archive":
            print(item["filename"] + "\t" + item["sha256"] + "\t" + version)
            sys.exit(0)
print("No se encontró una versión estable de Go compatible para esta arquitectura.", file=sys.stderr)
sys.exit(1)
' "$GO_ARCH" "$MIN_GO_VERSION" <<<"$RELEASES"
    )"
    IFS=$'\t' read -r GO_FILENAME GO_SHA256 GO_VERSION <<<"$GO_INFO"
    curl -fsSL "https://go.dev/dl/$GO_FILENAME" -o "$TMP_DIR/go.tar.gz"
    printf '%s  %s\n' "$GO_SHA256" "$TMP_DIR/go.tar.gz" | sha256sum --check
    GO_DIR="/usr/local/lib/go${GO_VERSION}"
    install -d "$GO_DIR"
    tar -C "$GO_DIR" --strip-components=1 -xzf "$TMP_DIR/go.tar.gz"
    GO_BIN="$GO_DIR/bin/go"
fi

GO_VERSION="$("$GO_BIN" version | sed -E 's/^go version go([0-9]+\.[0-9]+(\.[0-9]+)?).*/\1/')"
[[ "$GO_VERSION" =~ ^[0-9]+\.[0-9]+$ ]] && GO_VERSION="${GO_VERSION}.0"
if [[ -z "$GO_VERSION" ]] || dpkg --compare-versions "$GO_VERSION" lt "$MIN_GO_VERSION"; then
    echo "Se requiere Go $MIN_GO_VERSION o posterior; no se pudo instalar una versión compatible." >&2
    exit 1
fi
echo "Usando Go $GO_VERSION: $("$GO_BIN" version)"

echo "[3/4] Compilando el servidor en dist/debian..."
DIST_DIR="$ROOT_DIR/dist/debian"
mkdir -p "$DIST_DIR"

cd "$ROOT_DIR"
"$GO_BIN" mod download
"$GO_BIN" build -o "$DIST_DIR/servidor" ./servidor

if [[ ! -x "$DIST_DIR/servidor" ]]; then
    echo "Error: no se generó el ejecutable $DIST_DIR/servidor." >&2
    exit 1
fi

if [[ -f "$ROOT_DIR/servidor/server.conf" ]]; then
    install -m 0644 "$ROOT_DIR/servidor/server.conf" "$DIST_DIR/server.conf"
fi
if [[ -f "$ROOT_DIR/servidor/users.conf" ]]; then
    install -m 0644 "$ROOT_DIR/servidor/users.conf" "$DIST_DIR/users.conf"
fi

# Devolver propiedad de dist/ al usuario normal si se usó sudo
OWNER_UID="${SUDO_UID:-${PKEXEC_UID:-}}"
OWNER_GID="${SUDO_GID:-}"
if [[ -n "$OWNER_UID" && -n "$OWNER_GID" ]]; then
    chown -R "$OWNER_UID:$OWNER_GID" "$ROOT_DIR/dist"
fi

echo "[4/4] Verificando Tailscale..."
if ! command -v tailscale >/dev/null 2>&1; then
    curl -fsSL https://tailscale.com/install.sh -o "$TMP_DIR/tailscale-install.sh"
    sh "$TMP_DIR/tailscale-install.sh"
fi

echo
echo "✅ ¡Listo! Ejecutable creado en: $DIST_DIR/servidor"
echo "Inicia sesión en Tailscale con 'tailscale up' antes de conectar el cliente."
echo "Para iniciar el servidor:"
echo "  cd '$DIST_DIR' && ./servidor -config server.conf"