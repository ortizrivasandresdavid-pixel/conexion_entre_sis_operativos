#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_PATH="$(readlink -f -- "${BASH_SOURCE[0]}")"

if [[ ! -t 0 || ! -t 1 ]]; then
    if command -v x-terminal-emulator >/dev/null 2>&1; then
        exec x-terminal-emulator -e bash -c 'bash "$@"' _ "$SCRIPT_PATH" "$@"
    elif command -v gnome-terminal >/dev/null 2>&1; then
        exec gnome-terminal --wait -- bash -c 'bash "$@"' _ "$SCRIPT_PATH" "$@"
    else
        echo "Abre una terminal en $ROOT_DIR y ejecuta: bash ./iniciar-cli-servidor.sh" >&2
        exit 1
    fi
fi

cd "$ROOT_DIR"
if go run ./src/servercli "$@"; then
    status=0
else
    status=$?
fi

echo
read -r -p "El monitor terminó (código $status). Presiona Enter para cerrar esta ventana..."
exit "$status"
