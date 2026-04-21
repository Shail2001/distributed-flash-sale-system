import os
import time
import json
import threading
import logging
from datetime import datetime, timezone

from locust import HttpUser, task, events, between
from locust.runners import MasterRunner, WorkerRunner

# ── Config ────────────────────────────────────────────────────────────────────
POLL_INTERVAL    = int(os.environ.get("POLL_INTERVAL_MS", "500")) / 1000
ITEM_ID          = os.environ.get("ITEM_ID", "flash-sale-item")
ADMISSION_RATE   = os.environ.get("ADMISSION_RATE", "unknown")
NUM_WORKERS      = os.environ.get("NUM_WORKERS", "unknown")
USER_COUNT       = os.environ.get("USER_COUNT", "unknown")
INVENTORY_COUNT  = int(os.environ.get("INVENTORY_COUNT", "500"))
WAITING_ROOM_URL = os.environ.get("WAITING_ROOM_URL", "")
FLASH_SALE_URL   = os.environ.get("FLASH_SALE_URL", "")

# ── Shared experiment state (thread-safe) ─────────────────────────────────────
_lock                  = threading.Lock()
_sold_out              = False
_sell_out_time         = None
_experiment_start_time = None

_counters = {
    "joins":             0,
    "admitted":          0,
    "purchases_ok":      0,   # 201
    "wasted_admissions": 0,   # admitted but got 409
    "purchase_errors":   0,   # 5xx or network errors
    "poll_total":        0,
    "never_admitted":    0,   # users whose run-time ended before being admitted
}


def _inc(key, n=1):
    with _lock:
        _counters[key] += n


def _set_sell_out():
    global _sold_out, _sell_out_time
    with _lock:
        if not _sold_out:
            _sold_out = True
            _sell_out_time = time.time()
            elapsed = _sell_out_time - (_experiment_start_time or _sell_out_time)
            logging.info(
                f"[EXP3] SOLD OUT — "
                f"admission_rate={ADMISSION_RATE}/s "
                f"num_workers={NUM_WORKERS} "
                f"users={USER_COUNT} "
                f"time_to_sell_out={elapsed:.2f}s "
                f"purchases_ok={_counters['purchases_ok']} "
                f"wasted={_counters['wasted_admissions']}"
            )


def _record_start():
    global _experiment_start_time
    with _lock:
        if _experiment_start_time is None:
            _experiment_start_time = time.time()
            logging.info(
                f"[EXP3] First join received — experiment clock started "
                f"admission_rate={ADMISSION_RATE}/s "
                f"num_workers={NUM_WORKERS} "
                f"users={USER_COUNT} "
                f"inventory={INVENTORY_COUNT}"
            )


# ── User behavior ─────────────────────────────────────────────────────────────
class FlashSaleUser(HttpUser):
    """
    Each virtual user runs the full flash sale flow exactly once:
      1. POST /queue/join       — enter the waiting room
      2. GET  /queue/position   — poll until admitted (token received)
      3. POST /purchase         — attempt to buy one item
    After the flow completes the user goes idle for the rest of the run.
    """

    wait_time = between(0, 0)

    def on_start(self):
        # Unique user ID — includes object id + ms timestamp to avoid collisions
        # across 2000 concurrent users.
        self.user_id           = f"exp3-{id(self)}-{int(time.time()*1000)}"
        self.join_time         = None
        self.token             = None
        self.flow_complete     = False
        self.waiting_room_base = WAITING_ROOM_URL or self.host
        self.flash_sale_base   = FLASH_SALE_URL   or self.host

    @task
    def run_flash_sale_flow(self):
        """Runs once — user goes idle after completing the flow."""
        if self.flow_complete:
            time.sleep(1)  # idle — keep user alive until run-time ends
            return

        self._join_queue()
        self._poll_until_admitted()
        self._attempt_purchase()
        self.flow_complete = True

    # ── Step 1: Join ──────────────────────────────────────────────────────────
    def _join_queue(self):
        _record_start()
        self.join_time = time.time()

        with self.client.post(
            f"{self.waiting_room_base}/queue/join",
            json={"user_id": self.user_id},
            name="/queue/join",
            catch_response=True,
        ) as resp:
            if resp.status_code == 201:
                data = resp.json()
                _inc("joins")
                if data.get("admitted") and data.get("token"):
                    self.token = data["token"]
                    _inc("admitted")
                resp.success()
            else:
                resp.failure(f"join returned {resp.status_code}")

    # ── Step 2: Poll until admitted ───────────────────────────────────────────
    def _poll_until_admitted(self):
        if self.token:
            return

        while not self.token:
            _inc("poll_total")
            time.sleep(POLL_INTERVAL)

            with self.client.get(
                f"{self.waiting_room_base}/queue/position",
                params={"user_id": self.user_id},
                name="/queue/position [poll]",
                catch_response=True,
            ) as resp:
                if resp.status_code == 200:
                    data = resp.json()
                    if data.get("admitted") and data.get("token"):
                        self.token = data["token"]
                        _inc("admitted")

                        admit_latency_ms = (time.time() - self.join_time) * 1000
                        self.environment.events.request.fire(
                            request_type="ADMISSION",
                            name="time_to_admit_ms",
                            response_time=admit_latency_ms,
                            response_length=0,
                            exception=None,
                            context={},
                        )
                    resp.success()
                elif resp.status_code == 404:
                    resp.failure("user not in queue")
                    _inc("never_admitted")
                    return
                else:
                    resp.failure(f"position returned {resp.status_code}")

    # ── Step 3: Purchase ──────────────────────────────────────────────────────
    def _attempt_purchase(self):
        if not self.token:
            _inc("never_admitted")
            return

        with self.client.post(
            f"{self.flash_sale_base}/purchase",
            json={"customer_id": self.user_id, "quantity": 1},
            name="/purchase",
            catch_response=True,
        ) as resp:
            if resp.status_code == 201:
                _inc("purchases_ok")
                resp.success()
            elif resp.status_code == 409:
                _inc("wasted_admissions")
                _set_sell_out()
                resp.failure("SOLD_OUT")
            else:
                _inc("purchase_errors")
                resp.failure(f"purchase returned {resp.status_code}")


