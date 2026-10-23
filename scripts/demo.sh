#!/usr/bin/env sh
set -eu

ROOT_DIR=$(cd -- "$(dirname -- "$0")/.." && pwd)
RUNTIME_DIR=${COGNIGO_DEMO_RUNTIME_DIR:-"$ROOT_DIR/.local/demo"}
ENV_FILE=${DEMO_ENV_FILE:-"$ROOT_DIR/.env"}
WAIT_SECONDS=${COGNIGO_DEMO_WAIT_SECONDS:-180}
PROJECT_SUFFIX=$(printf '%s' "$ROOT_DIR" | cksum | awk '{print $1}')
COMPOSE_PROJECT_NAME="cognigo-demo-$PROJECT_SUFFIX"
DOCKER_BIN=${COGNIGO_DOCKER_BIN:-}

API_PID_FILE="$RUNTIME_DIR/api.pid"
MCP_PID_FILE="$RUNTIME_DIR/mcp.pid"
FRONTEND_PID_FILE="$RUNTIME_DIR/frontend.pid"
API_PGID_FILE="$RUNTIME_DIR/api.pgid"
MCP_PGID_FILE="$RUNTIME_DIR/mcp.pgid"
FRONTEND_PGID_FILE="$RUNTIME_DIR/frontend.pgid"
API_LOG="$RUNTIME_DIR/api.log"
MCP_LOG="$RUNTIME_DIR/mcp.log"
FRONTEND_LOG="$RUNTIME_DIR/frontend.log"

API_PORT=${COGNIGO_PORT:-9090}
MCP_PORT=${COGNIGO_MCP_PORT:-8081}
FRONTEND_PORT=${COGNIGO_FRONTEND_PORT:-8080}
MAILPIT_WEB_PORT=${COGNIGO_MAILPIT_WEB_PORT:-8025}

usage() {
  cat <<'EOF'
Usage: scripts/demo.sh [command]

Commands:
  start   Start Compose dependencies, API, MCP, and the frontend (default)
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
  MAILPIT_WEB_PORT=${COGNIGO_MAILPIT_WEB_PORT:-8025}
  WAIT_SECONDS=${COGNIGO_DEMO_WAIT_SECONDS:-180}
}

load_env_optional() {
  if [ -f "$ENV_FILE" ]; then
    load_env
  else
    warn "missing $ENV_FILE; using built-in demo defaults"
  fi
}

compose() {
  (cd "$ROOT_DIR" && docker_cmd compose --project-name "$COMPOSE_PROJECT_NAME" "$@")
}

resolve_docker() {
  [ -n "$DOCKER_BIN" ] && return 0
  if command -v docker >/dev/null 2>&1; then
    DOCKER_BIN=$(command -v docker)
  elif command -v docker.exe >/dev/null 2>&1; then
    DOCKER_BIN=$(command -v docker.exe)
  elif [ -x '/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe' ]; then
    DOCKER_BIN='/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe'
  else
    return 1
  fi
}

docker_cmd() {
  "$DOCKER_BIN" "$@"
}

docker_available() {
  resolve_docker && docker_cmd info >/dev/null 2>&1 && docker_cmd compose version >/dev/null 2>&1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

check_prerequisites() {
  require_command go
  require_command node
  require_command npm
  require_command curl
  resolve_docker || die "Docker CLI not found; install Docker or enable Docker Desktop WSL integration"
  docker_cmd info >/dev/null 2>&1 || die "Docker daemon is unavailable; start Docker Desktop"
  docker_cmd compose version >/dev/null 2>&1 || die "Docker Compose is unavailable"
  [ -f "$ROOT_DIR/config/config.toml" ] || die "missing config/config.toml"
}

port_in_use() {
  port=$1
  case "$port" in
    ''|*[!0-9]*) return 1 ;;
  esac
  if command -v lsof >/dev/null 2>&1 && lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null | head -n 1 | grep -q .; then
    return 0
  fi
  if command -v ss >/dev/null 2>&1 && ss -ltnH 2>/dev/null | awk -v port=":$port" 'index($4, port) == length($4) - length(port) + 1 {found=1} END {exit found ? 0 : 1}'; then
    return 0
  fi
  if command -v bash >/dev/null 2>&1 && bash -c 'exec 3<>"/dev/tcp/127.0.0.1/$1"' bash "$port" >/dev/null 2>&1; then
    return 0
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
  command_pid=$1
  ps -p "$command_pid" -o command= 2>/dev/null || true
}

