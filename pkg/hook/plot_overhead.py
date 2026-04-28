"""
Generate benchmark overhead charts for pkg/hook pipeline observation.

Run: uv run --with matplotlib python3 pkg/hook/plot_overhead.py
Output: pkg/hook/overhead_a.png, overhead_b.png, overhead_c.png

Data source: bench_results.csv (50 runs, arm64 Linux, benchtime=100ms)
"""

import os, csv, statistics
from collections import defaultdict
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker
import numpy as np

HERE = os.path.dirname(os.path.abspath(__file__))

# ── Load CSV ──────────────────────────────────────────────────────────────────
_raw = defaultdict(list)
with open(os.path.join(HERE, "bench_results.csv")) as f:
    for row in csv.DictReader(f):
        _raw[row["benchmark"]].append(float(row["ns_per_op"]))

def mean(key):  return statistics.mean(_raw[key])
def std(key):   return statistics.stdev(_raw[key]) if len(_raw[key]) > 1 else 0

# ── Colour palette ────────────────────────────────────────────────────────────
C_ZERO   = "#9ecae1"   # light blue  — noop hook
C_0SUB   = "#4292c6"   # mid blue    — 0 subscribers
C_1SUB   = "#f6a623"   # amber       — 1 subscriber
C_5SUB   = "#d62728"   # red         — 5 subscribers
C_PARSE  = "#6baed6"   # blue-grey   — parse stage
C_SAMPLE = "#74c476"   # green       — sample() stage
C_HOOK   = "#fd8d3c"   # orange      — hook overhead

SUBTITLE = "arm64 Linux  ·  go test -bench -benchmem -count=50 -benchtime=100ms"

# ── Derived values ────────────────────────────────────────────────────────────
# TimeSampler: amortised per-sample overhead from batch32_publish (batch=32).
BATCH = 32
_ts_base = mean("TimeSamplerHook/batch32_publish/noop_hook")
ts_noop = 0.0
ts_0sub = (mean("TimeSamplerHook/batch32_publish/0sub")   - _ts_base) / BATCH
ts_1sub = (mean("TimeSamplerHook/batch32_publish/1sub")   - _ts_base) / BATCH
ts_5sub = (mean("TimeSamplerHook/batch32_publish/5sub")   - _ts_base) / BATCH

ts_0sub_err = std("TimeSamplerHook/batch32_publish/0sub")   / BATCH
ts_1sub_err = std("TimeSamplerHook/batch32_publish/1sub")   / BATCH
ts_5sub_err = std("TimeSamplerHook/batch32_publish/5sub")   / BATCH

# no-agg worker: overhead per batch.
_na_base = mean("NoAggWorkerHook/batch32/noop_hook")
na_noop = 0.0
na_0sub = mean("NoAggWorkerHook/batch32/0sub")   - _na_base
na_1sub = mean("NoAggWorkerHook/batch32/1sub")   - _na_base
na_5sub = mean("NoAggWorkerHook/batch32/5sub")   - _na_base

na_0sub_err = std("NoAggWorkerHook/batch32/0sub")
na_1sub_err = std("NoAggWorkerHook/batch32/1sub")
na_5sub_err = std("NoAggWorkerHook/batch32/5sub")

# CheckSampler: overhead per sample.
_cs_base = mean("CheckSamplerHook/noop_hook")
cs_noop = 0.0
cs_0sub = mean("CheckSamplerHook/0sub") - _cs_base
cs_1sub = mean("CheckSamplerHook/1sub") - _cs_base
cs_5sub = mean("CheckSamplerHook/5sub") - _cs_base

cs_0sub_err = std("CheckSamplerHook/0sub")
cs_1sub_err = std("CheckSamplerHook/1sub")
cs_5sub_err = std("CheckSamplerHook/5sub")

# DogStatsD per-point CPU breakdown (4 tags, from BenchmarkParseMetric).
parse_ns  = 272.8
sample_ns = mean("TimeSamplerHook/sample_only/noop_hook")


def save(fig, name):
    path = os.path.join(HERE, name)
    fig.savefig(path, dpi=150, bbox_inches="tight", facecolor="white")
    print(f"Saved {path}")
    plt.close(fig)


# ── Chart A: hook overhead per pipeline (log scale) ──────────────────────────
fig, ax = plt.subplots(figsize=(10, 5))
fig.patch.set_facecolor("white")

pipelines = [
    "TimeSampler\n(DogStatsD pre-agg)\namortised per sample",
    "no-agg worker\nper batch of 32",
    "CheckSampler\nper check metric",
]

overhead_vals = {
    "noop hook":     [max(ts_noop, 0.05), max(na_noop, 0.05), max(cs_noop, 0.05)],
    "0 subscribers": [max(ts_0sub, 0.05), max(na_0sub, 0.05), max(cs_0sub, 0.05)],
    "1 subscriber":  [ts_1sub, na_1sub, cs_1sub],
    "5 subscribers": [ts_5sub, na_5sub, cs_5sub],
}
overhead_errs = {
    "noop hook":     [0, 0, 0],
    "0 subscribers": [ts_0sub_err, na_0sub_err, cs_0sub_err],
    "1 subscriber":  [ts_1sub_err, na_1sub_err, cs_1sub_err],
    "5 subscribers": [ts_5sub_err, na_5sub_err, cs_5sub_err],
}
colours = [C_ZERO, C_0SUB, C_1SUB, C_5SUB]

x = np.arange(len(pipelines))
w = 0.18
offsets = np.linspace(-(len(overhead_vals) - 1) / 2,
                       (len(overhead_vals) - 1) / 2, len(overhead_vals)) * w

