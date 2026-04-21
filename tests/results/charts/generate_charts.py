#!/usr/bin/env python3
"""Generate charts for the final experiments report from CSVs in tests/results/."""
from __future__ import annotations

import csv
import os
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

RESULTS = Path(__file__).resolve().parent.parent
CHARTS = RESULTS / "charts"
CHARTS.mkdir(exist_ok=True)

def read_csv(path):
    with open(path) as f:
        return list(csv.DictReader(f))

# ------------------------------------------------------------------
# Experiment 1 — waiting-room fairness
# ------------------------------------------------------------------
exp1 = read_csv(RESULTS / "exp1_sweep.csv")
# Filter to the v2 rows so the chart compares apples to apples (same atomic code path,
# only scoring differs). The pre-fix rows are kept in the CSV for the narrative but
# excluded from the chart.
exp1_v2 = [r for r in exp1 if r["strategy"] in ("timestamp_v2", "timestamp_incr_v2")]
users = sorted({int(r["users"]) for r in exp1_v2})

fig, ax = plt.subplots(figsize=(7, 4.2))
for strat, color, label in [
    ("timestamp_v2", "#d62728", "timestamp-only (Strategy A)"),
    ("timestamp_incr_v2", "#2ca02c", "monotonic INCR (Strategy B)"),
]:
    rows = sorted([r for r in exp1_v2 if r["strategy"] == strat], key=lambda r: int(r["users"]))
    xs = [int(r["users"]) for r in rows]
    ys = [float(r["collision_rate_pct"]) for r in rows]
    ax.plot(xs, ys, marker="o", linewidth=2, color=color, label=label)
ax.set_xlabel("Concurrent join requests")
ax.set_ylabel("Position collision rate (%)")
ax.set_title("Experiment 1 — Queue fairness: scoring strategy vs. collision rate")
ax.set_ylim(bottom=-2)
ax.grid(True, alpha=0.3)
ax.legend()
fig.tight_layout()
fig.savefig(CHARTS / "exp1_collision_rate.png", dpi=140)
plt.close(fig)

# Latency chart too
fig, ax = plt.subplots(figsize=(7, 4.2))
for strat, color, label in [
    ("timestamp_v2", "#d62728", "timestamp-only"),
    ("timestamp_incr_v2", "#2ca02c", "monotonic INCR"),
]:
    rows = sorted([r for r in exp1_v2 if r["strategy"] == strat], key=lambda r: int(r["users"]))
    xs = [int(r["users"]) for r in rows]
    ys = [int(r["latency_p99_ms"]) for r in rows]
    ax.plot(xs, ys, marker="s", linewidth=2, color=color, label=label)
ax.set_xlabel("Concurrent join requests")
ax.set_ylabel("p99 latency (ms)")
ax.set_title("Experiment 1 — Join p99 latency")
ax.grid(True, alpha=0.3)
ax.legend()
fig.tight_layout()
fig.savefig(CHARTS / "exp1_p99_latency.png", dpi=140)
plt.close(fig)

# ------------------------------------------------------------------
# Experiment 2 — inventory correctness
# ------------------------------------------------------------------
exp2 = read_csv(RESULTS / "exp2_sweep.csv")
buyers = sorted({int(r["buyers"]) for r in exp2})

fig, ax = plt.subplots(figsize=(7, 4.2))
width = 0.25
x_positions = list(range(len(buyers)))
for i, (strat, color) in enumerate([("atomic", "#1f77b4"), ("optimistic", "#d62728"), ("lua", "#2ca02c")]):
    rows = sorted([r for r in exp2 if r["strategy"] == strat], key=lambda r: int(r["buyers"]))
    xs = [p + (i - 1) * width for p in x_positions]
    ys = [int(r["accepted"]) for r in rows]
    ax.bar(xs, ys, width=width, color=color, label=strat)
ax.set_xticks(x_positions)
ax.set_xticklabels([str(b) for b in buyers])
ax.set_xlabel("Concurrent buyers")
ax.set_ylabel("Accepted purchases (of 100 available)")
ax.set_title("Experiment 2 — Inventory strategy: accepted purchases (100 units stock)")
ax.axhline(100, color="gray", linestyle="--", alpha=0.5, label="ideal (sell-out at 100)")
ax.grid(True, alpha=0.3, axis="y")
ax.legend()
fig.tight_layout()
fig.savefig(CHARTS / "exp2_accepted.png", dpi=140)
plt.close(fig)

# Throughput chart
fig, ax = plt.subplots(figsize=(7, 4.2))
for strat, color in [("atomic", "#1f77b4"), ("optimistic", "#d62728"), ("lua", "#2ca02c")]:
    rows = sorted([r for r in exp2 if r["strategy"] == strat], key=lambda r: int(r["buyers"]))
    xs = [int(r["buyers"]) for r in rows]
    ys = [float(r["throughput_rps"]) for r in rows]
    ax.plot(xs, ys, marker="o", linewidth=2, color=color, label=strat)
ax.set_xlabel("Concurrent buyers")
ax.set_ylabel("Throughput (requests/sec)")
ax.set_title("Experiment 2 — Throughput by strategy")
ax.grid(True, alpha=0.3)
ax.legend()
fig.tight_layout()
fig.savefig(CHARTS / "exp2_throughput.png", dpi=140)
plt.close(fig)

# ------------------------------------------------------------------
# Experiment 3 — admission rate tuning
# ------------------------------------------------------------------
exp3 = read_csv(RESULTS / "exp3_sweep.csv")

def rate_of(label):
    # label format: rate{N}ps
    return int(label.replace("rate", "").replace("ps", ""))

exp3 = sorted(exp3, key=lambda r: rate_of(r["label"]))
rates = [rate_of(r["label"]) for r in exp3]
sellout_ms = [int(r["time_to_sellout_ms"]) for r in exp3]
wasted = [int(r["wasted_admissions"]) for r in exp3]

fig, ax1 = plt.subplots(figsize=(7.5, 4.5))
# time to sellout (show -1 as "did not sell out" — replace with None to hide)
y1 = [s if s >= 0 else None for s in sellout_ms]
ax1.plot(rates, y1, marker="o", linewidth=2, color="#1f77b4", label="time to sell out (ms)")
ax1.set_xlabel("Admission rate (users / sec)")
ax1.set_ylabel("Time to sell out (ms)", color="#1f77b4")
ax1.tick_params(axis="y", labelcolor="#1f77b4")
ax1.set_xscale("log")

ax2 = ax1.twinx()
ax2.plot(rates, wasted, marker="s", linewidth=2, color="#d62728", label="wasted admissions")
ax2.set_ylabel("Wasted admissions", color="#d62728")
ax2.tick_params(axis="y", labelcolor="#d62728")

ax1.set_title("Experiment 3 — Admission rate tradeoff\n(500 users, 100 inventory)")
ax1.grid(True, alpha=0.3)

# Annotate "did not sell out" for the 1/s point
for r, s in zip(rates, sellout_ms):
    if s < 0:
        ax1.annotate("did not\nsell out", xy=(r, 0), xytext=(r * 1.05, 5000),
                     fontsize=8, color="#1f77b4")

fig.tight_layout()
fig.savefig(CHARTS / "exp3_admission_tradeoff.png", dpi=140)
plt.close(fig)

print("Generated charts in", CHARTS)
for p in sorted(CHARTS.glob("exp*.png")):
    print(" -", p.name)
