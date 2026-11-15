#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

if [[ ! -t 0 && ! -t 1 ]]; then
    SCRIPT_PATH="$(readlink -f -- "${BASH_SOURCE[0]}")"
    if command -v x-terminal-emulator >/dev/null 2>&1; then
        TERMINAL=(x-terminal-emulator -e)
    elif command -v gnome-terminal >/dev/null 2>&1; then
        TERMINAL=(gnome-terminal --wait --)
    else
        MESSAGE="No se encontró una terminal gráfica. Abre una terminal en $ROOT_DIR y ejecuta: bash ./preparar-debian.sh"
        echo "$MESSAGE" >&2
        if command -v zenity >/dev/null 2>&1; then
            zenity --error --title="Preparar servidor Debian" --text="$MESSAGE"
        fi
        exit 1
    fi

    exec "${TERMINAL[@]}" bash -c '
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
    ' bash "$SCRIPT_PATH" "$@"
fi

if [[ "$EUID" -ne 0 ]]; then
    SCRIPT_PATH="$(readlink -f -- "${BASH_SOURCE[0]}")"
    if ! command -v pkexec >/dev/null 2>&1; then
        echo "No se encontró pkexec para solicitar autorización de administrador." >&2
        exit 1
    fi
    exec pkexec "$SCRIPT_PATH" "$@"
fi

MIN_GO_VERSION="1.27.3"
ARCH="$(dpkg --print-architecture)"
PATH="/usr/local/go/bin:/usr/local/lib/go${MIN_GO_VERSION}/bin:$PATH"
export PATH

case "$ARCH" in
    amd64) GO_ARCH="amd64" ;;
    arm64) GO_ARCH="arm64" ;;
    *)
        echo "Arquitectura Debian no soportada: $ARCH (se admite amd64 o arm64)." >&2
        exit 1
        ;;
esac

DIST_DIR="$ROOT_DIR/dist/debian"
mkdir -p "$DIST_DIR"

OWNER_UID="${PKEXEC_UID:-${SUDO_UID:-}}"
OWNER_ENTRY=""
if [[ "$OWNER_UID" =~ ^[0-9]+$ ]]; then
    OWNER_ENTRY="$(getent passwd "$OWNER_UID" || true)"
    if [[ -n "$OWNER_ENTRY" ]]; then
        IFS=: read -r _ _ _ OWNER_GID _ <<<"$OWNER_ENTRY"
        chown "$OWNER_UID:$OWNER_GID" "$ROOT_DIR/dist" "$DIST_DIR"
    fi
fi

MISSING_PACKAGES=()
for dependency in "ca-certificates:ca-certificates" "curl:curl" "python3:python3"; do
    command_name="${dependency%%:*}"
    package_name="${dependency#*:}"
    if ! command -v "$command_name" >/dev/null 2>&1; then
        MISSING_PACKAGES+=("$package_name")
    fi
done
if [[ ! -r /etc/ssl/certs/ca-certificates.crt ]]; then
    MISSING_PACKAGES+=(ca-certificates)
