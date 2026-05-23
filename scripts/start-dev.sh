#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${2:-.env.dev}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

CMD="${1:-}"

if [ ! -f "$ENV_FILE" ]; then
  echo "ERROR: $ENV_FILE not found."
  exit 1
fi

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

export PATH="/home/stupidur/.local/share/fnm/node-versions/v20.20.2/installation/bin:/home/stupidur/.local/share/fnm/node-versions/v20.20.2/installation/lib/node_modules/corepack:$PATH"
export PATH="/home/stupidur/.local/go/bin:$PATH"

MULTICA_SERVER_BIN="${PROJECT_ROOT}/server/bin/multica-server"
MULTICA_SERVER_LOG="/tmp/multica-dev-backend.log"
MULTICA_FRONTEND_LOG="/tmp/multica-dev-frontend.log"

stop() {
  echo "==> Stopping dev services..."
  fuser -k "${PORT}/tcp" 2>/dev/null || true
  fuser -k "${FRONTEND_PORT}/tcp" 2>/dev/null || true
  echo "Done."
}

build_server() {
  echo "==> Building Go server..."
  mkdir -p "${PROJECT_ROOT}/server/bin"
  cd "${PROJECT_ROOT}/server"
  /home/stupidur/.local/go/bin/go build \
    -ldflags "-X main.version=dev" \
    -o bin/multica-server ./cmd/server
  echo "✓ Server built."
}

start() {
  echo "==> Starting dev services (env: $ENV_FILE)..."
  echo "  Backend: http://localhost:${PORT}"
  echo "  Frontend: http://localhost:${FRONTEND_PORT}"
  echo ""

  fuser -k "${PORT}/tcp" 2>/dev/null || true
  fuser -k "${FRONTEND_PORT}/tcp" 2>/dev/null || true
  sleep 1

  echo "==> Starting backend (port ${PORT})..."
  setsid env \
    PORT="${PORT}" \
    FRONTEND_ORIGIN="${FRONTEND_ORIGIN}" \
    MULTICA_APP_URL="${MULTICA_APP_URL}" \
    LOCAL_UPLOAD_BASE_URL="${LOCAL_UPLOAD_BASE_URL}" \
    DATABASE_URL="${DATABASE_URL}" \
    JWT_SECRET="${JWT_SECRET}" \
    MULTICA_DEV_VERIFICATION_CODE="${MULTICA_DEV_VERIFICATION_CODE}" \
    "${MULTICA_SERVER_BIN}" > "$MULTICA_SERVER_LOG" 2>&1 &
  echo "  Backend PID: $!"

  echo "==> Waiting for backend to be ready..."
  for i in $(seq 1 30); do
    if curl -sf "http://localhost:${PORT}/health" > /dev/null 2>&1; then
      echo "  ✓ Backend ready!"
      break
    fi
    if [ $i -eq 30 ]; then
      echo "ERROR: Backend failed to start. Logs:"
      tail -20 "$MULTICA_SERVER_LOG"
      exit 1
    fi
    sleep 1
  done

  echo "==> Starting frontend (port ${FRONTEND_PORT})..."
  cd "$PROJECT_ROOT"
  setsid env \
    FRONTEND_PORT="${FRONTEND_PORT}" \
    FRONTEND_ORIGIN="${FRONTEND_ORIGIN}" \
    REMOTE_API_URL="${REMOTE_API_URL}" \
    CORS_ALLOWED_ORIGINS="${CORS_ALLOWED_ORIGINS}" \
    pnpm -C apps/web dev > "$MULTICA_FRONTEND_LOG" 2>&1 &
  echo "  Frontend PID: $!"

  echo "==> Waiting for frontend to be ready..."
  for i in $(seq 1 60); do
    if curl -sf "http://localhost:${FRONTEND_PORT}" -o /dev/null 2>/dev/null; then
      echo "  ✓ Frontend ready!"
      break
    fi
    if [ $i -eq 60 ]; then
      echo "WARNING: Frontend may still be compiling. Check logs:"
      tail -10 "$MULTICA_FRONTEND_LOG"
    fi
    sleep 2
  done

  echo ""
  echo "=========================================="
  echo "✓ Dev services running!"
  echo "  Frontend: ${FRONTEND_ORIGIN}"
  echo "  Backend:  http://localhost:${PORT}"
  echo ""
  echo "  Logs:"
  echo "    Backend:  $MULTICA_SERVER_LOG"
  echo "    Frontend: $MULTICA_FRONTEND_LOG"
  echo ""
  echo "  Stop:     bash $0 stop"
  echo "  Restart:  bash $0 restart"
  echo "=========================================="
}

restart() {
  stop
  sleep 2
  start
}

case "$CMD" in
  stop)
    stop
    ;;
  start)
    start
    ;;
  restart)
    restart
    ;;
  build)
    build_server
    ;;
  *)
    echo "Usage: bash $0 {start|stop|restart|build} [.env.dev]"
    echo ""
    echo "  start    - Start backend + frontend dev servers"
    echo "  stop     - Stop processes on PORT and FRONTEND_PORT"
    echo "  restart  - Stop then start"
    echo "  build    - Build Go server binary only"
    echo ""
    echo "  Default env file: .env.dev"
    echo "  Pass env file as second arg: bash $0 start .env.worktree"
    ;;
esac