process_group_id() {
  group_pid=$1
  ps -p "$group_pid" -o pgid= 2>/dev/null | awk 'NF {print $1; exit}'
}

process_group_members() {
  members_group=$1
  ps -eo pid=,pgid= 2>/dev/null | awk -v group="$members_group" '$2 == group {print $1}'
}

process_command_matches() {
  matched_command=$1
  matched_expected=$2
  case "$matched_expected" in
    "npm run serve")
      case "$matched_command" in
        *"npm run serve"*|*"vue-cli-service"*"serve"*) return 0 ;;
        *) return 1 ;;
      esac
      ;;
    *)
      case "$matched_command" in
        *"$matched_expected"*) return 0 ;;
        *) return 1 ;;
      esac
      ;;
  esac
}

process_cwd() {
  cwd_pid=$1
  if [ -e "/proc/$cwd_pid/cwd" ]; then
    readlink "/proc/$cwd_pid/cwd" 2>/dev/null || true
  elif command -v lsof >/dev/null 2>&1; then
    lsof -a -p "$cwd_pid" -d cwd -Fn 2>/dev/null | sed -n 's/^n//p' | head -n 1
  fi
}

process_state() {
  state_pid=$1
  ps -p "$state_pid" -o state= 2>/dev/null | awk 'NF {print $1; exit}'
}

# kill -0 also succeeds for zombies, so require a live process state.
process_running() {
  running_pid=$1
  kill -0 "$running_pid" 2>/dev/null || return 1
  running_state=$(process_state "$running_pid")
  case "$running_state" in
    ''|Z*|z*|X*|x*) return 1 ;;
  esac
  return 0
}

process_group_has_live_members() {
  live_group=$1
  for live_member in $(process_group_members "$live_group"); do
    process_running "$live_member" && return 0
  done
  return 1
}

process_identity_owned() {
  identity_pid=$1
  identity_expected=$2
  process_running "$identity_pid" || return 1
  identity_command=$(process_command "$identity_pid")
  identity_cwd=$(process_cwd "$identity_pid")
  process_command_matches "$identity_command" "$identity_expected" || return 1
  case "$identity_cwd" in
    "$ROOT_DIR"|"$ROOT_DIR"/*) return 0 ;;
    *) return 1 ;;
  esac
}

pgid_value() {
  pgid_file=$1
  [ -s "$pgid_file" ] || return 1
  pgid_content=$(sed -n '1p' "$pgid_file")
  case "$pgid_content" in
    ''|*[!0-9]*) return 1 ;;
  esac
  printf '%s\n' "$pgid_content"
}

record_process_group() {
  record_pid=$1
  record_file=$2
  record_pgid=$(process_group_id "$record_pid" || true)
  case "$record_pgid" in
    ''|0|"$$") rm -f "$record_file" ;;
    *) printf '%s\n' "$record_pgid" >"$record_file" ;;
  esac
}

process_group_owned() {
  owner_pid=$1
  owner_expected=$2
  owner_group_file=${3:-}
  owner_group=$(pgid_value "$owner_group_file" 2>/dev/null || true)
  [ -n "$owner_group" ] || owner_group=$(process_group_id "$owner_pid" 2>/dev/null || true)
  [ -n "$owner_group" ] || return 1
  owner_members=$(process_group_members "$owner_group")
  [ -n "$owner_members" ] || return 1
  for owner_member in $owner_members; do
    process_identity_owned "$owner_member" "$owner_expected" && return 0
  done
  return 1
}

process_owned() {
  owned_pid=$1
  owned_expected=$2
  owned_group_file=${3:-}
  process_running "$owned_pid" || return 1
  process_identity_owned "$owned_pid" "$owned_expected" || process_group_owned "$owned_pid" "$owned_expected" "$owned_group_file"
}

recorded_running() {
  recorded_file=$1
  recorded_expected=$2
  recorded_group_file=${3:-}
  recorded_pid=$(pid_value "$recorded_file" 2>/dev/null || true)
  [ -n "$recorded_pid" ] && process_owned "$recorded_pid" "$recorded_expected" "$recorded_group_file"
}

clear_stale_pid() {
  stale_file=$1
  stale_expected=$2
  stale_group_file=${3:-}
  stale_pid=$(pid_value "$stale_file" 2>/dev/null || true)
  [ -z "$stale_pid" ] && return 0
  recorded_running "$stale_file" "$stale_expected" "$stale_group_file" && return 0
  rm -f "$stale_file"
  rm -f "$stale_group_file"
}

listener_pid() {
  port=$1
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null | head -n 1
    return 0
  fi
  if command -v ss >/dev/null 2>&1; then
    ss -ltnpH 2>/dev/null \
      | awk -v port=":$port" '$4 ~ (port "$" ) {print}' \
      | sed -n 's/.*pid=\([0-9][0-9]*\).*/\1/p' \
      | head -n 1
  fi
}

