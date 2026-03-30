# CS6650 Distributed Systems — HW9 Flash Sale Platform
## Milestone 1 Report

**Team:** Shail Shah, Vikas Neriyanuru, Darshan Ravindra Konnur
**Date:** March 2026

---

## 1. Problem Statement

Flash sales — limited-inventory product drops at high concurrency — represent one of
the hardest classes of distributed systems problems. The core challenge: when 10,000
users simultaneously attempt to buy the last 100 items, how do you guarantee exactly
100 orders are accepted and zero oversell occurs, across multiple service replicas,
without sacrificing throughput?

Traditional approaches (database row locks, read-check-write) serialize requests and
become bottlenecks. Our platform solves this using Redis atomic operations and a
decoupled order processing pipeline.

---

## 2. Architecture

```
Internet → ALB → Waiting Room → Flash Sale API → SQS → Order Worker → DynamoDB
```

Three Go microservices, all running on ECS Fargate, provisioned via Terraform:

**Flash Sale API (this service — Port 8080)**
- Handles purchase requests at high concurrency
- Uses Redis ElastiCache atomic DECR for inventory management
- Publishes accepted orders to SQS
- Prevents oversell without locks

**Order Worker (background service)**
- Polls SQS with NUM_WORKERS goroutines (Experiment 3 variable)
- Persists confirmed orders to DynamoDB
- Implements at-least-once delivery via visibility timeout

**Waiting Room (traffic shaping)**
- Issues timed tokens to rate-limit traffic into the Flash Sale API
- Prevents thundering herd at sale start

---

## 3. Team Breakdown

| Member | Responsibility |
|--------|---------------|
| Shail Shah | Terraform infrastructure, AWS provisioning, Waiting Room service |
| Vikas Neriyanuru | Flash Sale API (this service), atomic DECR design, testing |
| Darshan Ravindra Konnur | Order Worker service, DynamoDB writes, goroutine concurrency |

---

## 4. Key Design Decision — Why Atomic DECR?

Redis DECR is atomic at the server level (Redis is single-threaded for commands).
This means across 10 ECS replicas all hitting the same ElastiCache instance,
each DECR is serialized — making it impossible for two replicas to both see
"remaining > 0" and both decrement to the same value.

**The pattern:**
1. DECR inventory counter
2. If result >= 0 → inventory available, proceed
3. If result < 0 → sold out, INCR back to 0, return 409

No locks. No transactions. No read-check-write. Just atomic DECR.

---

## 5. Experiments

**Experiment 1 — Throughput & Latency Under Load**
Measure requests/sec and p50/p95/p99 latency for POST /purchase at increasing
concurrent user counts (10, 50, 100, 500 concurrent users).

**Experiment 2 — Inventory Strategy Comparison**
Compare three strategies under identical load:
- atomic_decr (Redis DECR — this implementation)
- optimistic_locking (read-check-write with retry)
- lua_script (Redis Lua for multi-key atomicity)
Metric: oversell rate, throughput, p99 latency

**Experiment 3 — Order Worker Goroutine Sweep**
Sweep NUM_WORKERS (20, 40, 80 goroutines) and measure:
- SQS message processing throughput
- DynamoDB write latency
- End-to-end order confirmation time

---

## 6. Preliminary Results

### 6.1 Smoke Test Results

```
=== FLASH SALE API SMOKE TEST RESULTS ===
Date: Mon Mar 30 09:27:54 EDT 2026

--- Step 2.1: Reset inventory ---
{"status":"reset","remaining":100}

--- Step 2.2: Check inventory after reset ---
{"item_id":"flash-sale-item","remaining":100,"sold_out":false}

--- Step 2.3: Health check ---
{"status":"healthy","remaining":100}

--- Step 2.4: Single purchase ---
{"order_id":"b105727c-0c47-4095-97d4-bab153d01c59","customer_id":"test-customer-1","status":"accepted","remaining_inventory":99}
HTTP_STATUS:201

--- Step 2.5: Verify inventory decremented ---
{"item_id":"flash-sale-item","remaining":99,"sold_out":false}

--- Step 2.6: Make 9 more purchases (customers 2–10) ---
All 9 accepted. Inventory counts down: 98→97→96→95→94→93→92→91→90

--- Step 2.7: Check inventory = 90 ---
{"item_id":"flash-sale-item","remaining":90,"sold_out":false}

--- Step 2.8: Invalid request (missing customer_id) ---
{"error":"invalid request","message":"Key: 'purchaseRequest.CustomerID' Error:Field validation for 'CustomerID' failed on the 'required' tag"}
HTTP_STATUS:400

--- Step 2.9: Reset + drain all 100 items ---
{"status":"reset","remaining":100}
[100 sequential purchases accepted]

--- Step 2.10: Purchase attempt when sold out ---
{"error":"SOLD_OUT","message":"No inventory remaining"}
HTTP_STATUS:409

--- Step 2.11: Inventory shows sold_out: true ---
{"item_id":"flash-sale-item","remaining":0,"sold_out":true}
```

