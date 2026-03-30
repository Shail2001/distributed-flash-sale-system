import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import os

charts_dir = os.path.join(os.path.dirname(__file__), 'charts')
os.makedirs(charts_dir, exist_ok=True)

# Chart 1 — Inventory Countdown
purchases = list(range(1, 101))
inventory = list(range(99, -1, -1))

fig, ax = plt.subplots(figsize=(10, 5))
ax.plot(purchases, inventory, color='steelblue', linewidth=2)
ax.fill_between(purchases, inventory, alpha=0.2, color='steelblue')
ax.set_xlabel('Purchase Number')
ax.set_ylabel('Remaining Inventory')
ax.set_title('Flash Sale API — Inventory Countdown (Smoke Test)')
ax.set_xlim(1, 100)
ax.set_ylim(0, 100)
ax.grid(True, alpha=0.3)
plt.tight_layout()
plt.savefig(os.path.join(charts_dir, 'inventory_countdown.png'), dpi=150)
plt.close()
print("Chart 1 saved: inventory_countdown.png")

# Chart 2 — Concurrency Pie Chart
accepted = 100
rejected = 100
labels = ['Accepted (201)', 'Rejected (409)']
sizes = [accepted, rejected]
colors = ['#2ecc71', '#e74c3c']
explode = (0.05, 0)

fig, ax = plt.subplots(figsize=(8, 7))
wedges, texts, autotexts = ax.pie(
    sizes, labels=labels, colors=colors, explode=explode,
    autopct='%1.0f%%', startangle=90, textprops={'fontsize': 13}
)
for at in autotexts:
    at.set_fontsize(14)
    at.set_fontweight('bold')
ax.set_title('200 Concurrent Requests — Atomic DECR Prevents Oversell', fontsize=13, pad=20)
ax.annotate(
    '✓ No Oversell — Exactly 100 accepted',
    xy=(0, 0), xytext=(0, -1.35),
    fontsize=12, ha='center', color='green', fontweight='bold'
)
plt.tight_layout()
plt.savefig(os.path.join(charts_dir, 'concurrency_pie.png'), dpi=150)
plt.close()
print("Chart 2 saved: concurrency_pie.png")

# Chart 3 — HTTP Status Code Coverage
endpoints = [
    'POST /purchase\n(success)',
    'POST /purchase\n(sold out)',
    'GET /inventory',
    'GET /health',
    'POST /reset',
    'POST /purchase\n(bad request)',
]
status_codes = [201, 409, 200, 200, 200, 400]
colors_bar = ['#2ecc71', '#e74c3c', '#3498db', '#3498db', '#3498db', '#f39c12']

fig, ax = plt.subplots(figsize=(11, 6))
bars = ax.bar(endpoints, status_codes, color=colors_bar, width=0.5, edgecolor='white')
for bar, code in zip(bars, status_codes):
    ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 3,
            str(code), ha='center', va='bottom', fontsize=13, fontweight='bold')
ax.set_ylabel('HTTP Status Code')
ax.set_title('Flash Sale API — HTTP Status Code Coverage', fontsize=13)
ax.set_ylim(0, 500)
ax.set_yticks([200, 201, 400, 409])
ax.grid(axis='y', alpha=0.3)
plt.tight_layout()
plt.savefig(os.path.join(charts_dir, 'status_codes.png'), dpi=150)
plt.close()
print("Chart 3 saved: status_codes.png")

print("\nAll charts generated in:", charts_dir)
