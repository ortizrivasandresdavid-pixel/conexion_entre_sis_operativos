#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
MIN_GO_VERSION="1.27.3"
ARCH="$(dpkg --print-architecture)"

case "$ARCH" in
    amd64) GO_ARCH="amd64" ;;
    arm64) GO_ARCH="arm64" ;;
    *)
        echo "Arquitectura Debian no soportada: $ARCH (se admite amd64 o arm64)." >&2
        exit 1
        ;;
esac

if [[ "$EUID" -eq 0 ]]; then
    SUDO=()
else
    command -v sudo >/dev/null 2>&1 || {
        echo "Instala sudo o ejecuta este script como root." >&2
        exit 1
    }
    SUDO=(sudo)
fi

"${SUDO[@]}" apt-get update
"${SUDO[@]}" apt-get install -y ca-certificates curl python3 xz-utils

TMP_DIR="$(mktemp -d)"
GO_TARBALL="$TMP_DIR/go.tar.gz"
cleanup() {
    rm -f -- "$GO_TARBALL" "$TMP_DIR/tailscale-install.sh"
    rmdir -- "$TMP_DIR"
}
trap cleanup EXIT

GO_BIN="$(command -v go || true)"
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
    "${SUDO[@]}" install -d "$GO_DIR"
    "${SUDO[@]}" tar -C "$GO_DIR" --strip-components=1 -xzf "$GO_TARBALL"
    GO_BIN="$GO_DIR/bin/go"
fi

GO_VERSION="$("$GO_BIN" version | sed -E 's/^go version go([0-9]+\.[0-9]+\.[0-9]+).*/\1/')"
if dpkg --compare-versions "$GO_VERSION" lt "$MIN_GO_VERSION"; then
    echo "Se requiere Go $MIN_GO_VERSION o posterior; se encontró Go $GO_VERSION." >&2
    exit 1
fi

if ! command -v tailscale >/dev/null 2>&1; then
    TAILSCALE_INSTALLER="$TMP_DIR/tailscale-install.sh"
    curl -fsSL https://tailscale.com/install.sh -o "$TAILSCALE_INSTALLER"
    "${SUDO[@]}" sh "$TAILSCALE_INSTALLER"
    rm -f -- "$TAILSCALE_INSTALLER"
fi

DIST_DIR="$ROOT_DIR/dist/debian"
mkdir -p "$DIST_DIR"
cd "$ROOT_DIR"
"$GO_BIN" mod download
"$GO_BIN" build -o "$DIST_DIR/servidor" ./servidor
install -m 0644 "$ROOT_DIR/servidor/server.conf" "$DIST_DIR/server.conf"
install -m 0644 "$ROOT_DIR/servidor/users.conf" "$DIST_DIR/users.conf"

echo
echo "Listo. Ejecutable y configuración creados en dist/debian."
echo "Edita dist/debian/server.conf y cambia AllowedClientIP por la IP VPN del cliente."
echo "Inicia sesión en Tailscale con 'sudo tailscale up' y luego ejecuta:"
echo "  cd '$DIST_DIR' && ./servidor -config server.conf"
