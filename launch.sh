#!/usr/bin/env bash

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$PROJECT_ROOT/backend"
FRONTEND_DIR="$PROJECT_ROOT/frontend"
RUN_DIR="$PROJECT_ROOT/.run"
RUN_BIN="$RUN_DIR/fakemex"
CONFIG_PATH="${FAKEMEX_CONFIG:-$PROJECT_ROOT/config/local.env}"
BACKEND_HOST="127.0.0.1"
BACKEND_PORT=8080
FRONTEND_HOST="127.0.0.1"
FRONTEND_PORT_DEFAULT=4200
REQUIRED_KEYS=(
  HL_NETWORK
  HL_API_URL
  HL_WS_URL
  SERVER_ADDR
  FRONTEND_ORIGIN
)

BACKEND_LOG="$RUN_DIR/backend.log"
FRONTEND_LOG="$RUN_DIR/frontend.log"
BACKEND_PID=""
FRONTEND_PID=""
CLEANUP_DONE=0
READY_TIMEOUT=${FAKEMEX_READY_TIMEOUT:-60}

fail() {
  echo "launch.sh: $*" >&2
  exit 1
}

require_cmd() {
  local cmd=$1
  if ! command -v "$cmd" >/dev/null 2>&1; then
    fail "required command not found: $cmd"
  fi
}

