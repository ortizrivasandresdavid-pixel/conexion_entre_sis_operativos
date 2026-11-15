#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

if [[ ! -t 0 || ! -t 1 ]]; then
    SCRIPT_PATH="$(readlink -f -- "${BASH_SOURCE[0]}")"
    TERMINAL_COMMAND='
        script=$1
        shift
        "$script" "$@"
        status=$?
        echo
        read -r -p "Servidor detenido. Presiona Enter para cerrar esta ventana..."
        exit "$status"
    '
    if command -v x-terminal-emulator >/dev/null 2>&1; then
        exec x-terminal-emulator -e bash -c "$TERMINAL_COMMAND" bash "$SCRIPT_PATH" "$@"
    elif command -v gnome-terminal >/dev/null 2>&1; then
        exec gnome-terminal --wait -- bash -c "$TERMINAL_COMMAND" bash "$SCRIPT_PATH" "$@"
    else
        echo "Abre una terminal en $ROOT_DIR y ejecuta: bash ./iniciar-servidor-debian.sh" >&2
        exit 1
    fi
fi

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

cd "$ROOT_DIR"
exec "$GO_BIN" run ./servidor -config "$ROOT_DIR/servidor/server.conf" "$@"
