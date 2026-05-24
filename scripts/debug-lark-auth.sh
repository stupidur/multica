#!/usr/bin/env bash
set -euo pipefail

ENV_FILE="${1:-.env.dev}"
MODE="${2:-summary}"

if [ ! -f "$ENV_FILE" ]; then
  echo "ERROR: $ENV_FILE not found."
  exit 1
fi

set -a
# shellcheck disable=SC1090
. "$ENV_FILE"
set +a

FRONTEND_URL="${LARK_REDIRECT_URI%/auth/lark}"
BACKEND_URL="http://127.0.0.1:${PORT:-8091}"
FRONTEND_LOCAL="http://127.0.0.1:${FRONTEND_PORT:-3001}"
FRONTEND_LOG="/tmp/multica-dev-frontend.log"
BACKEND_LOG="/tmp/multica-dev-backend.log"

print_summary() {
  echo "== Lark auth config =="
  echo "Public auth URL: ${FRONTEND_URL}/auth/lark"
  echo "Redirect URI:    ${LARK_REDIRECT_URI:-}"
  echo "App ID:          ${LARK_APP_ID:-}"
  echo "Devtools JS:     ${FEISHU_DEVTOOLS_SCRIPT_URL:-<not set>}"
  echo ""

  echo "== Local health =="
  curl -I -s --max-time 5 "${FRONTEND_LOCAL}/auth/lark" | sed -n '1,8p' || true
  echo ""
  curl -s --max-time 5 "${BACKEND_URL}/health" || true
  echo ""
  echo ""

  echo "== Recent frontend Lark/onboarding logs =="
  if [ -f "$FRONTEND_LOG" ]; then
    grep -E "lark-auth|/auth/lark|/onboarding|\[browser\].*(api|auth)|error|Error" "$FRONTEND_LOG" | tail -80 || true
  else
    echo "missing $FRONTEND_LOG"
  fi
  echo ""

  echo "== Recent backend Lark/auth logs =="
  if [ -f "$BACKEND_LOG" ]; then
    grep -E "lark|/auth/lark|/api/me|/api/workspaces|user logged in via lark|status=50|status=401" "$BACKEND_LOG" | tail -80 || true
  else
    echo "missing $BACKEND_LOG"
  fi
}

follow_logs() {
  echo "Following Lark auth logs. Press Ctrl-C to stop."
  tail -n 0 -F "$FRONTEND_LOG" "$BACKEND_LOG" | grep --line-buffered -E "lark-auth|/auth/lark|/onboarding|/api/me|/api/workspaces|user logged in via lark|status=50|status=401|\[browser\].*(api|auth)|error|Error"
}

case "$MODE" in
  summary)
    print_summary
    ;;
  follow)
    follow_logs
    ;;
  *)
    echo "Usage: bash scripts/debug-lark-auth.sh [.env.dev] [summary|follow]"
    exit 1
    ;;
esac