trim() {
  local value=$1
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

read_config_value() {
  local file=$1
  local key=$2
  local line
  while IFS= read -r line || [[ -n "${line-}" ]]; do
    line="${line%%#*}"
    line="${line//$'\r'/}"
    if [[ "$line" =~ ^[[:space:]]*$key[[:space:]]*=(.*)$ ]]; then
      line="$(trim "${BASH_REMATCH[1]}")"
      line="${line#\"}"
      line="${line%\"}"
      line="${line#\'}"
      line="${line%\'}"
      printf '%s' "$line"
      return 0
    fi
  done < "$file"
  return 1
}

is_port_open() {
  local host=$1
  local port=$2
  (echo >/dev/tcp/"$host"/"$port") >/dev/null 2>&1
}

is_running() {
  local pid=$1
  kill -0 "$pid" >/dev/null 2>&1
}

wait_for_http() {
  local url=$1
  local label=$2
  local timeout=$READY_TIMEOUT
  local deadline=$((SECONDS + timeout))

  while ((SECONDS < deadline)); do
    if curl --silent --fail --max-time 1 "$url" >/dev/null; then
      echo "✓ ${label} ready: ${url}"
      return 0
    fi
    sleep 1
  done
  fail "timed out waiting for ${label}: ${url}"
}

terminate_pid() {
  local pid=$1
  local label=$2
  local timeout="${3:-5}"
  local end
  local waited

  if [[ -z "$pid" ]] || ! is_running "$pid"; then
    return 0
  fi

  kill -TERM "$pid" 2>/dev/null || true
  end=$((SECONDS + timeout))
  while ((SECONDS < end)); do
    if ! is_running "$pid"; then
      wait "$pid" 2>/dev/null || true
      return 0
    fi
    sleep 1
  done

  kill -KILL "$pid" 2>/dev/null || true
  waited=0
  while ((waited < 3)); do
    if ! is_running "$pid"; then
      wait "$pid" 2>/dev/null || true
      return 0
    fi
    sleep 1
    waited=$((waited + 1))
  done

  fail "could not stop ${label} process ($pid)"
}

cleanup() {
  if (( CLEANUP_DONE )); then
    return
  fi
  CLEANUP_DONE=1
  set +e
  terminate_pid "$BACKEND_PID" "backend"
  terminate_pid "$FRONTEND_PID" "frontend"
  set -e
}

show_status() {
  echo "Backend : ${BACKEND_ADDR}"
  echo "Frontend: ${FRONTEND_URL}"
  echo "Backend logs: ${BACKEND_LOG}"
  echo "Frontend logs: ${FRONTEND_LOG}"
  echo
  echo "Press Ctrl+C to stop both services."
}

resolve_frontend_port_from_origin() {
  local origin=$1
  local host_port=${origin#*://}
  host_port=${host_port%%/*}
  if [[ "$host_port" == *:* ]]; then
    echo "${host_port##*:}"
    return 0
  fi
  echo "$FRONTEND_PORT_DEFAULT"
}

if ! [[ "$BACKEND_PORT" =~ ^[0-9]+$ ]] || ((BACKEND_PORT < 1 || BACKEND_PORT > 65535)); then
  fail "invalid backend port: $BACKEND_PORT"
fi

trap 'cleanup' EXIT
trap 'cleanup; exit 130' INT TERM

require_cmd go
require_cmd npm
require_cmd curl

if [[ ! -f "$CONFIG_PATH" ]]; then
  fail "missing config file: $CONFIG_PATH"
fi
if [[ ! -r "$CONFIG_PATH" ]]; then
  fail "unreadable config file: $CONFIG_PATH"
fi

for key in "${REQUIRED_KEYS[@]}"; do
  if [[ -z "$(read_config_value "$CONFIG_PATH" "$key")" ]]; then
    fail "config missing required key: $key"
  fi
done

trading_enabled="$(read_config_value "$CONFIG_PATH" "TRADING_ENABLED" || printf 'false')"
trading_enabled="${trading_enabled,,}"
if [[ "$trading_enabled" != "true" && "$trading_enabled" != "false" ]]; then
  fail "TRADING_ENABLED must be true or false"
fi
if [[ "$trading_enabled" == "true" ]]; then
  for key in HL_ACCOUNT_ADDRESS HL_API_WALLET_ADDRESS HL_API_WALLET_PRIVATE_KEY; do
    if [[ -z "$(read_config_value "$CONFIG_PATH" "$key")" ]]; then
      fail "trading config missing required key: $key"
    fi
  done
fi

server_addr="$(read_config_value "$CONFIG_PATH" "SERVER_ADDR")"
frontend_origin="$(read_config_value "$CONFIG_PATH" "FRONTEND_ORIGIN")"

if [[ -n "${server_addr-}" ]]; then
  backend_host="${server_addr%%:*}"
  backend_port="${server_addr##*:}"
else
  fail "config missing SERVER_ADDR"
fi

if [[ -z "${backend_host-}" || -z "${backend_port-}" || ! "${backend_port}" =~ ^[0-9]+$ ]]; then
  fail "invalid SERVER_ADDR in config: ${server_addr}"
fi

if [[ "${backend_host}" != "127.0.0.1" && "${backend_host}" != "localhost" ]]; then
  backend_host="127.0.0.1"
fi

BACKEND_ADDR="${BACKEND_HOST}:${BACKEND_PORT}"
if [[ "${backend_port}" != "${BACKEND_PORT}" ]]; then
  fail "backend must use loopback port ${BACKEND_PORT} to match frontend proxy; got ${backend_port}"
fi

FRONTEND_PORT="$(resolve_frontend_port_from_origin "$frontend_origin")"
if [[ ! "$FRONTEND_PORT" =~ ^[0-9]+$ ]] || ((FRONTEND_PORT < 1 || FRONTEND_PORT > 65535 )); then
  fail "invalid frontend port: ${FRONTEND_PORT}"
fi

if is_port_open "$BACKEND_HOST" "$BACKEND_PORT"; then
  fail "backend port ${BACKEND_PORT} is already in use. Free it before launching (or run with no override support for backend host/port)."
fi

if is_port_open "$FRONTEND_HOST" "$FRONTEND_PORT"; then
  fail "frontend port ${FRONTEND_PORT} is already in use. Free it before launching."
fi

mkdir -p "$RUN_DIR"

if [[ ! -x "$FRONTEND_DIR/node_modules/.bin/ng" ]]; then
  (cd "$FRONTEND_DIR" && npm ci)
fi

(cd "$BACKEND_DIR" && go build -o "$RUN_BIN" ./cmd/fakemex)
if [[ ! -x "$RUN_BIN" ]]; then
  fail "backend build failed: missing binary $RUN_BIN"
fi

(
  cd "$BACKEND_DIR"
  exec ../.run/fakemex -config "$CONFIG_PATH"
) >"$BACKEND_LOG" 2>&1 &
BACKEND_PID=$!

(
  cd "$FRONTEND_DIR"
  exec ./node_modules/.bin/ng serve --host "$FRONTEND_HOST" --port "$FRONTEND_PORT" --proxy-config proxy.conf.json
) >"$FRONTEND_LOG" 2>&1 &
FRONTEND_PID=$!

FRONTEND_URL="http://${FRONTEND_HOST}:${FRONTEND_PORT}/"
wait_for_http "http://${BACKEND_ADDR}/api/v1/health" "backend health"
wait_for_http "${FRONTEND_URL}" "frontend root"

if ! is_running "$BACKEND_PID"; then
  fail "backend process failed to start (check ${BACKEND_LOG})"
fi
if ! is_running "$FRONTEND_PID"; then
  fail "frontend process failed to start (check ${FRONTEND_LOG})"
fi

show_status
echo "Startup completed."

while true; do
  if ! is_running "$BACKEND_PID"; then
    code=0
    wait "$BACKEND_PID" || code=$?
    cleanup
    fail "backend exited unexpectedly with status ${code} (see ${BACKEND_LOG})"
  fi

  if ! is_running "$FRONTEND_PID"; then
    code=0
    wait "$FRONTEND_PID" || code=$?
    cleanup
    fail "frontend exited unexpectedly with status ${code} (see ${FRONTEND_LOG})"
  fi

  sleep 1
done
