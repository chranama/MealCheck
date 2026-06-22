#!/bin/bash
set -euo pipefail

PATH="/usr/local/bin:/opt/homebrew/bin:/usr/local/go/bin:/usr/bin:/bin:/usr/sbin:/sbin:${PATH:-}"

ENV_FILE="${MEALCHECK_LLAMA_ENV_FILE:-/Users/chranama-server/MealCheck-data/mealcheck-llama.env}"

LLAMA_SERVER_BIN="/Users/chranama-server/llama.cpp/build/bin/llama-server"
LLAMA_MODEL_PATH="/Users/chranama-server/MealCheck-data/models/Qwen3-0.6B-Q4_K_M.gguf"
LLAMA_HOST="127.0.0.1"
LLAMA_PORT="11435"
LLAMA_CTX_SIZE="2048"
LLAMA_THREADS="4"
LLAMA_BATCH_SIZE="256"
LLAMA_UBATCH_SIZE="64"
LLAMA_GPU_LAYERS="0"
LLAMA_PARALLEL="1"
LLAMA_CACHE_RAM="512"
LLAMA_EXTRA_ARGS=""

log() {
  printf '%s mealcheck-llama: %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*" >&2
}

if [ -f "$ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
else
  log "environment file not found; using wrapper defaults: $ENV_FILE"
fi

if [ ! -x "$LLAMA_SERVER_BIN" ]; then
  log "llama-server binary is not executable: $LLAMA_SERVER_BIN"
  exit 127
fi

if [ ! -f "$LLAMA_MODEL_PATH" ]; then
  log "model file not found: $LLAMA_MODEL_PATH"
  exit 2
fi

mkdir -p /Users/chranama-server/MealCheck-data/logs

args=(
  "$LLAMA_SERVER_BIN"
  -m "$LLAMA_MODEL_PATH"
  --host "$LLAMA_HOST"
  --port "$LLAMA_PORT"
  --threads "$LLAMA_THREADS"
  --ctx-size "$LLAMA_CTX_SIZE"
  --batch-size "$LLAMA_BATCH_SIZE"
  --ubatch-size "$LLAMA_UBATCH_SIZE"
  --gpu-layers "$LLAMA_GPU_LAYERS"
  --parallel "$LLAMA_PARALLEL"
  --cache-ram "$LLAMA_CACHE_RAM"
)

extra_args=()
if [ -n "${LLAMA_EXTRA_ARGS:-}" ]; then
  read -r -a extra_args <<<"$LLAMA_EXTRA_ARGS"
fi

log "starting llama-server model=$LLAMA_MODEL_PATH host=$LLAMA_HOST port=$LLAMA_PORT threads=$LLAMA_THREADS ctx=$LLAMA_CTX_SIZE gpu_layers=$LLAMA_GPU_LAYERS parallel=$LLAMA_PARALLEL"
if [ "${#extra_args[@]}" -gt 0 ]; then
  exec "${args[@]}" "${extra_args[@]}"
fi
exec "${args[@]}"
