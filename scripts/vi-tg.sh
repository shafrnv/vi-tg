#!/bin/sh
# Start Go backend, then Rust TUI. Backend is stopped on exit.
set -eu

SERVER_BIN="${VI_TG_SERVER:-/usr/bin/vi-tg-server}"
TUI_BIN="${VI_TG_TUI:-/usr/bin/vi-tg-tui}"
HOST="${VI_TG_HOST:-127.0.0.1}"
PORT="${VI_TG_PORT:-8080}"

if ! command -v "$SERVER_BIN" >/dev/null 2>&1 && [ ! -x "$SERVER_BIN" ]; then
  echo "vi-tg: backend not found: $SERVER_BIN" >&2
  exit 1
fi
if ! command -v "$TUI_BIN" >/dev/null 2>&1 && [ ! -x "$TUI_BIN" ]; then
  echo "vi-tg: TUI not found: $TUI_BIN" >&2
  exit 1
fi

"$SERVER_BIN" &
server_pid=$!

cleanup() {
  kill "$server_pid" 2>/dev/null || true
  wait "$server_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# Wait until HTTP API is up (max ~10s)
i=0
while [ "$i" -lt 50 ]; do
  if command -v curl >/dev/null 2>&1; then
    if curl -fsS "http://${HOST}:${PORT}/health" >/dev/null 2>&1; then
      break
    fi
  else
    # Fallback: give the server a moment
    if [ "$i" -ge 5 ]; then
      break
    fi
  fi
  i=$((i + 1))
  sleep 0.2
done

exec "$TUI_BIN" "$@"
