# Distributed Flash Sale System

**CS6650 Distributed Systems — Northeastern University**  
**Team:** Shail Shah · Vikas Neriyanuru · Darshan Ravindra Konnur

---

## Why we built it

Flash sales put three hard distributed-systems problems on one critical path:

1. **Fair admission** when concurrent arrivals dwarf what the system can serve.
2. **Correct inventory accounting** when every request is trying to buy the last unit.
3. **Back-pressure control** so accepting a purchase does not outrun the layer that must persist it.

Our HW7 experiment, a naive "async everywhere" design, made the third problem obvious: 20 users over 60 seconds produced an SQS backlog of about 1,196 messages because there was nothing gating entry into the async path. This final project is the missing layer that design did not have: a stateful, ordered, rate-controlled waiting room between raw traffic and the purchase flow.

---

## Architecture

```text
Internet -> ALB -> Waiting Room -> Flash Sale API -> SQS -> Order Worker -> DynamoDB
                 (Redis ZSET)      (atomic DECR)         (idempotent writes)
```

Three Go services on AWS ECS Fargate, provisioned via Terraform:

| Service | Responsibility |
|---------|----------------|
| [`waiting-room`](services/waiting-room/) | Queue positions via Redis sorted-set; atomic `INCR + ZADD + ZRANK` Lua for fairness; token-bucket admission controller. |
| [`flash-sale-api`](services/flash-sale-api/) | Inventory reservation in Redis; optional admission-token gate; publishes confirmed purchases to SQS. |
| [`order-worker`](services/order-worker/) | SQS consumer; idempotent DynamoDB writes; inventory-audit updates. |

Auto-scaling on ECS task count is driven by CloudWatch CPU and SQS depth (Terraform in [`terraform/`](terraform/)).

---

## Experiments

| # | Question | Variables | Key metric |
|---|----------|-----------|------------|
| 1 | Does the queue preserve arrival order under concurrent joins? | `timestamp` vs `timestamp + INCR` scoring; 100 / 500 / 1k users | Position-collision rate |
| 2 | Which inventory strategy prevents oversell without wasting stock? | `atomic_decr` vs `WATCH/MULTI/EXEC` vs `lua_script`; 200 / 500 / 1k buyers | Accepted vs. oversell |
| 3 | How should admission rate be tuned against worker capacity? | Admission rate 1 / 5 / 25 / 100 per second; 500 users, 100 units | Time-to-sellout, wasted admissions, peak SQS depth |

Full write-up (5 pages, PDF): [`docs/report/experiments_report.pdf`](docs/report/experiments_report.pdf)

Charts: [`tests/results/charts/`](tests/results/charts/)  
Raw CSVs and per-run summaries: [`tests/results/`](tests/results/)

### Headline findings

- **Strategy B (`INCR` tiebreaker) eliminates queue collisions.** Timestamp-only scoring stabilizes around 70% collisions at 1k users; the fixed `INCR + ZADD + ZRANK` path hits 0.
- **Optimistic locking is correct but catastrophically wasteful.** Under contention it accepts only 17 / 12 / 28 of the available 100 units across the 200 / 500 / 1k buyer sweeps, with zero oversell.
- **Admission control refutes the HW7 backlog prediction.** With a real gate in place, peak SQS depth stayed at 0 across every rate we tested.
- **10k-user stress exposed a real library-defaults failure.** The go-redis default pool saturated under burst traffic; `PoolSize=2000` and `PoolTimeout=10s` fixed the user-visible errors.

---

## Quick start (fully local)

The full stack runs against Docker Redis and LocalStack (SQS + DynamoDB). No real AWS resources are required.

```bash
# 1. Start Redis and LocalStack
docker compose -f docker-compose.local.yml up -d

# 2. Create the SQS queue and DynamoDB tables in LocalStack
bash scripts/bootstrap_local.sh

# 3. Load local env vars
source .env.local

# 4. Start all three services in separate terminals
(cd services/waiting-room && QUEUE_STRATEGY=timestamp_incr ADMISSION_RATE=25 go run .)
(cd services/flash-sale-api && APP_PORT=8081 INVENTORY_STRATEGY=atomic_decr go run .)
(cd services/order-worker && go run .)

# 5. Smoke test the API
bash scripts/smoke_test.sh http://localhost:8081

# 6. End-to-end smoke: purchase -> SQS -> order-worker -> DynamoDB
(cd tests/order-flow && go test -count=1 -v)
```

