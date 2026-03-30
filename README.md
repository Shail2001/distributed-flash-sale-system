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
| [Order Worker](services/order-worker/) | Darshan | 🔄 In Progress | SQS consumer, DynamoDB writes |
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
export SQS_QUEUE_URL=<your-queue-url> AWS_REGION=us-east-1 APP_PORT=8080
go run .
```

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
│   └── results/        Experiment results (populated during AWS runs)
└── scripts/            Utility scripts (smoke test, load test)
```
