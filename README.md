# Distributed Flash Sale System
**CS6650 Distributed Systems | Northeastern University**
**Team:** Shail Shah · Vikas Neriyanuru · Darshan Ravindra Konnur

---

## Problem

Flash sales — limited-inventory product drops at high concurrency — are one of the
hardest distributed systems problems. When thousands of users simultaneously attempt
to buy the last 100 items, how do you guarantee exactly 100 orders are accepted with
zero oversell, across multiple service replicas, without sacrificing throughput?

---

## Architecture

```
Internet → ALB → Waiting Room → Flash Sale API → SQS → Order Worker → DynamoDB
```

Three Go microservices on AWS ECS Fargate, provisioned via Terraform:

| Service | Owner | Status | Description |
|---------|-------|--------|-------------|
| [Flash Sale API](services/flash-sale-api/) | Vikas | ✅ Done | Atomic inventory, SQS publish |
| [Order Worker](services/order-worker/) | Darshan | ✅ Done | SQS consumer, DynamoDB writes |
| [Waiting Room](services/waiting-room/) | Shail | 🔄 In Progress | Traffic shaping, token issuance |

---

## Infrastructure

Terraform modules in [terraform/](terraform/):
- `network` — VPC, subnets, security groups
- `alb` — Application Load Balancer
- `ecs` — ECS Fargate cluster and task definitions
- `redis` — ElastiCache for inventory counter
- `sqs` — Order queue + Dead Letter Queue
- `dynamodb` — Orders table + Inventory audit table
- `ecr` — Container registries
- `logging` — CloudWatch log groups

---

## Experiments

| # | Name | Variable | Metric |
|---|------|----------|--------|
| 1 | Throughput & Latency | Concurrent users (10/50/100/500) | req/s, p99 latency |
| 2 | Inventory Strategy | atomic_decr vs optimistic vs lua_script | oversell rate, throughput |
| 3 | Worker Concurrency | NUM_WORKERS (20/40/80) | SQS processing throughput |

---

## Preliminary Results (Milestone 1)

- **Concurrency test:** 200 concurrent requests → exactly 100 accepted, 0 oversell ✅
- **POST /purchase throughput:** ~417 req/s (2.4ms avg, Apple M4 local)
- **GET /inventory throughput:** ~923 req/s (1.1ms avg, Apple M4 local)

Full results: [tests/flash-sale-api/](tests/flash-sale-api/)

---

## Quick Start (Local)

```bash
# Start Redis
docker run -d -p 6379:6379 redis:7-alpine

# Run Flash Sale API
cd services/flash-sale-api
export REDIS_ENDPOINT=localhost REDIS_PORT=6379 INVENTORY_COUNT=100
export INVENTORY_STRATEGY=atomic_decr   # atomic_decr | optimistic | lua_script
export SQS_QUEUE_URL=<your-queue-url> AWS_REGION=us-east-1 APP_PORT=8080
go run .

# Run Order Worker
cd ../order-worker
export AWS_REGION=us-east-1
export SQS_QUEUE_URL=<your-queue-url>
export DYNAMODB_TABLE=<orders-table>
export INVENTORY_TABLE=<inventory-table>
export NUM_WORKERS=10
go run .
```

The order worker:
- long-polls SQS in batches of up to 10 messages
- writes each order to DynamoDB with idempotent conditional insert on `order_id`
- updates the inventory audit table in the same DynamoDB transaction
- deletes the SQS message only after the transaction succeeds

## Deploy Order Worker

Build, push, and roll out the worker on ECS:

```bash
export AWS_REGION=us-east-1
export PROJECT=flash-sale
./scripts/deploy_order_worker.sh
```

Optional overrides:
- `ECR_REPO_NAME` to override the default `${PROJECT}-order-worker`
- `ECS_CLUSTER` to override the default `${PROJECT}-cluster`
- `ECS_SERVICE` to override the default `${PROJECT}-order-worker`
- pass a custom image tag as the first script argument

## AWS Order Flow Smoke Test

Run the full AWS-backed smoke test for:
`purchase -> SQS -> order-worker -> DynamoDB`

```bash
export AWS_REGION=us-east-1
./scripts/run_order_flow_smoke_test.sh
```

The script resolves these from Terraform outputs when not explicitly provided:
- `API_BASE_URL`
- `DYNAMODB_TABLE`
- `INVENTORY_TABLE`
- `SQS_QUEUE_URL`

Useful overrides:
- `RESET_BEFORE_TEST=false` to avoid resetting inventory before the run
- `ITEM_ID` to target a different inventory item key
- `ORDER_FLOW_TIMEOUT_SECONDS` to increase the end-to-end wait window
- `ORDER_FLOW_POLL_INTERVAL_MS` to tune DynamoDB polling cadence

The smoke test itself lives in `tests/order-flow/` and will skip if the required live AWS environment variables are not set.

## Experiment 2 Load Harness (Locust)

Run a headless Locust sweep for inventory strategy experiments:

```bash
export HOST=http://<alb-dns-or-localhost:8080>
export USERS=400
export SPAWN_RATE=80
export DURATION=90s
export PURCHASE_QUANTITY=1
export INVENTORY_STRATEGY=atomic_decr

# Optional: scale flash-sale-api task count before run
export API_DESIRED_COUNT=3

./scripts/run_experiment2_locust.sh
```

Outputs are written under `tests/results/exp2/`.

---

## Repo Structure

```
├── docs/               Architecture and design docs
├── terraform/          AWS infrastructure (Terraform)
├── services/           Go microservices
│   ├── flash-sale-api/ Atomic inventory API
│   ├── order-worker/   SQS → DynamoDB worker
│   └── waiting-room/   Traffic shaping service
├── tests/              Test suites and results
│   ├── flash-sale-api/ Smoke, concurrency, benchmark tests
│   ├── locust/         Locust workload for Experiment 2
│   ├── order-flow/     AWS-backed end-to-end smoke test
│   ├── order-worker/   Unit tests for worker config and order parsing
│   └── results/        Experiment results (populated during AWS runs)
└── scripts/            Utility scripts (smoke test, deploy, locust runner)
```
