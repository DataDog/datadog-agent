"""
Generate benchmark overhead charts for pkg/hook pipeline observation.

Run: uv run --with matplotlib python3 pkg/hook/plot_overhead.py
Output: pkg/hook/overhead_a.png, overhead_b.png, overhead_c.png
"""

import os
import matplotlib.pyplot as plt
import numpy as np

HERE = os.path.dirname(os.path.abspath(__file__))

# ── Colour palette ────────────────────────────────────────────────────────────
C_ZERO   = "#9ecae1"   # light blue  — zero/noop (idle)
C_0SUB   = "#4292c6"   # mid blue    — 0 subscribers
C_1SUB   = "#f6a623"   # amber       — 1 subscriber
C_5SUB   = "#d62728"   # red         — 5 subscribers
C_PARSE  = "#6baed6"   # blue-grey   — parse stage
C_SAMPLE = "#74c476"   # green       — sample() stage
C_HOOK   = "#fd8d3c"   # orange      — hook overhead

# ── Benchmark data (arm64 Linux, go test -bench -benchmem -benchtime=500ms) ──
# TimeSampler: amortised per-sample cost from batch32_publish.
ts_noop = (2626 - 2626) / 32   # 0.0 ns
ts_0sub = (2682 - 2626) / 32   # 1.75 ns
ts_1sub = (2827 - 2626) / 32   # 6.3 ns
ts_5sub = (3363 - 2626) / 32   # 23.0 ns

# no-agg worker: overhead per batch of 32.
na_noop = 2.17 - 2.14          # ≈ 0 ns
na_0sub = 2.17 - 2.14
na_1sub = 1544 - 2.14          # 1542 ns
na_5sub = 2300 - 2.14          # 2298 ns

# CheckSampler: overhead per check metric sample.
cs_noop = 2.17 - 2.14
cs_0sub = 2.17 - 2.14
cs_1sub = 405  - 2.14          # 403 ns
cs_5sub = 975  - 2.14          # 973 ns

# DogStatsD per-point CPU breakdown (4 tags).
parse_ns  = 272.8
sample_ns = 82.0

SUBTITLE = "arm64 Linux  ·  go test -bench -benchmem"


def save(fig, name):
    path = os.path.join(HERE, name)
    fig.savefig(path, dpi=150, bbox_inches="tight", facecolor="white")
    print(f"Saved {path}")
    plt.close(fig)


# ── Chart A: hook overhead by pipeline (log scale) ───────────────────────────
fig, ax = plt.subplots(figsize=(10, 5))
fig.patch.set_facecolor("white")

pipelines = [
    "TimeSampler\n(DogStatsD pre-agg)\namortised per sample",
    "no-agg worker\nper batch of 32",
    "CheckSampler\nper check metric",
]
overhead = {
    "noop / 0 sub": [max(ts_0sub, 0.05), max(na_0sub, 0.05), max(cs_0sub, 0.05)],
    "1 subscriber":  [ts_1sub, na_1sub, cs_1sub],
    "5 subscribers": [ts_5sub, na_5sub, cs_5sub],
}
colours = [C_0SUB, C_1SUB, C_5SUB]

x = np.arange(len(pipelines))
w = 0.24
offsets = np.linspace(-(len(overhead) - 1) / 2, (len(overhead) - 1) / 2, len(overhead)) * w

for (label, vals), colour, offset in zip(overhead.items(), colours, offsets):
    bars = ax.bar(x + offset, vals, width=w, color=colour, label=label,
                  zorder=3, edgecolor="white", linewidth=0.5)
    for bar, v in zip(bars, vals):
        if v < 0.1:
            txt = "~0"
        elif v >= 1000:
            txt = f"{v/1000:.1f} µs"
        elif v >= 1:
            txt = f"{v:.0f} ns"
        else:
            txt = f"{v:.1f} ns"
        ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() * 1.3,
                txt, ha="center", va="bottom", fontsize=8, color="#333333")

ax.set_yscale("log")
ax.set_ylim(0.03, 12000)
ax.set_xticks(x)
ax.set_xticklabels(pipelines, fontsize=10)
ax.set_ylabel("Overhead vs idle (ns, log scale)", fontsize=10)
ax.set_title(
    "Hook overhead per pipeline × subscriber count\n"
    "noop and 0-subscriber are identical — idle cost is one atomic read (~2 ns)",
    fontsize=10, loc="left",
)
ax.yaxis.grid(True, which="both", linestyle="--", alpha=0.4, zorder=0)
ax.set_axisbelow(True)
ax.legend(fontsize=9, loc="upper left", framealpha=0.9)
ax.spines[["top", "right"]].set_visible(False)

