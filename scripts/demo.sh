#!/usr/bin/env sh
set -eu

ROOT_DIR=$(cd -- "$(dirname -- "$0")/.." && pwd)
RUNTIME_DIR=${COGNIGO_DEMO_RUNTIME_DIR:-"$ROOT_DIR/.local/demo"}
ENV_FILE=${DEMO_ENV_FILE:-"$ROOT_DIR/.env"}
WAIT_SECONDS=${COGNIGO_DEMO_WAIT_SECONDS:-60}
PROJECT_SUFFIX=$(printf '%s' "$ROOT_DIR" | cksum | awk '{print $1}')
COMPOSE_PROJECT_NAME="cognigo-demo-$PROJECT_SUFFIX"

API_PID_FILE="$RUNTIME_DIR/api.pid"
MCP_PID_FILE="$RUNTIME_DIR/mcp.pid"
FRONTEND_PID_FILE="$RUNTIME_DIR/frontend.pid"
API_LOG="$RUNTIME_DIR/api.log"
MCP_LOG="$RUNTIME_DIR/mcp.log"
FRONTEND_LOG="$RUNTIME_DIR/frontend.log"

API_PORT=${COGNIGO_PORT:-9090}
MCP_PORT=${COGNIGO_MCP_PORT:-8081}
FRONTEND_PORT=${COGNIGO_FRONTEND_PORT:-8080}
MAILPIT_WEB_PORT=8025

usage() {
  cat <<'EOF'
Usage: scripts/demo.sh <command>

Commands:
  start   Start Compose dependencies, API, MCP, and the frontend
  stop    Stop services started by this script and this repository's Compose project
  status  Show service, process, and endpoint status
  logs    Show logs; use "logs -f [api|mcp|frontend]" to follow one service
EOF
}

say() {
  printf '[demo] %s\n' "$*"
}

warn() {
  printf '[demo] warning: %s\n' "$*" >&2
}

die() {
  printf '[demo] error: %s\n' "$*" >&2
  exit 1
}

