#!/usr/bin/env bash
set -euo pipefail

# Experiment 2 load harness:
# - optionally scales flash-sale-api ECS desired count
# - runs Locust in headless mode
# - writes CSV artifacts per run under tests/results/exp2

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCUST_FILE="$ROOT_DIR/tests/locust/locustfile.py"
RESULTS_DIR="$ROOT_DIR/tests/results/exp2"

HOST="${HOST:-}"
USERS="${USERS:-400}"
SPAWN_RATE="${SPAWN_RATE:-80}"
DURATION="${DURATION:-90s}"
PURCHASE_QUANTITY="${PURCHASE_QUANTITY:-1}"
INVENTORY_STRATEGY="${INVENTORY_STRATEGY:-atomic_decr}"
API_DESIRED_COUNT="${API_DESIRED_COUNT:-}"
ECS_CLUSTER="${ECS_CLUSTER:-}"
ECS_SERVICE="${ECS_SERVICE:-}"

if [[ -z "$HOST" ]]; then
  if command -v terraform >/dev/null 2>&1; then
    HOST="http://$(cd "$ROOT_DIR/terraform" && terraform output -raw alb_dns_name)"
  else
    echo "ERROR: HOST is required when terraform is unavailable" >&2
    exit 1
  fi
fi

if [[ -n "$API_DESIRED_COUNT" ]]; then
  if [[ -z "$ECS_CLUSTER" ]]; then
    ECS_CLUSTER="$(cd "$ROOT_DIR/terraform" && terraform output -raw ecs_cluster_name)"
  fi
  if [[ -z "$ECS_SERVICE" ]]; then
    ECS_SERVICE="$(cd "$ROOT_DIR/terraform" && terraform output -raw flash_sale_api_service_name)"
  fi
  echo "Scaling service $ECS_SERVICE to desired count=$API_DESIRED_COUNT"
  aws ecs update-service \
    --cluster "$ECS_CLUSTER" \
    --service "$ECS_SERVICE" \
    --desired-count "$API_DESIRED_COUNT" >/dev/null
  aws ecs wait services-stable --cluster "$ECS_CLUSTER" --services "$ECS_SERVICE"
fi

mkdir -p "$RESULTS_DIR"
STAMP="$(date +%Y%m%d_%H%M%S)"
RUN_ID="${INVENTORY_STRATEGY}_tasks-${API_DESIRED_COUNT:-unchanged}_${STAMP}"
CSV_PREFIX="$RESULTS_DIR/${RUN_ID}"
LOG_FILE="$RESULTS_DIR/${RUN_ID}.log"

echo "Starting Locust run"
echo "  host=$HOST"
echo "  users=$USERS spawn_rate=$SPAWN_RATE duration=$DURATION"
echo "  strategy=$INVENTORY_STRATEGY quantity=$PURCHASE_QUANTITY"
echo "  output_prefix=$CSV_PREFIX"

(
  cd "$ROOT_DIR/tests/locust"
  PURCHASE_QUANTITY="$PURCHASE_QUANTITY" \
  locust \
    --headless \
    --locustfile "$LOCUST_FILE" \
    --host "$HOST" \
    --users "$USERS" \
    --spawn-rate "$SPAWN_RATE" \
    --run-time "$DURATION" \
    --csv "$CSV_PREFIX" \
    --only-summary
) | tee "$LOG_FILE"

echo "Run complete. Artifacts:"
echo "  ${CSV_PREFIX}_stats.csv"
echo "  ${CSV_PREFIX}_stats_history.csv"
echo "  ${CSV_PREFIX}_failures.csv"
echo "  ${CSV_PREFIX}_exceptions.csv"
echo "  $LOG_FILE"