for xpos, note in zip([1, 2], ["2 allocs/batch\n(no accumulator)", "2 allocs/sample\n(no accumulator)"]):
    ax.annotate(
        note,
        xy=(xpos + offsets[1], overhead["1 subscriber"][xpos]),
        xytext=(xpos + offsets[1] + 0.38, overhead["1 subscriber"][xpos] * 2),
        fontsize=7.5, color="#555555",
        arrowprops=dict(arrowstyle="->", color="#888888", lw=0.8),
    )

fig.text(0.99, 0.01, SUBTITLE, ha="right", va="bottom", fontsize=7.5, color="#888888")
save(fig, "overhead_a.png")


# ── Chart B: DogStatsD per-point latency breakdown ───────────────────────────
fig, ax = plt.subplots(figsize=(6, 5))
fig.patch.set_facecolor("white")

modes = ["0 sub", "1 sub", "5 sub"]
hooks = [ts_0sub, ts_1sub, ts_5sub]
x2    = np.arange(len(modes))
bw    = 0.45

ax.bar(x2, [parse_ns]*3,  bw, label="Parse & enrich", color=C_PARSE,  zorder=3)
ax.bar(x2, [sample_ns]*3, bw, bottom=[parse_ns]*3,    label="sample()", color=C_SAMPLE, zorder=3)
b_hook = ax.bar(x2, hooks, bw, bottom=[parse_ns + sample_ns]*3,
                label="Hook overhead", color=C_HOOK, zorder=3)

for bar, h in zip(b_hook, hooks):
    pct = h / (parse_ns + sample_ns + h) * 100
    lbl = f"+{h:.1f} ns\n({pct:.1f}%)" if h > 0.5 else "~0 ns\n(<0.1%)"
    ax.text(bar.get_x() + bar.get_width() / 2,
            bar.get_height() + bar.get_y() + 4,
            lbl, ha="center", va="bottom", fontsize=8, color="#333333")

ax.set_xticks(x2)
ax.set_xticklabels(modes, fontsize=10)
ax.set_ylabel("Latency per metric point (ns)", fontsize=10)
ax.set_title("DogStatsD per-point CPU breakdown\n(4 tags, TimeSampler path)", fontsize=10, loc="left")
ax.yaxis.grid(True, linestyle="--", alpha=0.4, zorder=0)
ax.set_axisbelow(True)
ax.legend(fontsize=9, loc="upper left", framealpha=0.9)
ax.spines[["top", "right"]].set_visible(False)
ax.set_ylim(0, 430)
fig.text(0.99, 0.01, SUBTITLE, ha="right", va="bottom", fontsize=7.5, color="#888888")
save(fig, "overhead_b.png")


# ── Chart C: sample() cost — hook append is free ─────────────────────────────
fig, ax = plt.subplots(figsize=(6, 5))
fig.patch.set_facecolor("white")

modes_c = ["noop", "0 sub", "1 sub", "5 sub"]
ns_c    = [82.35, 82.48, 81.77, 81.93]
cols_c  = [C_ZERO, C_0SUB, C_1SUB, C_5SUB]

bars_c = ax.bar(modes_c, ns_c, color=cols_c, zorder=3, edgecolor="white", linewidth=0.5)
for bar, v in zip(bars_c, ns_c):
    ax.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 0.2,
            f"{v:.1f} ns\n0 allocs",
            ha="center", va="bottom", fontsize=9, color="#333333")

ax.set_ylim(78, 88)
ax.set_ylabel("ns per sample() call", fontsize=10)
ax.set_title("sample() cost: hookBatch append is free\n"
             "All modes within measurement noise — 0 allocs/op", fontsize=10, loc="left")
ax.yaxis.grid(True, linestyle="--", alpha=0.4, zorder=0)
ax.set_axisbelow(True)
ax.spines[["top", "right"]].set_visible(False)
fig.text(0.99, 0.01, SUBTITLE, ha="right", va="bottom", fontsize=7.5, color="#888888")
save(fig, "overhead_c.png")