fi
if (( ${#MISSING_PACKAGES[@]} > 0 )); then
    apt-get update
    apt-get install -y "${MISSING_PACKAGES[@]}"
fi

TMP_DIR="$(mktemp -d)"
GO_TARBALL="$TMP_DIR/go.tar.gz"
cleanup() {
    rm -f -- "$GO_TARBALL" "$TMP_DIR/tailscale-install.sh"
    rmdir -- "$TMP_DIR"
}
trap cleanup EXIT

GO_BIN="$(command -v go || true)"
if [[ -z "$GO_BIN" && -x /usr/local/go/bin/go ]]; then
    GO_BIN="/usr/local/go/bin/go"
fi
if [[ -z "$GO_BIN" && -x "/usr/local/lib/go${MIN_GO_VERSION}/bin/go" ]]; then
    GO_BIN="/usr/local/lib/go${MIN_GO_VERSION}/bin/go"
fi
GO_VERSION=""
if [[ -n "$GO_BIN" ]]; then
    GO_VERSION="$("$GO_BIN" version | sed -E 's/^go version go([0-9]+\.[0-9]+\.[0-9]+).*/\1/')"
fi

if [[ -z "$GO_VERSION" ]] || dpkg --compare-versions "$GO_VERSION" lt "$MIN_GO_VERSION"; then
    RELEASES="$(curl -fsSL 'https://go.dev/dl/?mode=json')"
    GO_INFO="$(
        python3 -c '
import json
import sys

arch = sys.argv[1]
minimum = (1, 27, 3)
releases = json.load(sys.stdin)
for release in releases:
    if not release.get("stable"):
        continue
    version = release["version"].removeprefix("go")
    if tuple(map(int, version.split("."))) < minimum:
        continue
    for item in release["files"]:
        if item["os"] == "linux" and item["arch"] == arch and item["kind"] == "archive":
            print(item["filename"] + "\t" + item["sha256"])
            sys.exit(0)
print("No se encontró una versión estable de Go compatible para esta arquitectura.", file=sys.stderr)
sys.exit(1)
' "$GO_ARCH" <<<"$RELEASES"
    )"
    IFS=$'\t' read -r GO_FILENAME GO_SHA256 <<<"$GO_INFO"
    GO_VERSION="${GO_FILENAME#go}"
    GO_VERSION="${GO_VERSION%.linux-${GO_ARCH}.tar.gz}"

    curl -fsSL "https://go.dev/dl/$GO_FILENAME" -o "$GO_TARBALL"
    printf '%s  %s\n' "$GO_SHA256" "$GO_TARBALL" | sha256sum --check

    GO_DIR="/usr/local/lib/go${GO_VERSION}"
    install -d "$GO_DIR"
    tar -C "$GO_DIR" --strip-components=1 -xzf "$GO_TARBALL"
    GO_BIN="$GO_DIR/bin/go"
fi

GO_VERSION="$("$GO_BIN" version | sed -E 's/^go version go([0-9]+\.[0-9]+\.[0-9]+).*/\1/')"
if dpkg --compare-versions "$GO_VERSION" lt "$MIN_GO_VERSION"; then
    echo "Se requiere Go $MIN_GO_VERSION o posterior; se encontró Go $GO_VERSION." >&2
    exit 1
fi

cd "$ROOT_DIR"
"$GO_BIN" mod download
"$GO_BIN" build -o "$DIST_DIR/servidor" ./servidor
if [[ ! -x "$DIST_DIR/servidor" ]]; then
    echo "La compilación terminó, pero no se creó un ejecutable en $DIST_DIR/servidor." >&2
    exit 1
fi
install -m 0644 "$ROOT_DIR/servidor/server.conf" "$DIST_DIR/server.conf"
install -m 0644 "$ROOT_DIR/servidor/users.conf" "$DIST_DIR/users.conf"

if [[ -n "$OWNER_UID" && "$OWNER_UID" =~ ^[0-9]+$ ]]; then
    if [[ -n "$OWNER_ENTRY" ]]; then
        chown -R "$OWNER_UID:$OWNER_GID" "$DIST_DIR"
    fi
fi

if ! command -v tailscale >/dev/null 2>&1; then
    TAILSCALE_INSTALLER="$TMP_DIR/tailscale-install.sh"
    curl -fsSL https://tailscale.com/install.sh -o "$TAILSCALE_INSTALLER"
    sh "$TAILSCALE_INSTALLER"
    rm -f -- "$TAILSCALE_INSTALLER"
fi

echo
echo "Listo. Ejecutable y configuración creados en dist/debian."
echo "Edita dist/debian/server.conf y cambia AllowedClientIP por la IP VPN del cliente."
echo "Inicia sesión en Tailscale con 'sudo tailscale up' y luego ejecuta:"
echo "  cd '$DIST_DIR' && ./servidor -config server.conf"
