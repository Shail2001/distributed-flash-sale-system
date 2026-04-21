#!/usr/bin/env bash
set -euo pipefail

# Full Experiment 2 matrix runner:
# strategies x user-levels x API replica counts, with optional consistency checks.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RESULTS_DIR="${RESULTS_DIR:-$ROOT_DIR/tests/results/exp2}"
CONSISTENCY_DIR="$RESULTS_DIR/consistency"
LOCUST_RUNNER="$ROOT_DIR/scripts/run_experiment2_locust.sh"

HOST="${HOST:-}"
STRATEGIES="${STRATEGIES:-atomic_decr optimistic lua_script}"
USERS_LEVELS="${USERS_LEVELS:-200 500 1000}"
REPLICA_COUNTS="${REPLICA_COUNTS:-1 2 4}"
SPAWN_RATE="${SPAWN_RATE:-80}"
DURATION="${DURATION:-90s}"
PURCHASE_QUANTITY="${PURCHASE_QUANTITY:-1}"

SCALE_API="${SCALE_API:-true}"
ECS_CLUSTER="${ECS_CLUSTER:-}"
ECS_SERVICE="${ECS_SERVICE:-}"
RUN_CONSISTENCY_CHECK="${RUN_CONSISTENCY_CHECK:-true}"
CONSISTENCY_STRICT="${CONSISTENCY_STRICT:-true}"

if [[ ! -x "$LOCUST_RUNNER" ]]; then
  echo "ERROR: Missing executable $LOCUST_RUNNER" >&2
  exit 1
fi

if [[ -z "$HOST" ]]; then
  if command -v terraform >/dev/null 2>&1; then
    HOST="http://$(cd "$ROOT_DIR/terraform" && terraform output -raw alb_dns_name)"
  else
    echo "ERROR: HOST is required when terraform is unavailable" >&2
    exit 1
  fi
fi

mkdir -p "$RESULTS_DIR" "$CONSISTENCY_DIR"

if [[ "$SCALE_API" == "true" ]]; then
  if [[ -z "$ECS_CLUSTER" ]]; then
    ECS_CLUSTER="$(cd "$ROOT_DIR/terraform" && terraform output -raw ecs_cluster_name)"
  fi
  if [[ -z "$ECS_SERVICE" ]]; then
    ECS_SERVICE="$(cd "$ROOT_DIR/terraform" && terraform output -raw flash_sale_api_service_name)"
  fi
fi

echo "Starting Experiment 2 matrix"
echo "  host=$HOST"
echo "  strategies=$STRATEGIES"
echo "  users=$USERS_LEVELS"
echo "  replicas=$REPLICA_COUNTS"
echo "  results_dir=$RESULTS_DIR"

for replicas in $REPLICA_COUNTS; do
  if [[ "$SCALE_API" == "true" ]]; then
    echo "Scaling $ECS_SERVICE to $replicas replicas..."
    aws ecs update-service \
      --cluster "$ECS_CLUSTER" \
      --service "$ECS_SERVICE" \
      --desired-count "$replicas" >/dev/null
    aws ecs wait services-stable --cluster "$ECS_CLUSTER" --services "$ECS_SERVICE"
  fi

  for strategy in $STRATEGIES; do
    for users in $USERS_LEVELS; do
      run_tag="exp2_${strategy}_u${users}_r${replicas}"
      echo "------------------------------------------------------------"
      echo "Run: $run_tag"

      HOST="$HOST" \
      USERS="$users" \
      SPAWN_RATE="$SPAWN_RATE" \
      DURATION="$DURATION" \
      PURCHASE_QUANTITY="$PURCHASE_QUANTITY" \
      INVENTORY_STRATEGY="$strategy" \
      API_DESIRED_COUNT="$replicas" \
      ECS_CLUSTER="$ECS_CLUSTER" \
      ECS_SERVICE="$ECS_SERVICE" \
      RESULTS_DIR="$RESULTS_DIR" \
      RUN_TAG="$run_tag" \
      "$LOCUST_RUNNER"

      if [[ "$RUN_CONSISTENCY_CHECK" == "true" ]]; then
        echo "Running consistency check for $run_tag..."
        set +e
        (
          cd "$ROOT_DIR"
          python3 scripts/consistency_check.py
        ) | tee "$CONSISTENCY_DIR/${run_tag}_consistency.log"
        consistency_exit="${PIPESTATUS[0]}"
        set -e

        if [[ -f "$ROOT_DIR/consistency_result.json" ]]; then
          mv "$ROOT_DIR/consistency_result.json" "$CONSISTENCY_DIR/${run_tag}_consistency.json"
        fi

        if [[ "$consistency_exit" -ne 0 ]]; then
          msg="Consistency check failed for $run_tag (exit=$consistency_exit)"
          if [[ "$CONSISTENCY_STRICT" == "true" ]]; then
            echo "ERROR: $msg" >&2
            exit "$consistency_exit"
          fi
          echo "WARNING: $msg" >&2
        fi
      fi
    done
  done
done

echo "Experiment 2 matrix completed successfully."
