# Architecture — Distributed Flash Sale System

## System Diagram

```
Internet
    │
    ▼
AWS ALB (Application Load Balancer)
    │
    ▼
Waiting Room Service (Go, ECS Fargate)
    │  Issues timed JWT tokens, rate-limits traffic
    │
    ▼
Flash Sale API (Go + Gin, ECS Fargate, port 8080)
    │  Atomic Redis DECR → prevents oversell
    │  Publishes accepted orders to SQS
    │
    ├──→ Redis ElastiCache
    │       inventory:flash-sale-item  (integer counter)
    │
    └──→ SQS Queue (flash-sale-orders)
              │
              ▼
        Order Worker (Go, ECS Fargate)
              │  NUM_WORKERS goroutines poll SQS
              │
              ├──→ DynamoDB (flash-sale-orders table)
              │       Confirmed orders with TTL
              │
              └──→ DynamoDB (flash-sale-inventory table)
                      confirmed_orders counter (audit)
```

## Key Design Decisions

### Why Redis DECR for inventory?
Redis commands are atomic at the server level (single-threaded command processing).
`DECR` across 10 ECS replicas hitting the same ElastiCache node is serialized —
making it impossible for two replicas to both see remaining > 0 and both succeed.

Pattern: `DECR → if result < 0 → INCR back → return 409`

### Why SQS between API and Worker?
Decouples purchase acceptance (latency-sensitive) from order persistence (throughput-sensitive).
The API can respond in < 5ms while the worker processes at its own pace.
SQS visibility timeout + DLQ provides at-least-once delivery with automatic retry.

### Why ECS Fargate?
Serverless containers — no EC2 instance management. Scales horizontally per service independently.
Experiment 3 sweeps NUM_WORKERS per task definition without redeploying infrastructure.

## Consistency Guarantee

After a completed flash sale:
```
Redis counter (remaining) + DynamoDB confirmed_orders = INVENTORY_COUNT
```
Any deviation indicates oversell (Redis < 0 briefly) or message loss (DLQ messages).