### Reproducing the experiments locally

```bash
source .env.local

# Experiment 1 — fairness sweep
(cd tests/exp1-fairness && USERS=1000 STRATEGY_LABEL=timestamp_incr go run .)

# Experiment 2 — inventory correctness sweep
# Start flash-sale-api with the strategy you want to test:
(cd services/flash-sale-api && APP_PORT=8081 INVENTORY_STRATEGY=optimistic OPTIMISTIC_RETRY_LIMIT=1 go run .)
(cd tests/exp2-inventory && BUYERS=500 INVENTORY=100 STRATEGY_LABEL=optimistic go run .)

# Experiment 3 — admission-rate tuning
(cd services/flash-sale-api && APP_PORT=8081 INVENTORY_STRATEGY=atomic_decr REQUIRE_ADMISSION_TOKEN=true go run .)
(cd tests/exp3-admission && ADMISSION_RATE_LABEL=rate25 USERS=500 INVENTORY=100 RUN_DURATION_SECONDS=60 go run .)
```

Notes:

- `INVENTORY_STRATEGY` accepts `atomic_decr`, `optimistic`, or `lua_script`.
- To reproduce the report's optimistic-locking results, set `OPTIMISTIC_RETRY_LIMIT=1` so `WATCH` conflicts count as rejected attempts instead of being retried.
- CSVs land in `tests/results/`; regenerate charts with `python3 tests/results/charts/generate_charts.py`.

### Optional Locust harness for Experiment 2

Darshan also added a Locust-based Experiment 2 driver for AWS or ALB-backed runs:

```bash
export HOST=http://<alb-dns-or-localhost:8080>
export USERS=400
export SPAWN_RATE=80
export DURATION=90s
export PURCHASE_QUANTITY=1
export INVENTORY_STRATEGY=atomic_decr

./scripts/run_experiment2_locust.sh
```

Outputs are written under `tests/results/exp2/`.

For the full Experiment 2 matrix (`3 strategies x 3 user levels x 3 replica counts`)
with post-run consistency checks:

```bash
export HOST=http://<alb-dns-or-localhost:8080>
export STRATEGIES="atomic_decr optimistic lua_script"
export USERS_LEVELS="200 500 1000"
export REPLICA_COUNTS="1 2 4"
export SCALE_API=true
export RUN_CONSISTENCY_CHECK=true

./scripts/run_exp2_matrix.sh
```

---

## AWS deploy

Each service has its own deploy script (`scripts/deploy_*.sh`) that builds the container, pushes to ECR, and forces a new ECS deployment:

```bash
export AWS_REGION=us-east-1
export PROJECT=flash-sale

./scripts/deploy_waiting_room.sh
./scripts/deploy_flash_sale_api.sh
./scripts/deploy_order_worker.sh
```

Smoke the deployed stack end-to-end (resolves Terraform outputs automatically):

```bash
./scripts/run_order_flow_smoke_test.sh
```

---

## Repo layout

```text
├── docs/report/        Final experiments report (Markdown + PDF)
├── terraform/          AWS infrastructure (8 modules)
├── services/
│   ├── waiting-room/   Redis ZSET queue + admission ticker
│   ├── flash-sale-api/ Inventory reservation + admission-token gate + SQS publish
│   └── order-worker/   SQS consumer -> DynamoDB writes
├── tests/
│   ├── exp1-fairness/  Concurrent-join harness
│   ├── exp2-inventory/ Concurrent-purchase harness
│   ├── exp3-admission/ Full-chain admission-rate sweep
│   ├── flash-sale-api/ API tests
│   ├── locust/         Optional Locust workload for Experiment 2
│   ├── order-flow/     End-to-end smoke test
│   ├── order-worker/   Worker unit tests
│   ├── waiting-room/   Waiting-room smoke test
│   └── results/        CSVs, summaries, generated charts
├── scripts/            Local bootstrap, smoke tests, deploy helpers
└── docker-compose.local.yml
```
