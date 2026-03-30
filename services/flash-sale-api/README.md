# Flash Sale API

Go + Gin service handling atomic inventory management and order queuing for the flash sale platform.

**Stack:** Go 1.21 · Gin · Redis (ElastiCache) · SQS · ECS Fargate

---

## Local Development

### Prerequisites
- Go 1.21+
- Docker (for Redis)
- AWS credentials configured (`aws configure`)

### Start Redis locally
```bash
docker run -d -p 6379:6379 redis:7-alpine
```

### Set environment variables
```bash
export REDIS_ENDPOINT=localhost
export REDIS_PORT=6379
export SQS_QUEUE_URL=https://sqs.us-east-1.amazonaws.com/<account>/<queue-name>
export INVENTORY_COUNT=100
export AWS_REGION=us-east-1
export APP_PORT=8080
```

### Run
```bash
go run .
```

---

## Endpoints

| Method | Endpoint     | Status   | Description                          |
|--------|-------------|----------|--------------------------------------|
| GET    | /health      | 200/503  | Ping Redis, return inventory count   |
| GET    | /inventory   | 200      | Read Redis counter (no decrement)    |
| POST   | /purchase    | 201/409  | Atomic DECR + publish to SQS         |
| POST   | /reset       | 200      | Restore counter to INVENTORY_COUNT   |

---

## E2E Smoke Test

```bash
ALB=localhost:8080

# 1. Reset inventory
curl -X POST http://$ALB/reset

# 2. Check inventory
curl http://$ALB/inventory
# Expected: { "remaining": 100, "sold_out": false }

# 3. Make a purchase
curl -X POST http://$ALB/purchase \
  -H "Content-Type: application/json" \
  -d '{"customer_id": "test-1", "quantity": 1}'
# Expected: 201 with order_id

# 4. Confirm inventory decremented
curl http://$ALB/inventory
# Expected: { "remaining": 99, "sold_out": false }
```

---

## Docker Build

```bash
docker build --platform linux/amd64 -t flash-sale-api .
docker run -p 8080:8080 --env-file .env flash-sale-api
```

> **Note:** Always build with `--platform linux/amd64` for ECS Fargate compatibility.

---

## Key Design Decision — Why Atomic DECR?

Redis `DECR` is atomic at the server level — no race conditions across multiple ECS instances.
The pattern is: **DECR → check result → if < 0, INCR back and return 409**.
This prevents oversell without locks, transactions, or read-check-write patterns.
