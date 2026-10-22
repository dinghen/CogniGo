#!/usr/bin/env sh
set -u

failures=0
ROOT_DIR=$(cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

check_command() {
  if command -v "$1" >/dev/null 2>&1; then
    printf '[ok]   %s\n' "$1"
  else
    printf '[fail] %s is not installed\n' "$1"
    failures=$((failures + 1))
  fi
}

check_file() {
  if [ -s "$1" ]; then
    printf '[ok]   %s\n' "$1"
  else
    printf '[warn] %s is missing; run make models\n' "$1"
  fi
}

check_command go
check_command node
check_command npm
check_command curl
check_command tar

docker_bin=${COGNIGO_DOCKER_BIN:-}
if [ -z "$docker_bin" ] && command -v docker >/dev/null 2>&1; then
  docker_bin=$(command -v docker)
elif [ -z "$docker_bin" ] && command -v docker.exe >/dev/null 2>&1; then
  docker_bin=$(command -v docker.exe)
elif [ -z "$docker_bin" ] && [ -x '/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe' ]; then
  docker_bin='/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe'
fi

if [ -n "$docker_bin" ]; then
  if "$docker_bin" info >/dev/null 2>&1 && "$docker_bin" compose version >/dev/null 2>&1; then
    printf '[ok]   Docker daemon\n'
  else
    printf '[fail] Docker daemon or Compose is unavailable; start Docker Desktop\n'
    failures=$((failures + 1))
  fi
else
  printf '[fail] Docker CLI not found; install Docker or enable Docker Desktop WSL integration\n'
  failures=$((failures + 1))
fi

if [ -f .env ]; then
	printf '[ok]   .env\n'
	set -a
	. ./.env
	set +a
	if ! grep -Eq '^OPENAI_API_KEY=.+$' .env; then
    printf '[warn] OPENAI_API_KEY is empty; AI features will not work\n'
  fi
else
  printf '[warn] .env is missing; copy .env.example to .env\n'
fi

check_file "${COGNIGO_ONNX_LIBRARY_PATH:-.local/onnxruntime/lib/libonnxruntime.so.1.22.0}"
check_file "${COGNIGO_ONNX_MODEL_PATH:-models/mobilenetv2/mobilenetv2-7.onnx}"
check_file "${COGNIGO_LABELS_PATH:-imagenet_classes.txt}"

if [ "$failures" -ne 0 ]; then
  printf '\nDoctor found %d blocking environment issue(s).\n' "$failures"
  exit 1
fi

printf '\nEnvironment checks passed.\n'
