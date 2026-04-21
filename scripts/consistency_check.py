#!/usr/bin/env python3
"""
Post-experiment consistency check.

Verifies:
    Redis inventory counter + DynamoDB confirmed_orders == INVENTORY_COUNT

Usage:
    python consistency_check.py

Required environment variables:
    AWS_REGION
    REDIS_ENDPOINT
    DYNAMODB_TABLE      (orders table)
    INVENTORY_TABLE     (inventory audit table)

Optional:
    REDIS_PORT          (default: 6379)
    INVENTORY_COUNT     (default: 100)
    ITEM_ID             (default: flash-sale-item)

Exit codes:
    0 — consistent (invariant holds)
    1 — inconsistent (oversell or data loss detected)
    2 — configuration or connectivity error
"""

import os
import sys
import json
import urllib.request
from datetime import datetime, timezone

# Dependencies
try:
    import boto3
    import redis
except ImportError:
    print("ERROR: Missing dependencies. Run: pip install boto3 redis")
    sys.exit(2)

# Config
def load_config():
    cfg = {
        "aws_region":       os.environ.get("AWS_REGION", ""),
        "redis_endpoint":   os.environ.get("REDIS_ENDPOINT", ""),
        "redis_port":       int(os.environ.get("REDIS_PORT", "6379")),
        "api_base_url":     os.environ.get("API_BASE_URL", ""),
        "dynamodb_table":   os.environ.get("DYNAMODB_TABLE", ""),
        "inventory_table":  os.environ.get("INVENTORY_TABLE", ""),
        "inventory_count":  int(os.environ.get("INVENTORY_COUNT", "100")),
        "baseline_confirmed": int(os.environ.get("BASELINE_CONFIRMED", "0")),
        "baseline_order_count": int(os.environ.get("BASELINE_ORDER_COUNT", "0")),
        "item_id":          os.environ.get("ITEM_ID", "flash-sale-item"),
    }

    missing = [k for k in ("aws_region", "dynamodb_table", "inventory_table")
               if not cfg[k]]
    if not cfg["redis_endpoint"] and not cfg["api_base_url"]:
        missing.append("redis_endpoint or api_base_url")
    if missing:
        print(f"ERROR: Missing required environment variables: {', '.join(missing).upper()}")
        sys.exit(2)

    return cfg

# Redis
def get_redis_inventory(cfg):
    try:
        rdb = redis.Redis(
            host=cfg["redis_endpoint"],
            port=cfg["redis_port"],
            decode_responses=True,
            socket_connect_timeout=5,
        )
        rdb.ping()
        key = f"inventory:{cfg['item_id']}"
        val = rdb.get(key)
        if val is None:
            print(f"WARNING: Redis key '{key}' not found — treating as 0")
            return 0
        return max(0, int(val))  # clamp to 0; counter can be briefly negative during races
    except redis.ConnectionError as e:
        print(f"ERROR: Cannot connect to Redis at {cfg['redis_endpoint']}:{cfg['redis_port']}: {e}")
        sys.exit(2)

def get_api_inventory(cfg):
    if not cfg["api_base_url"]:
        print("ERROR: API_BASE_URL not provided for API inventory check")
        sys.exit(2)
    try:
        url = f"{cfg['api_base_url'].rstrip('/')}/inventory"
        with urllib.request.urlopen(url, timeout=10) as resp:
            payload = json.loads(resp.read().decode("utf-8"))
            return max(0, int(payload.get("remaining", 0)))
    except Exception as e:
        print(f"ERROR: API inventory query failed: {e}")
        sys.exit(2)

# DynamoDB
def get_dynamodb_confirmed_orders(cfg):
    try:
        ddb = boto3.client("dynamodb", region_name=cfg["aws_region"])
        response = ddb.get_item(
            TableName=cfg["inventory_table"],
            Key={"item_id": {"S": cfg["item_id"]}},
        )
        item = response.get("Item", {})
        if not item:
            print(f"WARNING: No inventory record for item_id='{cfg['item_id']}' — treating as 0")
            return 0
        return int(item.get("confirmed_orders", {}).get("N", "0"))
    except Exception as e:
        print(f"ERROR: DynamoDB query failed: {e}")
        sys.exit(2)

def get_dynamodb_order_count(cfg):
    try:
        ddb = boto3.client("dynamodb", region_name=cfg["aws_region"])
        count = 0
        kwargs = {
            "TableName": cfg["dynamodb_table"],
            "Select": "COUNT",
        }
        while True:
            response = ddb.scan(**kwargs)
            count += response["Count"]
            if "LastEvaluatedKey" not in response:
                break
            kwargs["ExclusiveStartKey"] = response["LastEvaluatedKey"]
        return count
    except Exception as e:
        print(f"WARNING: Could not count order records: {e}")
        return None


