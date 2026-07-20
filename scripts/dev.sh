#!/usr/bin/env bash
# Sobe api-go (api + collector) e o frontend em modo dev, SEM build de
# imagem docker — só o Postgres roda em container "de verdade" (o mesmo do
# docker-compose.yml). A api/collector rodam via `go run` dentro de um
# container golang:1.23-alpine descartável, com o código-fonte montado por
# volume (recompila na hora, sem passar pelo Dockerfile multi-stage); o
# frontend roda com `npm run dev` (Vite) direto no host.
#
# Uso:
#   ./scripts/dev.sh                  # postgres + api + web
#   ./scripts/dev.sh --with-collector # + collector (chama Huawei/FoxESS de
#                                        verdade — evite reiniciar toda hora,
#                                        a Huawei tem rate limit apertado)
#
# Portas padrão: api :8092, web :5173 — de propósito DIFERENTES das portas
# de produção do docker-compose (:8091/:8090), pra rodar os dois ao mesmo
# tempo sem conflito. Sobrescreva com DEV_API_PORT/DEV_WEB_PORT se precisar.
#
# Ctrl+C encerra tudo (trap abaixo). Loga tudo junto no mesmo terminal, cada
# linha prefixada com [api]/[collector]/[web] via sed.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

WITH_COLLECTOR=false
for arg in "$@"; do
  case "$arg" in
    --with-collector) WITH_COLLECTOR=true ;;
    *) echo "argumento desconhecido: $arg (uso: --with-collector)" >&2; exit 1 ;;
  esac
done

if [ ! -f .env ]; then
  echo "Não encontrei .env na raiz do projeto — copie .env.example pra .env e preencha antes de rodar." >&2
  exit 1
fi
set -a
# shellcheck disable=SC1091
source .env
set +a

for var in POSTGRES_PASSWORD JWT_SECRET CONFIG_ENCRYPTION_KEY; do
  if [ -z "${!var:-}" ]; then
    echo "Variável $var vazia em .env — preencha antes de rodar." >&2
    exit 1
  fi
done

API_PORT="${DEV_API_PORT:-8092}"
WEB_PORT="${DEV_WEB_PORT:-5173}"
GO_IMAGE="golang:1.23-alpine"
API_CONTAINER="solar-dev-api"
COLLECTOR_CONTAINER="solar-dev-collector"

PIDS=()
cleanup() {
  echo
  echo "Encerrando..."
  docker stop "$API_CONTAINER" >/dev/null 2>&1 || true
  if [ "$WITH_COLLECTOR" = true ]; then
    docker stop "$COLLECTOR_CONTAINER" >/dev/null 2>&1 || true
  fi
  for pid in "${PIDS[@]:-}"; do
    kill "$pid" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT INT TERM

echo "==> Subindo Postgres (docker compose, sem rebuild)..."
docker compose up -d postgres

echo "==> Esperando Postgres aceitar conexão..."
until docker compose exec -T postgres pg_isready -U "${POSTGRES_USER:-solarhome}" >/dev/null 2>&1; do
  sleep 1
done

# Descobre a rede real do container do postgres (não assume nome fixo de
# projeto docker-compose) pra api/collector conseguirem resolver o
# hostname "postgres" igual fariam dentro do compose.
NETWORK="$(docker inspect "$(docker compose ps -q postgres)" -f '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}')"
DATABASE_URL="postgres://${POSTGRES_USER:-solarhome}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB:-solarhome}?sslmode=disable"

docker rm -f "$API_CONTAINER" "$COLLECTOR_CONTAINER" >/dev/null 2>&1 || true

echo "==> Subindo api-go (go run, porta $API_PORT)..."
docker run --rm --name "$API_CONTAINER" \
  --network "$NETWORK" \
  -p "${API_PORT}:8000" \
  -v "$ROOT_DIR/api-go:/src" -w /src \
  -v go-dev-build-cache:/root/.cache/go-build \
  -v go-dev-mod-cache:/go/pkg/mod \
  -e DATABASE_URL="$DATABASE_URL" \
  -e JWT_SECRET="$JWT_SECRET" \
  -e CONFIG_ENCRYPTION_KEY="$CONFIG_ENCRYPTION_KEY" \
  "$GO_IMAGE" go run ./cmd/api 2>&1 | sed -u 's/^/[api] /' &
PIDS+=($!)

if [ "$WITH_COLLECTOR" = true ]; then
  echo "==> Subindo collector (go run — chama Huawei/FoxESS de verdade)..."
  docker run --rm --name "$COLLECTOR_CONTAINER" \
    --network "$NETWORK" \
    -v "$ROOT_DIR/api-go:/src" -w /src \
    -v go-dev-build-cache:/root/.cache/go-build \
    -v go-dev-mod-cache:/go/pkg/mod \
    -e DATABASE_URL="$DATABASE_URL" \
    -e CONFIG_ENCRYPTION_KEY="$CONFIG_ENCRYPTION_KEY" \
    "$GO_IMAGE" go run ./cmd/collector 2>&1 | sed -u 's/^/[collector] /' &
  PIDS+=($!)
fi

if [ ! -d "$ROOT_DIR/web/node_modules" ]; then
  echo "==> node_modules ausente, rodando npm ci..."
  (cd "$ROOT_DIR/web" && npm ci)
fi

echo "==> Subindo web (vite dev, porta $WEB_PORT)..."
(
  cd "$ROOT_DIR/web"
  # Vite faz proxy de /api pra api-go (ver vite.config.ts) — igual o nginx
  # faz em produção, o browser só fala com a própria origem.
  VITE_DEV_API_TARGET="http://localhost:${API_PORT}" npx vite --port "$WEB_PORT" 2>&1 | sed -u 's/^/[web] /'
) &
PIDS+=($!)

cat <<EOF

------------------------------------------------------------
  Painel (dev, Vite/HMR):  http://localhost:${WEB_PORT}
  API:                     http://localhost:${API_PORT}
  Postgres:                localhost:5432
  Collector:                $([ "$WITH_COLLECTOR" = true ] && echo "rodando" || echo "desligado (use --with-collector pra ligar)")
  Ctrl+C encerra tudo.
------------------------------------------------------------

EOF

wait
