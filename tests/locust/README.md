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

## Run Locust directly

```bash
cd tests/locust
PURCHASE_QUANTITY=1 locust --locustfile locustfile.py --host http://localhost:8080
```