recover_listener() {
  name=$1
  port=$2
  file=$3
  group_file=$4
  expected=$5
  listener=$(listener_pid "$port" || true)
  [ -n "$listener" ] || return 1
  process_owned "$listener" "$expected" || return 1
  printf '%s\n' "$listener" >"$file"
  record_process_group "$listener" "$group_file"
  say "recovered $name (pid $listener)"
  return 0
}

ensure_app_ports_free() {
  for entry in "api:$API_PORT" "mcp:$MCP_PORT" "frontend:$FRONTEND_PORT"; do
    service=${entry%%:*}
    port=${entry#*:}
    if port_in_use "$port"; then
      case "$service" in
        api) expected="bin/cognigo"; file=$API_PID_FILE; group_file=$API_PGID_FILE ;;
        mcp) expected="common/mcp/bin/cognigo-mcp"; file=$MCP_PID_FILE; group_file=$MCP_PGID_FILE ;;
        frontend) expected="npm run serve"; file=$FRONTEND_PID_FILE; group_file=$FRONTEND_PGID_FILE ;;
      esac
      recorded_running "$file" "$expected" "$group_file" || \
        recover_listener "$service" "$port" "$file" "$group_file" "$expected" || \
        die "port $port for $service is already in use; set its COGNIGO_*_PORT/COGNIGO_FRONTEND_PORT or stop the owning process"
    fi
  done
}

build_if_needed() {
  if binary_needs_build "$ROOT_DIR/bin/cognigo" "$ROOT_DIR"; then
    say "building API binary"
    (cd "$ROOT_DIR" && make build)
  fi
  if binary_needs_build "$ROOT_DIR/common/mcp/bin/cognigo-mcp" "$ROOT_DIR/common/mcp"; then
    say "building MCP binary"
    (cd "$ROOT_DIR" && make build-mcp)
  fi
  if [ ! -x "$ROOT_DIR/vue-frontend/node_modules/.bin/vue-cli-service" ]; then
    say "installing frontend dependencies"
    (cd "$ROOT_DIR" && make frontend-install)
  fi
}

binary_needs_build() {
  build_target=$1
  build_source_dir=$2
  [ ! -x "$build_target" ] && return 0
  for build_metadata in "$ROOT_DIR/Makefile" "$build_source_dir/go.mod" "$build_source_dir/go.sum"; do
    [ -f "$build_metadata" ] || continue
    if find "$build_metadata" -newer "$build_target" -print -quit 2>/dev/null | grep -q .; then
      return 0
    fi
  done
  find "$build_source_dir" -type f -name '*.go' -newer "$build_target" -print -quit 2>/dev/null | grep -q .
}