load_env() {
  [ -f "$ENV_FILE" ] || die "missing $ENV_FILE; run: cp .env.example .env"
  case "$ENV_FILE" in
    "$ROOT_DIR"/*) ;;
    *) [ -r "$ENV_FILE" ] || die "environment file is not readable: $ENV_FILE" ;;
  esac
  set -a
  # shellcheck disable=SC1090
  . "$ENV_FILE"
  set +a
  API_PORT=${COGNIGO_PORT:-9090}
  MCP_PORT=${COGNIGO_MCP_PORT:-8081}
  FRONTEND_PORT=${COGNIGO_FRONTEND_PORT:-8080}
}

load_env_optional() {
  if [ -f "$ENV_FILE" ]; then
    load_env
  else
    warn "missing $ENV_FILE; using built-in demo defaults"
  fi
}

compose() {
  COMPOSE_PROJECT_NAME="$COMPOSE_PROJECT_NAME" docker compose "$@"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

check_prerequisites() {
  require_command docker
  require_command go
  require_command node
  require_command npm
  require_command curl
  docker info >/dev/null 2>&1 || die "Docker daemon is unavailable"
  docker compose version >/dev/null 2>&1 || die "Docker Compose is unavailable"
  [ -f "$ROOT_DIR/config/config.toml" ] || die "missing config/config.toml"
}

port_in_use() {
  port=$1
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null | head -n 1 | grep -q .
    return $?
  fi
  if command -v ss >/dev/null 2>&1; then
    ss -ltnH 2>/dev/null | awk -v port=":$port" 'index($4, port) == length($4) - length(port) + 1 {found=1} END {exit found ? 0 : 1}'
    return $?
  fi
  return 1
}

pid_value() {
  file=$1
  [ -s "$file" ] || return 1
  pid=$(sed -n '1p' "$file")
  case "$pid" in
    ''|*[!0-9]*) return 1 ;;
  esac
  printf '%s\n' "$pid"
}

process_command() {
  pid=$1
  ps -p "$pid" -o command= 2>/dev/null || true
}

process_cwd() {
  pid=$1
  if [ -e "/proc/$pid/cwd" ]; then
    readlink "/proc/$pid/cwd" 2>/dev/null || true
  elif command -v lsof >/dev/null 2>&1; then
    lsof -a -p "$pid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1
  fi
}

process_owned() {
  pid=$1
  expected=$2
  kill -0 "$pid" 2>/dev/null || return 1
  command=$(process_command "$pid")
  cwd=$(process_cwd "$pid")
  case "$command" in
    *"$expected"*) ;;
    *) return 1 ;;
  esac
  case "$cwd" in
    "$ROOT_DIR"|"$ROOT_DIR"/*) return 0 ;;
    *) return 1 ;;
  esac
}

recorded_running() {
  file=$1
  expected=$2
  pid=$(pid_value "$file" 2>/dev/null || true)
  [ -n "$pid" ] && process_owned "$pid" "$expected"
}

clear_stale_pid() {
  file=$1
  expected=$2
  pid=$(pid_value "$file" 2>/dev/null || true)
  [ -z "$pid" ] && return 0
  recorded_running "$file" "$expected" && return 0
  rm -f "$file"
}

ensure_app_ports_free() {
  for entry in "api:$API_PORT" "mcp:$MCP_PORT" "frontend:$FRONTEND_PORT"; do
    service=${entry%%:*}
    port=${entry#*:}
    if port_in_use "$port"; then
      case "$service" in
        api) expected="bin/cognigo"; file=$API_PID_FILE ;;
        mcp) expected="common/mcp/bin/cognigo-mcp"; file=$MCP_PID_FILE ;;
        frontend) expected="npm run serve"; file=$FRONTEND_PID_FILE ;;
      esac
      recorded_running "$file" "$expected" || die "port $port for $service is already in use; set its COGNIGO_*_PORT/COGNIGO_FRONTEND_PORT or stop the owning process"
    fi
  done
}

build_if_needed() {
  if [ ! -x "$ROOT_DIR/bin/cognigo" ]; then
    say "building API binary"
    (cd "$ROOT_DIR" && make build)
  fi
  if [ ! -x "$ROOT_DIR/common/mcp/bin/cognigo-mcp" ]; then
    say "building MCP binary"
    (cd "$ROOT_DIR" && make build-mcp)
  fi
  if [ ! -x "$ROOT_DIR/vue-frontend/node_modules/.bin/vue-cli-service" ]; then
    say "installing frontend dependencies"
    (cd "$ROOT_DIR" && make frontend-install)
  fi
}

compose_service_ready() {
  service=$1
  container=$(compose ps -q "$service" 2>/dev/null | head -n 1)
  [ -n "$container" ] || return 1
  state=$(docker inspect --format '{{.State.Status}}' "$container" 2>/dev/null || true)
  [ "$state" = running ] || return 1
  health=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container" 2>/dev/null || true)
  [ "$health" = healthy ] || [ "$health" = none ]
}

wait_for_compose() {
  deadline=$(( $(date +%s) + WAIT_SECONDS ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    ready=1
    for service in mysql redis rabbitmq mailpit; do
      compose_service_ready "$service" || ready=0
    done
    [ "$ready" -eq 1 ] && return 0
    sleep 2
  done
  compose ps >&2 || true
  die "Compose services did not become ready within ${WAIT_SECONDS}s"
}

http_code() {
  url=$1
  code=$(curl -sS -o /dev/null -w '%{http_code}' --max-time 3 "$url" 2>/dev/null || true)
  [ -n "$code" ] || code=000
  printf '%s' "$code"
}

endpoint_ready() {
  url=$1
  expected=$2
  code=$(http_code "$url")
  case ",$expected," in
    *,"$code",*) return 0 ;;
    *) return 1 ;;
  esac
}

wait_for_apps() {
  deadline=$(( $(date +%s) + WAIT_SECONDS ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    endpoint_ready "http://127.0.0.1:$API_PORT/" 200 && \
      endpoint_ready "http://127.0.0.1:$FRONTEND_PORT/" 200 && \
      endpoint_ready "http://127.0.0.1:$MAILPIT_WEB_PORT/" 200 && \
      (endpoint_ready "http://127.0.0.1:$MCP_PORT/mcp" 200,400) && return 0
    sleep 2
  done
  die "application services did not become ready; inspect $RUNTIME_DIR/*.log"
}

start_process() {
  name=$1
  pid_file=$2
  log_file=$3
  expected=$4
  shift 4

  clear_stale_pid "$pid_file" "$expected"
  if recorded_running "$pid_file" "$expected"; then
    say "$name already running (pid $(pid_value "$pid_file"))"
    return 0
  fi

  say "starting $name"
  if command -v setsid >/dev/null 2>&1; then
    setsid env COGNIGO_MCP_URL="http://127.0.0.1:$MCP_PORT/mcp" "$@" >"$log_file" 2>&1 &
  else
    env COGNIGO_MCP_URL="http://127.0.0.1:$MCP_PORT/mcp" "$@" >"$log_file" 2>&1 &
  fi
  pid=$!
  printf '%s\n' "$pid" >"$pid_file"
  case "$name" in
    api) STARTED_API=1 ;;
    mcp) STARTED_MCP=1 ;;
  esac
  sleep 1
  process_owned "$pid" "$expected" || die "$name exited during startup; inspect $log_file"
}

start_frontend() {
  clear_stale_pid "$FRONTEND_PID_FILE" "npm run serve"
  if recorded_running "$FRONTEND_PID_FILE" "npm run serve"; then
    say "frontend already running (pid $(pid_value "$FRONTEND_PID_FILE"))"
    return 0
  fi
  say "starting frontend"
  if command -v setsid >/dev/null 2>&1; then
    # shellcheck disable=SC2016
    setsid sh -c 'export COGNIGO_API_URL="$3"; cd "$1" && exec npm run serve -- --port "$2"' sh "$ROOT_DIR/vue-frontend" "$FRONTEND_PORT" "http://127.0.0.1:$API_PORT" >"$FRONTEND_LOG" 2>&1 &
  else
    (export COGNIGO_API_URL="http://127.0.0.1:$API_PORT"; cd "$ROOT_DIR/vue-frontend" && npm run serve -- --port "$FRONTEND_PORT") >"$FRONTEND_LOG" 2>&1 &
  fi
  pid=$!
  printf '%s\n' "$pid" >"$FRONTEND_PID_FILE"
  STARTED_FRONTEND=1
  sleep 1
  process_owned "$pid" "npm run serve" || die "frontend exited during startup; inspect $FRONTEND_LOG"
}

start() {
  load_env
  check_prerequisites
  mkdir -p "$RUNTIME_DIR"
  START_IN_PROGRESS=1
  trap cleanup_started EXIT
  trap 'cleanup_started; exit 130' INT TERM HUP
  build_if_needed
  ensure_app_ports_free
  say "starting Compose dependencies"
  compose up -d mysql redis rabbitmq mailpit
  wait_for_compose
  start_process api "$API_PID_FILE" "$API_LOG" "bin/cognigo" "$ROOT_DIR/bin/cognigo"
  start_process mcp "$MCP_PID_FILE" "$MCP_LOG" "common/mcp/bin/cognigo-mcp" "$ROOT_DIR/common/mcp/bin/cognigo-mcp" -mode server -http-addr ":$MCP_PORT"
  start_frontend
  wait_for_apps
  START_IN_PROGRESS=0
  trap - EXIT INT TERM HUP
  status
}

stop_process() {
  name=$1
  file=$2
  expected=$3
  pid=$(pid_value "$file" 2>/dev/null || true)
  if [ -z "$pid" ]; then
    say "$name is not recorded"
    return 0
  fi
  if ! process_owned "$pid" "$expected"; then
    warn "not stopping pid $pid for $name because ownership could not be verified"
    rm -f "$file"
    return 0
  fi
  say "stopping $name (pid $pid)"
  if command -v setsid >/dev/null 2>&1; then
    kill -TERM -"$pid" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
  else
    kill -TERM "$pid" 2>/dev/null || true
  fi
  deadline=$(( $(date +%s) + 10 ))
  while kill -0 "$pid" 2>/dev/null && [ "$(date +%s)" -lt "$deadline" ]; do sleep 1; done
  if kill -0 "$pid" 2>/dev/null; then
    warn "$name did not stop after 10s; leaving it running"
  else
    rm -f "$file"
  fi
}

cleanup_started() {
  [ "${START_IN_PROGRESS:-0}" -eq 1 ] || return 0
  START_IN_PROGRESS=0
  warn "startup did not complete; stopping processes started by this invocation"
  [ "${STARTED_FRONTEND:-0}" -eq 0 ] || stop_process frontend "$FRONTEND_PID_FILE" "npm run serve" || true
  [ "${STARTED_MCP:-0}" -eq 0 ] || stop_process mcp "$MCP_PID_FILE" "common/mcp/bin/cognigo-mcp" || true
  [ "${STARTED_API:-0}" -eq 0 ] || stop_process api "$API_PID_FILE" "bin/cognigo" || true
}

stop() {
  stop_process frontend "$FRONTEND_PID_FILE" "npm run serve"
  stop_process mcp "$MCP_PID_FILE" "common/mcp/bin/cognigo-mcp"
  stop_process api "$API_PID_FILE" "bin/cognigo"
  load_env_optional
  if docker info >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    say "stopping this repository's Compose project"
    compose down
  else
    warn "Docker is unavailable; no Compose services were stopped"
  fi
}

status_process() {
  name=$1
  file=$2
  expected=$3
  pid=$(pid_value "$file" 2>/dev/null || true)
  if [ -n "$pid" ] && process_owned "$pid" "$expected"; then
    printf '  %-9s running (pid %s)\n' "$name" "$pid"
  else
    printf '  %-9s stopped\n' "$name"
  fi
}

status() {
  load_env_optional
  printf 'CogniGo demo status\n'
  printf '  root:     %s\n' "$ROOT_DIR"
  printf '  runtime:  %s\n' "$RUNTIME_DIR"
  printf '  compose:  %s\n' "$COMPOSE_PROJECT_NAME"
  printf '  endpoints: api=%s frontend=%s mcp=%s mailpit=%s\n' \
    "http://127.0.0.1:$API_PORT" "http://127.0.0.1:$FRONTEND_PORT" \
    "http://127.0.0.1:$MCP_PORT/mcp" "http://127.0.0.1:$MAILPIT_WEB_PORT"
  status_process api "$API_PID_FILE" "bin/cognigo"
  status_process mcp "$MCP_PID_FILE" "common/mcp/bin/cognigo-mcp"
  status_process frontend "$FRONTEND_PID_FILE" "npm run serve"
  printf '  api HTTP:     %s\n' "$(http_code "http://127.0.0.1:$API_PORT/")"
  printf '  frontend HTTP: %s\n' "$(http_code "http://127.0.0.1:$FRONTEND_PORT/")"
  printf '  mcp HTTP:     %s (400 is expected for a browser GET)\n' "$(http_code "http://127.0.0.1:$MCP_PORT/mcp")"
  printf '  mailpit HTTP: %s\n' "$(http_code "http://127.0.0.1:$MAILPIT_WEB_PORT/")"
  if docker info >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    compose ps || true
  else
    warn "Docker is unavailable; skipping Compose status"
  fi
  printf '  logs: %s\n' "$RUNTIME_DIR"
}

logs() {
  follow=0
  service=all
  if [ "${1:-}" = "-f" ]; then
    follow=1
    shift
  fi
  [ "$#" -eq 0 ] || service=$1
  case "$service" in
    api) files=$API_LOG ;;
    mcp) files=$MCP_LOG ;;
    frontend) files=$FRONTEND_LOG ;;
    all) files="$API_LOG $MCP_LOG $FRONTEND_LOG" ;;
    *) die "unknown log service: $service (use api, mcp, frontend, or all)" ;;
  esac
  mkdir -p "$RUNTIME_DIR"
  if [ "$follow" -eq 1 ]; then
    for file in $files; do
      [ -f "$file" ] || : >"$file"
    done
    # shellcheck disable=SC2086
    exec tail -f $files
  fi
  for file in $files; do
    printf '\n--- %s ---\n' "$file"
    if [ -f "$file" ]; then tail -n 80 "$file"; else printf '(no log yet)\n'; fi
  done
}

main() {
  command=${1:-}
  shift || true
  case "$command" in
    start) [ "$#" -eq 0 ] || die "start does not accept arguments"; start ;;
    stop) [ "$#" -eq 0 ] || die "stop does not accept arguments"; stop ;;
    status) [ "$#" -eq 0 ] || die "status does not accept arguments"; status ;;
    logs) logs "$@" ;;
    -h|--help|help|'') usage ;;
    *) usage >&2; exit 2 ;;
  esac
}

main "$@"
