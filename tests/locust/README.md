# Locust Harness (Experiment 2)

This folder contains a reusable Locust workload for flash-sale API experiments.

## Setup

```bash
cd tests/locust
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

## Run via script (recommended)

From repo root:

```bash
export HOST=http://<alb-dns-or-localhost:8080>
export USERS=400
export SPAWN_RATE=80
export DURATION=90s
export PURCHASE_QUANTITY=1
export INVENTORY_STRATEGY=atomic_decr

# Optional ECS scaling sweep input
export API_DESIRED_COUNT=3

./scripts/run_experiment2_locust.sh
```

Results are written to `tests/results/exp2/` as CSV + log files.

## Full Exp 2 matrix (strategies x users x replicas)

From repo root:

```bash
export HOST=http://<alb-dns-or-localhost:8080>
export STRATEGIES="atomic_decr optimistic lua_script"
export USERS_LEVELS="200 500 1000"
export REPLICA_COUNTS="1 2 4"
export SCALE_API=true
export RUN_CONSISTENCY_CHECK=true

./scripts/run_exp2_matrix.sh
```

Per-run consistency artifacts are saved under `tests/results/exp2/consistency/`.

## Run Locust directly

```bash
cd tests/locust
PURCHASE_QUANTITY=1 locust --locustfile locustfile.py --host http://localhost:8080
```
