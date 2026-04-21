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
WAIT_FOR_WORKER_DRAIN="${WAIT_FOR_WORKER_DRAIN:-true}"
WORKER_DRAIN_TIMEOUT_SECONDS="${WORKER_DRAIN_TIMEOUT_SECONDS:-180}"
SQS_QUEUE_URL="${SQS_QUEUE_URL:-}"
START_STRATEGY="${START_STRATEGY:-}"

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

if [[ -z "$SQS_QUEUE_URL" ]] && command -v terraform >/dev/null 2>&1; then
  SQS_QUEUE_URL="$(cd "$ROOT_DIR/terraform" && terraform output -raw orders_queue_url 2>/dev/null || true)"
fi

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

  start_seen=false
  for strategy in $STRATEGIES; do
    if [[ -n "$START_STRATEGY" && "$start_seen" == "false" ]]; then
      if [[ "$strategy" != "$START_STRATEGY" ]]; then
        continue
      fi
      start_seen=true
    fi
    for users in $USERS_LEVELS; do
      run_tag="exp2_${strategy}_u${users}_r${replicas}"
      echo "------------------------------------------------------------"
      echo "Run: $run_tag"

      echo "Resetting inventory before run..."
      reset_status="$(curl -s -o /dev/null -w "%{http_code}" -X POST "$HOST/reset" || true)"
      if [[ "$reset_status" != "200" ]]; then
        echo "ERROR: inventory reset failed for $run_tag (status=$reset_status)" >&2
        exit 1
      fi
      run_inventory_count="$(curl -s "$HOST/inventory" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("remaining", 0))')"
      echo "Inventory after reset: $run_inventory_count"

      baseline_confirmed=0
      baseline_order_count=0
      if [[ "$RUN_CONSISTENCY_CHECK" == "true" ]]; then
        baseline_confirmed="$(aws dynamodb get-item \
          --table-name "$INVENTORY_TABLE" \
          --key "{\"item_id\":{\"S\":\"flash-sale-item\"}}" \
          --query "Item.confirmed_orders.N" \
          --output text 2>/dev/null || true)"
        baseline_order_count="$(python3 - <<'PY'
import os
import boto3

table = os.environ.get("DYNAMODB_TABLE")
region = os.environ.get("AWS_REGION", "us-east-1")

ddb = boto3.client("dynamodb", region_name=region)
count = 0
kwargs = {"TableName": table, "Select": "COUNT"}
while True:
    resp = ddb.scan(**kwargs)
    count += int(resp.get("Count", 0))
    lek = resp.get("LastEvaluatedKey")
    if not lek:
        break
    kwargs["ExclusiveStartKey"] = lek
print(count)
PY
)"
        [[ -z "$baseline_confirmed" || "$baseline_confirmed" == "None" ]] && baseline_confirmed=0
        baseline_confirmed="${baseline_confirmed%%$'\n'*}"
        [[ -z "$baseline_order_count" || "$baseline_order_count" == "None" ]] && baseline_order_count=0
        baseline_order_count="${baseline_order_count%%$'\n'*}"
      fi

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
        if [[ "$WAIT_FOR_WORKER_DRAIN" == "true" && -n "$SQS_QUEUE_URL" ]]; then
          echo "Waiting for worker/SQS drain before consistency check..."
          deadline=$((SECONDS + WORKER_DRAIN_TIMEOUT_SECONDS))
          while true; do
            visible="$(aws sqs get-queue-attributes \
              --queue-url "$SQS_QUEUE_URL" \
              --attribute-names ApproximateNumberOfMessages ApproximateNumberOfMessagesNotVisible \
              --query "Attributes.ApproximateNumberOfMessages" \
              --output text 2>/dev/null || echo 0)"
            not_visible="$(aws sqs get-queue-attributes \
              --queue-url "$SQS_QUEUE_URL" \
              --attribute-names ApproximateNumberOfMessages ApproximateNumberOfMessagesNotVisible \
              --query "Attributes.ApproximateNumberOfMessagesNotVisible" \
              --output text 2>/dev/null || echo 0)"
            [[ "$visible" == "None" || -z "$visible" ]] && visible=0
            [[ "$not_visible" == "None" || -z "$not_visible" ]] && not_visible=0
            if [[ "$visible" -eq 0 && "$not_visible" -eq 0 ]]; then
              break
            fi
            if [[ "$SECONDS" -ge "$deadline" ]]; then
              echo "WARNING: Timed out waiting for SQS drain (visible=$visible not_visible=$not_visible)"
              break
            fi
            sleep 5
          done
        fi

        echo "Running consistency check for $run_tag..."
        set +e
        (
          cd "$ROOT_DIR"
          BASELINE_CONFIRMED="$baseline_confirmed" \
          BASELINE_ORDER_COUNT="$baseline_order_count" \
          INVENTORY_COUNT="$run_inventory_count" \
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