All endpoints behaved exactly as specified. Invalid requests return 400, sold-out returns 409, successful purchases return 201 with order_id and remaining inventory.

### 6.2 Concurrency Test — Oversell Prevention

- Total concurrent requests: 200
- Inventory count: 100
- Accepted (201): **100**
- Rejected (409): **100**
- Errors: 0
- **Oversell detected: NO ✓**
- **Result: Atomic DECR correctly serialized all requests — exactly 100 accepted**

200 goroutines fired simultaneously against a single API server. Despite true parallel execution, the Redis DECR atomicity ensured not a single extra unit was sold.

### 6.3 Benchmark Results

Environment: Apple M4, macOS (darwin/arm64), local Redis + LocalStack SQS

```
goos: darwin
goarch: arm64
pkg: tests
cpu: Apple M4
BenchmarkPurchase-10     	    9372	   2399718 ns/op	   19627 B/op	     144 allocs/op
BenchmarkInventory-10    	   15792	   1083262 ns/op	   17111 B/op	     125 allocs/op
PASS
ok  	tests	48.288s
```

**Derived throughput:**
- POST /purchase: ~417 req/s (2.4 ms/op average)
- GET /inventory: ~923 req/s (1.1 ms/op average)

Note: These are single-client sequential benchmarks on localhost. Production throughput on ECS with connection pooling and parallel clients will be significantly higher.

---

## 7. Project Plan & Timeline

| Week | Task | Owner | Status |
|------|------|-------|--------|
| Week 1 | Flash Sale API implementation | Vikas | ✅ Done |
| Week 1 | Terraform infrastructure | Shail | 🔄 In Progress |
| Week 1 | Order Worker implementation | Darshan | 🔄 In Progress |
| Week 2 | Deploy all 3 services to ECS | All | ⏳ Pending |
| Week 2 | Run Experiment 1 (throughput) | All | ⏳ Pending |
| Week 3 | Run Experiment 2 (strategy comparison) | Vikas | ⏳ Pending |
| Week 3 | Run Experiment 3 (goroutine sweep) | Darshan | ⏳ Pending |
| Week 4 | Analyze results, write final report | All | ⏳ Pending |

---

## 8. Role of AI

Claude (Anthropic) was used to:
- Generate the initial Flash Sale API boilerplate from the API blueprint spec
- Write test files (smoke tests, concurrency tests, benchmarks)
- Generate this report structure

All generated code was reviewed, understood, and validated by team members before
committing. The atomic DECR design decision and experiment design were made by the team.

**Cost/benefit:**
- Benefit: Significantly faster initial scaffolding (~2 hours → ~20 minutes)
- Benefit: Test coverage that would have taken additional hours to write manually
- Cost: Generated code requires careful review — especially the rollback INCR logic
- Cost: AI doesn't know our specific AWS account setup (queue URLs, table names)

---

## 9. Observability Plan

- **CloudWatch Logs**: All purchases logged with order_id, customer_id, strategy, remaining
- **CloudWatch Metrics**: Custom metrics for accepted/rejected order counts
- **Health endpoint**: GET /health for ALB target group health checks
- **Order Worker /stats**: Admin endpoint on port 8081 for goroutine and processing metrics
- **DynamoDB consistency check**: confirmed_orders + Redis counter should always = INVENTORY_COUNT

---

## 10. Repo

https://github.com/Shail2001/distributed-flash-sale-system