compose_service_ready() {
  service=$1
  container=$(compose ps -q "$service" 2>/dev/null | head -n 1)
  [ -n "$container" ] || return 1
  state=$(docker_cmd inspect --format '{{.State.Status}}' "$container" 2>/dev/null || true)
  [ "$state" = running ] || return 1
  health=$(docker_cmd inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container" 2>/dev/null || true)
  [ "$health" = healthy ] || [ "$health" = none ]
}

ensure_compose_ports_free() {
  for entry in \
    "mysql:${COGNIGO_MYSQL_PORT:-3306}" \
    "redis:${COGNIGO_REDIS_PORT:-6379}" \
    "rabbitmq:${COGNIGO_RABBITMQ_PORT:-5672}" \
    "rabbitmq:${COGNIGO_RABBITMQ_MANAGEMENT_PORT:-15672}" \
    "mailpit:${COGNIGO_SMTP_PORT:-1025}" \
    "mailpit:$MAILPIT_WEB_PORT"; do
    service=${entry%%:*}
    port=${entry#*:}
    if port_in_use "$port" && ! compose_service_ready "$service"; then
      die "Compose host port $port for $service is already in use; set the matching COGNIGO_*_PORT in $ENV_FILE"
    fi
  done
}

wait_for_compose() {
  deadline=$(( $(date +%s) + WAIT_SECONDS ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    ready=1
    for entry in \
      "mysql:${COGNIGO_MYSQL_PORT:-3306}" \
      "redis:${COGNIGO_REDIS_PORT:-6379}" \
      "rabbitmq:${COGNIGO_RABBITMQ_PORT:-5672}" \
      "mailpit:${COGNIGO_SMTP_PORT:-1025}"; do
      service=${entry%%:*}
      port=${entry#*:}
      compose_service_ready "$service" || ready=0
      port_in_use "$port" || ready=0
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
    endpoint_ready "http://127.0.0.1:$API_PORT/" 200,404 && \
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
  case "$name" in
    api) group_file=$API_PGID_FILE ;;
    mcp) group_file=$MCP_PGID_FILE ;;
    *) group_file="" ;;
  esac

  clear_stale_pid "$pid_file" "$expected" "$group_file"
  if recorded_running "$pid_file" "$expected" "$group_file"; then
    say "$name already running (pid $(pid_value "$pid_file"))"
    return 0
  fi

  say "starting $name"
  if command -v setsid >/dev/null 2>&1; then
    # API and MCP load configuration from paths relative to the repository root.
    # shellcheck disable=SC2016
    setsid sh -c 'root=$1; mcp_url=$2; shift 2; cd "$root" && exec env COGNIGO_MCP_URL="$mcp_url" "$@"' \
      sh "$ROOT_DIR" "http://127.0.0.1:$MCP_PORT/mcp" "$@" >"$log_file" 2>&1 &
  else
    (
      cd "$ROOT_DIR"
      exec env COGNIGO_MCP_URL="http://127.0.0.1:$MCP_PORT/mcp" "$@"
    ) >"$log_file" 2>&1 &
  fi
  started_pid=$!
  printf '%s\n' "$started_pid" >"$pid_file"
  record_process_group "$started_pid" "$group_file"
  case "$name" in
    api) STARTED_API=1 ;;
    mcp) STARTED_MCP=1 ;;
  esac
  deadline=$(( $(date +%s) + 10 ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    process_owned "$started_pid" "$expected" "$group_file" && return 0
    process_running "$started_pid" || die "$name exited during startup; inspect $log_file"
    sleep 1
  done
  die "$name exited during startup; inspect $log_file"
}

start_frontend() {
  clear_stale_pid "$FRONTEND_PID_FILE" "npm run serve" "$FRONTEND_PGID_FILE"
  if recorded_running "$FRONTEND_PID_FILE" "npm run serve" "$FRONTEND_PGID_FILE"; then
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
  started_pid=$!
  printf '%s\n' "$started_pid" >"$FRONTEND_PID_FILE"
  record_process_group "$started_pid" "$FRONTEND_PGID_FILE"
  STARTED_FRONTEND=1
  deadline=$(( $(date +%s) + 10 ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    process_owned "$started_pid" "npm run serve" "$FRONTEND_PGID_FILE" && return 0
    process_running "$started_pid" || die "frontend exited during startup; inspect $FRONTEND_LOG"
    sleep 1
  done
  die "frontend exited during startup; inspect $FRONTEND_LOG"
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
  ensure_compose_ports_free
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
  group_file=$4
  port=${5:-}
  pid=$(pid_value "$file" 2>/dev/null || true)
  if [ -z "$pid" ] && [ -n "$port" ]; then
    pid=$(listener_pid "$port" || true)
  fi
  if [ -z "$pid" ]; then
    say "$name is not recorded"
    return 0
  fi
  if ! process_owned "$pid" "$expected" "$group_file"; then
    warn "not stopping pid $pid for $name because ownership could not be verified"
    rm -f "$file"
    rm -f "$group_file"
    return 0
  fi
  say "stopping $name (pid $pid)"
  group=$(pgid_value "$group_file" 2>/dev/null || process_group_id "$pid" 2>/dev/null || true)
  if [ -n "$group" ] && [ "$group" != "$$" ]; then
    kill -TERM -"$group" 2>/dev/null || kill -TERM "$pid" 2>/dev/null || true
  else
    kill -TERM "$pid" 2>/dev/null || true
  fi
  deadline=$(( $(date +%s) + 10 ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if [ -n "$group" ]; then
      process_group_has_live_members "$group" || break
    elif ! process_running "$pid"; then
      break
    fi
    sleep 1
  done
  if [ -n "$group" ] && process_group_has_live_members "$group"; then
    warn "$name did not stop after 10s; leaving it running"
  elif [ -z "$group" ] && process_running "$pid"; then
    warn "$name did not stop after 10s; leaving it running"
  else
    rm -f "$file"
    rm -f "$group_file"
  fi
}

cleanup_started() {
  [ "${START_IN_PROGRESS:-0}" -eq 1 ] || return 0
  START_IN_PROGRESS=0
  warn "startup did not complete; stopping processes started by this invocation"
  [ "${STARTED_FRONTEND:-0}" -eq 0 ] || stop_process frontend "$FRONTEND_PID_FILE" "npm run serve" "$FRONTEND_PGID_FILE" "$FRONTEND_PORT" || true
  [ "${STARTED_MCP:-0}" -eq 0 ] || stop_process mcp "$MCP_PID_FILE" "common/mcp/bin/cognigo-mcp" "$MCP_PGID_FILE" "$MCP_PORT" || true
  [ "${STARTED_API:-0}" -eq 0 ] || stop_process api "$API_PID_FILE" "bin/cognigo" "$API_PGID_FILE" "$API_PORT" || true
}

stop() {
  load_env_optional
  stop_process frontend "$FRONTEND_PID_FILE" "npm run serve" "$FRONTEND_PGID_FILE" "$FRONTEND_PORT"
  stop_process mcp "$MCP_PID_FILE" "common/mcp/bin/cognigo-mcp" "$MCP_PGID_FILE" "$MCP_PORT"
  stop_process api "$API_PID_FILE" "bin/cognigo" "$API_PGID_FILE" "$API_PORT"
  if docker_available; then
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
  group_file=$4
  port=${5:-}
  pid=$(pid_value "$file" 2>/dev/null || true)
  if [ -z "$pid" ] && [ -n "$port" ]; then
    pid=$(listener_pid "$port" || true)
  fi
  if [ -n "$pid" ] && process_owned "$pid" "$expected" "$group_file"; then
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
  status_process api "$API_PID_FILE" "bin/cognigo" "$API_PGID_FILE" "$API_PORT"
  status_process mcp "$MCP_PID_FILE" "common/mcp/bin/cognigo-mcp" "$MCP_PGID_FILE" "$MCP_PORT"
  status_process frontend "$FRONTEND_PID_FILE" "npm run serve" "$FRONTEND_PGID_FILE" "$FRONTEND_PORT"
  printf '  api HTTP:     %s\n' "$(http_code "http://127.0.0.1:$API_PORT/")"
  printf '  frontend HTTP: %s\n' "$(http_code "http://127.0.0.1:$FRONTEND_PORT/")"
  printf '  mcp HTTP:     %s (400 is expected for a browser GET)\n' "$(http_code "http://127.0.0.1:$MCP_PORT/mcp")"
  printf '  mailpit HTTP: %s\n' "$(http_code "http://127.0.0.1:$MAILPIT_WEB_PORT/")"
  if docker_available; then
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
  command=${1:-start}
  [ "$#" -eq 0 ] || shift
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