# ── Event hooks ───────────────────────────────────────────────────────────────
@events.test_start.add_listener
def on_test_start(environment, **kwargs):
    global _sold_out, _sell_out_time, _experiment_start_time
    with _lock:
        _sold_out              = False
        _sell_out_time         = None
        _experiment_start_time = None
        for k in _counters:
            _counters[k] = 0

    if isinstance(environment.runner, WorkerRunner):
        return

    logging.info(
        f"[EXP3] Test starting — "
        f"admission_rate={ADMISSION_RATE}/s "
        f"num_workers={NUM_WORKERS} "
        f"users={USER_COUNT} "
        f"inventory={INVENTORY_COUNT} "
        f"poll_interval={POLL_INTERVAL}s"
    )

    # Auto-reset both services before each run.
    import requests
    base = WAITING_ROOM_URL or environment.host
    fs   = FLASH_SALE_URL   or environment.host
    try:
        requests.post(f"{base}/queue/reset", timeout=5)
        logging.info("[EXP3] Waiting room queue reset ✓")
    except Exception as e:
        logging.warning(f"[EXP3] Could not reset waiting room: {e}")
    try:
        requests.post(f"{fs}/reset", timeout=5)
        logging.info("[EXP3] Flash sale inventory reset ✓")
    except Exception as e:
        logging.warning(f"[EXP3] Could not reset flash sale: {e}")


@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    if isinstance(environment.runner, WorkerRunner):
        return

    with _lock:
        elapsed = (
            round(_sell_out_time - _experiment_start_time, 2)
            if _sold_out and _experiment_start_time else None
        )
        admitted   = _counters["admitted"]
        wasted_pct = (
            round(_counters["wasted_admissions"] / admitted * 100, 1)
            if admitted > 0 else 0.0
        )

    logging.info("=" * 60)
    logging.info("[EXP3] FINAL RESULTS")
    logging.info(f"  admission_rate:      {ADMISSION_RATE}/s")
    logging.info(f"  num_workers:         {NUM_WORKERS}")
    logging.info(f"  user_count:          {USER_COUNT}")
    logging.info(f"  inventory_count:     {INVENTORY_COUNT}")
    logging.info(f"  joins:               {_counters['joins']}")
    logging.info(f"  admitted:            {_counters['admitted']}")
    logging.info(f"  purchases_ok:        {_counters['purchases_ok']}")
    logging.info(f"  wasted_admissions:   {_counters['wasted_admissions']} ({wasted_pct}%)")
    logging.info(f"  never_admitted:      {_counters['never_admitted']}")
    logging.info(f"  purchase_errors:     {_counters['purchase_errors']}")
    logging.info(f"  total_polls:         {_counters['poll_total']}")
    logging.info(f"  sold_out:            {_sold_out}")
    logging.info(f"  time_to_sell_out:    {f'{elapsed}s' if elapsed else 'N/A (did not sell out)'}")
    logging.info("=" * 60)

    # Save machine-readable result alongside the CSV files.
    result = {
        "timestamp":         datetime.now(timezone.utc).isoformat(),
        "admission_rate":    ADMISSION_RATE,
        "num_workers":       NUM_WORKERS,
        "user_count":        USER_COUNT,
        "inventory_count":   INVENTORY_COUNT,
        "joins":             _counters["joins"],
        "admitted":          _counters["admitted"],
        "purchases_ok":      _counters["purchases_ok"],
        "wasted_admissions": _counters["wasted_admissions"],
        "wasted_pct":        wasted_pct,
        "never_admitted":    _counters["never_admitted"],
        "purchase_errors":   _counters["purchase_errors"],
        "total_polls":       _counters["poll_total"],
        "sold_out":          _sold_out,
        "time_to_sell_out":  elapsed,
    }

    tag = f"exp3_{USER_COUNT}u_rate{ADMISSION_RATE}_workers{NUM_WORKERS}"
    path = f"tests/results/{tag}.json"
    try:
        import os as _os
        _os.makedirs("tests/results", exist_ok=True)
        with open(path, "w") as f:
            json.dump(result, f, indent=2)
        logging.info(f"[EXP3] Result saved to {path}")
    except Exception as e:
        logging.warning(f"[EXP3] Could not save result JSON: {e}")