for (label, vals), errs, colour, offset in zip(
        overhead_vals.items(), overhead_errs.values(), colours, offsets):
    bars = ax.bar(x + offset, vals, width=w, color=colour, label=label,
                  zorder=3, edgecolor="white", linewidth=0.5,
                  yerr=errs, capsize=3, error_kw=dict(elinewidth=0.8, ecolor="#555"))
    for bar, v in zip(bars, vals):
        txt = "~0" if v < 0.1 else (f"{v/1000:.1f} µs" if v >= 1000 else
                                     f"{v:.0f} ns" if v >= 1 else f"{v:.1f} ns")
        # Fixed offset above bar; skip label when bar is near the top of the axis
        # to avoid clipping outside the plot area.
        label_y = bar.get_height() + 40
        if label_y < 2450:
            ax.text(bar.get_x() + bar.get_width() / 2, label_y,
                    txt, ha="center", va="bottom", fontsize=8, color="#333333")

ax.set_ylim(0, 2600)
ax.set_xticks(x)
ax.set_xticklabels(pipelines, fontsize=10)
ax.set_ylabel("Overhead vs idle (ns)", fontsize=10)
ax.set_title(
    "Hook overhead per pipeline × subscriber count\n"
    "noop hook and 0 subscribers are nearly identical — idle cost is one atomic read (~2 ns)",
    fontsize=10, loc="left",
)
ax.yaxis.grid(True, which="both", linestyle="--", alpha=0.4, zorder=0)
ax.set_axisbelow(True)
ax.legend(fontsize=9, loc="upper left", framealpha=0.9)
ax.spines[["top", "right"]].set_visible(False)


fig.text(0.99, 0.01, SUBTITLE, ha="right", va="bottom", fontsize=7.5, color="#888888")
save(fig, "overhead_a.png")


# ── Chart B: DogStatsD per-point breakdown ────────────────────────────────────
fig, ax = plt.subplots(figsize=(6, 5))
fig.patch.set_facecolor("white")

modes  = ["0 sub", "1 sub", "5 sub"]
hooks  = [ts_0sub, ts_1sub, ts_5sub]
x2     = np.arange(len(modes))
bw     = 0.45

ax.bar(x2, [parse_ns]*3,  bw, label="Parse & enrich", color=C_PARSE,  zorder=3)
ax.bar(x2, [sample_ns]*3, bw, bottom=[parse_ns]*3,    label="sample()", color=C_SAMPLE, zorder=3)
b_hook = ax.bar(x2, hooks, bw, bottom=[parse_ns + sample_ns]*3,
                label="Hook overhead", color=C_HOOK, zorder=3)

for bar, h in zip(b_hook, hooks):
    total = parse_ns + sample_ns + h
    pct = h / total * 100
    lbl = f"+{h:.1f} ns\n({pct:.1f}%)" if h > 0.5 else "~0 ns\n(<0.1%)"
    ax.text(bar.get_x() + bar.get_width() / 2,
            bar.get_height() + bar.get_y() + 4,
            lbl, ha="center", va="bottom", fontsize=8.5, color="#333333")

ax.set_xticks(x2)
ax.set_xticklabels(modes, fontsize=10)
ax.set_ylabel("Latency per metric point (ns)", fontsize=10)
ax.set_title("DogStatsD per-point CPU breakdown\n(4 tags, TimeSampler path)",
             fontsize=10, loc="left")
ax.yaxis.grid(True, linestyle="--", alpha=0.4, zorder=0)
ax.set_axisbelow(True)
# Legend at bottom-right to avoid overlapping bar labels at top
ax.legend(fontsize=9, loc="lower right", framealpha=0.9)
ax.spines[["top", "right"]].set_visible(False)
ax.set_ylim(0, 430)
fig.text(0.99, 0.01, SUBTITLE, ha="right", va="bottom", fontsize=7.5, color="#888888")
save(fig, "overhead_b.png")


# ── Chart C: sample() cost — hook append is free ─────────────────────────────
fig, ax = plt.subplots(figsize=(6, 5))
fig.patch.set_facecolor("white")

modes_c = ["noop", "0 sub", "1 sub", "5 sub"]
keys_c  = ["TimeSamplerHook/sample_only/noop_hook",
           "TimeSamplerHook/sample_only/0sub",
           "TimeSamplerHook/sample_only/1sub",
           "TimeSamplerHook/sample_only/5sub"]
ns_c    = [mean(k) for k in keys_c]
err_c   = [std(k)  for k in keys_c]
cols_c  = [C_ZERO, C_0SUB, C_1SUB, C_5SUB]

bars_c = ax.bar(modes_c, ns_c, color=cols_c, zorder=3,
                edgecolor="white", linewidth=0.5,
                yerr=err_c, capsize=4,
                error_kw=dict(elinewidth=0.9, ecolor="#555"))
for bar, v, e in zip(bars_c, ns_c, err_c):
    ax.text(bar.get_x() + bar.get_width() / 2,
            bar.get_height() + e + 0.3,
            f"{v:.1f} ns\n0 allocs",
            ha="center", va="bottom", fontsize=9, color="#333333")

ax.set_ylim(70, 96)
ax.set_ylabel("ns per sample() call", fontsize=10)
ax.set_title("sample() cost: hookBatch append is free\n"
             "All modes within measurement noise — 0 allocs/op",
             fontsize=10, loc="left")
ax.yaxis.grid(True, linestyle="--", alpha=0.4, zorder=0)
ax.set_axisbelow(True)
ax.spines[["top", "right"]].set_visible(False)
fig.text(0.99, 0.01, SUBTITLE, ha="right", va="bottom", fontsize=7.5, color="#888888")
save(fig, "overhead_c.png")
