#!/usr/bin/env bash
# Packaging only; controller production resource operations use the Engine API.
set -Eeuo pipefail
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
source "$ROOT/deploy/controller/images.env"
LAB_ROOT=${MEALCHECK_LAB_ROOT:-/tmp/controller-lab}
MODEL_SOURCE=${MEALCHECK_LAB_MODEL_SOURCE:-/Users/chranama/infra/models/gguf/Qwen3-0.6B-Q4_K_M.gguf}
LABEL=mealcheck.packaging=lab
PREFIX=mealcheck-packaging-lab
command=${1:-help}
owned() {
  [[ $(docker inspect -f '{{index .Config.Labels "mealcheck.packaging"}}' "$1") == lab ]]
}
case "$command" in
build)
  docker build -f "$ROOT/deploy/controller/Dockerfile" -t "$API_IMAGE" "$ROOT"
  ;;
prepare)
  umask 077
  mkdir -p "$LAB_ROOT/models" "$LAB_ROOT/secrets"
  if [[ ! -e "$LAB_ROOT/models/model.gguf" ]]; then cp "$MODEL_SOURCE" "$LAB_ROOT/models/model.gguf"; fi
  actual=$(shasum -a 256 "$LAB_ROOT/models/model.gguf" | awk '{print $1}')
  [[ "$actual" == "$MODEL_SHA256" ]] || { echo 'model checksum mismatch' >&2; exit 1; }
  if [[ ! -e "$LAB_ROOT/secrets/postgres-password" ]]; then
    openssl rand -hex 32 > "$LAB_ROOT/secrets/postgres-password"
  fi
  password=$(cat "$LAB_ROOT/secrets/postgres-password")
  [[ "$password" =~ ^[a-f0-9]{64}$ ]] || { echo 'lab password must be generated hex' >&2; exit 1; }
  printf 'postgres://mealcheck:%s@postgres:5432/mealcheck?sslmode=disable\n' "$password" > "$LAB_ROOT/secrets/database-url"
  chmod 600 "$LAB_ROOT/secrets/postgres-password" "$LAB_ROOT/secrets/database-url"
  ;;
up)
  [[ -f "$LAB_ROOT/secrets/database-url" && -f "$LAB_ROOT/models/model.gguf" ]] || { echo 'run prepare first' >&2; exit 1; }
  # Fresh packaging resources only: refuse collision rather than adopting.
  docker network create --label "$LABEL" "$PREFIX-network"
  for volume in "$PREFIX-database" "$PREFIX-artifacts"; do
    if docker volume inspect "$volume" >/dev/null 2>&1; then
      [[ $(docker volume inspect -f '{{index .Labels "mealcheck.packaging"}}' "$volume") == lab ]] || { echo 'unowned volume collision' >&2; exit 1; }
    else
      docker volume create --label "$LABEL" "$volume"
    fi
  done
  docker run -d --name "$PREFIX-postgres" --label "$LABEL" --network "$PREFIX-network" --network-alias postgres \
    --cpus 2 --memory 1g -e POSTGRES_DB=mealcheck -e POSTGRES_USER=mealcheck \
    -e POSTGRES_PASSWORD_FILE=/run/secrets/postgres-password \
    -v "$LAB_ROOT/secrets/postgres-password:/run/secrets/postgres-password:ro" \
    -v "$PREFIX-database:/var/lib/postgresql/data" "$POSTGRES_IMAGE"
  docker run -d --name "$PREFIX-model" --label "$LABEL" --network "$PREFIX-network" --network-alias model \
    --cpus 8 --memory 4g -v "$LAB_ROOT/models/model.gguf:/models/model.gguf:ro" \
    "$MODEL_IMAGE" --model /models/model.gguf --alias mealcheck-lab-model --host 0.0.0.0 --port 8080 \
    --ctx-size 8192 --threads 8 --n-gpu-layers 0
  ready=0
  for ((i=0;i<60;i++)); do
    if docker exec "$PREFIX-postgres" pg_isready -U mealcheck -d mealcheck >/dev/null; then ready=1; break; fi
    sleep 1
  done
  [[ "$ready" == 1 ]] || { echo 'database startup timed out' >&2; exit 1; }
  docker run -d --name "$PREFIX-api" --label "$LABEL" --network "$PREFIX-network" --network-alias api \
    --cpus 2 --memory 2g -p 127.0.0.1:18080:8080 \
    -v "$LAB_ROOT/secrets/database-url:/run/secrets/database-url:ro" \
    -v "$PREFIX-artifacts:/var/lib/mealcheck/artifacts" \
    -e MEALCHECK_ADDR=0.0.0.0:8080 -e MEALCHECK_STORE=postgres -e MEALCHECK_HOSTED_MODE=local_model \
    -e MEALCHECK_LOCAL_MODEL_ENABLED=true -e MEALCHECK_LOCAL_MODEL_BASE_URL=http://model:8080/v1 \
    -e MEALCHECK_LOCAL_MODEL_NAME=mealcheck-lab-model -e MEALCHECK_DATA_DIR=/var/lib/mealcheck \
    -e MEALCHECK_ARTIFACT_DIR=/var/lib/mealcheck/artifacts \
    -e MEALCHECK_FNDDS_FALLBACK_PATH=/opt/mealcheck/data/reference/fndds-2021-2023/fndds.sqlite "$API_IMAGE"
  ;;
smoke)
  MEALCHECK_DEPLOYED_API_URL=http://127.0.0.1:18080 MEALCHECK_DELETE_RUNS=0 \
    MEALCHECK_POLL_ATTEMPTS=240 MEALCHECK_DEPLOYED_OUTPUT_DIR="$LAB_ROOT/smoke" \
    "$ROOT/scripts/test-deployed-local-model-live.sh"
  ;;
restart)
  for role in api model postgres; do owned "$PREFIX-$role"; docker stop "$PREFIX-$role" >/dev/null; done
  for role in postgres model api; do docker start "$PREFIX-$role" >/dev/null; done
  ;;
down)
  for role in api model postgres; do
    if docker container inspect "$PREFIX-$role" >/dev/null 2>&1; then owned "$PREFIX-$role"; docker rm -f "$PREFIX-$role"; fi
  done
  if docker network inspect "$PREFIX-network" >/dev/null 2>&1; then
    [[ $(docker network inspect -f '{{index .Labels "mealcheck.packaging"}}' "$PREFIX-network") == lab ]]
    docker network rm "$PREFIX-network"
  fi
  echo 'Database and artifact volumes retained.'
  ;;
*) echo "usage: $0 {build|prepare|up|smoke|restart|down}"; exit 2 ;;
esac
