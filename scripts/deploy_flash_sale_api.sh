#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE_DIR="$ROOT_DIR/services/flash-sale-api"

AWS_REGION="${AWS_REGION:-us-east-1}"
PROJECT="${PROJECT:-flash-sale}"
ECR_REPO_NAME="${ECR_REPO_NAME:-${PROJECT}-api}"
ECS_CLUSTER="${ECS_CLUSTER:-${PROJECT}-cluster}"
ECS_SERVICE="${ECS_SERVICE:-${PROJECT}-api}"
IMAGE_TAG="${1:-$(date +%Y%m%d-%H%M%S)}"

if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI is required" >&2
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 1
fi

AWS_ACCOUNT_ID="$(aws sts get-caller-identity --query Account --output text)"
ECR_REGISTRY="${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"
ECR_REPO_URL="${ECR_REGISTRY}/${ECR_REPO_NAME}"

echo "==> Logging into ECR ${ECR_REGISTRY}"
aws ecr get-login-password --region "$AWS_REGION" | docker login --username AWS --password-stdin "$ECR_REGISTRY"

echo "==> Building flash-sale-api image (${IMAGE_TAG})"
docker build --platform linux/amd64 -t "${ECR_REPO_NAME}:${IMAGE_TAG}" "$SERVICE_DIR"

echo "==> Tagging image"
docker tag "${ECR_REPO_NAME}:${IMAGE_TAG}" "${ECR_REPO_URL}:${IMAGE_TAG}"
docker tag "${ECR_REPO_NAME}:${IMAGE_TAG}" "${ECR_REPO_URL}:latest"

echo "==> Pushing image tags to ECR"
docker push "${ECR_REPO_URL}:${IMAGE_TAG}"
docker push "${ECR_REPO_URL}:latest"

echo "==> Forcing ECS deployment"
aws ecs update-service \
  --region "$AWS_REGION" \
  --cluster "$ECS_CLUSTER" \
  --service "$ECS_SERVICE" \
  --force-new-deployment >/dev/null

echo "Deployment triggered successfully."
echo "Image: ${ECR_REPO_URL}:${IMAGE_TAG}"
echo "Service: ${ECS_CLUSTER}/${ECS_SERVICE}"