# Report
def run_check(cfg):
    print("-" * 60)
    print("  Flash Sale - Post-Experiment Consistency Check")
    print("-" * 60)
    print(f"  Timestamp:        {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M:%S UTC')}")
    print(f"  Item ID:          {cfg['item_id']}")
    print(f"  Inventory count:  {cfg['inventory_count']}")
    if cfg["redis_endpoint"]:
        print(f"  Redis:            {cfg['redis_endpoint']}:{cfg['redis_port']}")
    if cfg["api_base_url"]:
        print(f"  API base URL:     {cfg['api_base_url']}")
    print(f"  DynamoDB tables:  {cfg['dynamodb_table']}, {cfg['inventory_table']}")
    print()

    if cfg["redis_endpoint"]:
        redis_remaining = get_redis_inventory(cfg)
        inventory_source = "redis"
    else:
        redis_remaining = get_api_inventory(cfg)
        inventory_source = "api_inventory"

    dynamo_confirmed_abs   = get_dynamodb_confirmed_orders(cfg)
    dynamo_order_count_abs = get_dynamodb_order_count(cfg)
    inventory_count    = cfg["inventory_count"]

    dynamo_confirmed = max(0, dynamo_confirmed_abs - cfg["baseline_confirmed"])
    dynamo_order_count = None
    if dynamo_order_count_abs is not None:
        dynamo_order_count = max(0, dynamo_order_count_abs - cfg["baseline_order_count"])

    total = redis_remaining + dynamo_confirmed

    print("  Results:")
    print(f"    Redis remaining inventory:     {redis_remaining}")
    print(f"    DynamoDB confirmed_orders:     {dynamo_confirmed} (abs={dynamo_confirmed_abs}, baseline={cfg['baseline_confirmed']})")
    print(f"    -------------------------------------")
    print(f"    Sum (should == {inventory_count}):          {total}")
    if dynamo_order_count is not None:
        print(f"    DynamoDB order record count:   {dynamo_order_count} (abs={dynamo_order_count_abs}, baseline={cfg['baseline_order_count']})")
    print()

    # Invariant checks
    passed = True
    issues = []

    # Primary invariant: remaining + confirmed == INVENTORY_COUNT
    if total != inventory_count:
        delta = total - inventory_count
        if delta > 0:
            issues.append(
                f"OVERSELL: sum={total} exceeds INVENTORY_COUNT={inventory_count} by {delta}"
            )
        else:
            issues.append(
                f"DATA LOSS: sum={total} is {abs(delta)} less than INVENTORY_COUNT={inventory_count}"
            )
        passed = False

    # Secondary: DynamoDB order records should match confirmed_orders counter
    if dynamo_order_count is not None and dynamo_order_count != dynamo_confirmed:
        issues.append(
            f"AUDIT MISMATCH: orders table has {dynamo_order_count} records "
            f"but confirmed_orders counter = {dynamo_confirmed}"
        )
        passed = False

    # No negative remaining (should never happen — INCR rollback prevents it)
    if redis_remaining < 0:
        issues.append(f"NEGATIVE INVENTORY: Redis counter = {redis_remaining}")
        passed = False

    if passed:
        print("CONSISTENT — invariant holds")
        print(f"     {redis_remaining} remaining + {dynamo_confirmed} confirmed = {inventory_count}")
    else:
        print("INCONSISTENT — invariant violated:")
        for issue in issues:
            print(f" {issue}")

    result = {
        "timestamp":           datetime.now(timezone.utc).isoformat(),
        "item_id":             cfg["item_id"],
        "inventory_source":    inventory_source,
        "inventory_count":     inventory_count,
        "baseline_confirmed":  cfg["baseline_confirmed"],
        "baseline_order_count": cfg["baseline_order_count"],
        "redis_remaining":     redis_remaining,
        "dynamo_confirmed":    dynamo_confirmed,
        "dynamo_confirmed_abs": dynamo_confirmed_abs,
        "dynamo_order_count":  dynamo_order_count,
        "dynamo_order_count_abs": dynamo_order_count_abs,
        "sum":                 total,
        "consistent":          passed,
        "issues":              issues,
    }

    result_path = "consistency_result.json"
    with open(result_path, "w") as f:
        json.dump(result, f, indent=2)
    print(f"\n  Full result saved to: {result_path}")
    print("-" * 60)

    return passed


# Entry point
if __name__ == "__main__":
    cfg = load_config()
    consistent = run_check(cfg)
    sys.exit(0 if consistent else 1)
