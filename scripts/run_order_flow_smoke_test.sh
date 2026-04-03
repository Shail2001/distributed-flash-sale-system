#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$ROOT_DIR/tests/order-flow"
TERRAFORM_DIR="$ROOT_DIR/terraform"

AWS_REGION="${AWS_REGION:-us-east-1}"
ITEM_ID="${ITEM_ID:-flash-sale-item}"
RESET_BEFORE_TEST="${RESET_BEFORE_TEST:-true}"
ORDER_FLOW_TIMEOUT_SECONDS="${ORDER_FLOW_TIMEOUT_SECONDS:-90}"
ORDER_FLOW_POLL_INTERVAL_MS="${ORDER_FLOW_POLL_INTERVAL_MS:-2000}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "$1 is required" >&2
    exit 1
  fi
}

resolve_tf_output() {
  local output_name=$1
  terraform -chdir="$TERRAFORM_DIR" output -raw "$output_name"
}

require_command aws
require_command go

echo "==> Verifying AWS credentials"
aws sts get-caller-identity >/dev/null

if [[ -z "${API_BASE_URL:-}" || -z "${DYNAMODB_TABLE:-}" || -z "${INVENTORY_TABLE:-}" || -z "${SQS_QUEUE_URL:-}" ]]; then
  require_command terraform
fi

if [[ -z "${API_BASE_URL:-}" ]]; then
  echo "==> Resolving ALB DNS from Terraform outputs"
  ALB_DNS_NAME="${ALB_DNS_NAME:-$(resolve_tf_output alb_dns_name)}"
  API_BASE_URL="http://${ALB_DNS_NAME}"
fi

if [[ -z "${DYNAMODB_TABLE:-}" ]]; then
  echo "==> Resolving orders table from Terraform outputs"
  DYNAMODB_TABLE="$(resolve_tf_output orders_table_name)"
fi

if [[ -z "${INVENTORY_TABLE:-}" ]]; then
  echo "==> Resolving inventory table from Terraform outputs"
  INVENTORY_TABLE="$(resolve_tf_output inventory_table_name)"
fi

if [[ -z "${SQS_QUEUE_URL:-}" ]]; then
  echo "==> Resolving orders queue URL from Terraform outputs"
  SQS_QUEUE_URL="$(resolve_tf_output orders_queue_url)"
fi

echo "==> Running AWS order-flow smoke test"
echo "API_BASE_URL=${API_BASE_URL}"
echo "DYNAMODB_TABLE=${DYNAMODB_TABLE}"
echo "INVENTORY_TABLE=${INVENTORY_TABLE}"
echo "SQS_QUEUE_URL=${SQS_QUEUE_URL}"

export AWS_REGION API_BASE_URL DYNAMODB_TABLE INVENTORY_TABLE SQS_QUEUE_URL ITEM_ID RESET_BEFORE_TEST
export ORDER_FLOW_TIMEOUT_SECONDS ORDER_FLOW_POLL_INTERVAL_MS

(
  cd "$TEST_DIR"
  go test -count=1 -run TestAWSOrderFlowSmoke -v ./...
)
