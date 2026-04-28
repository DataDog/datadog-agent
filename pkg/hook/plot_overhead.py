"""
Generate benchmark overhead charts for pkg/hook pipeline observation.

Run: python3 pkg/hook/plot_overhead.py
Output: pkg/hook/overhead.png
"""

import matplotlib.pyplot as plt
import matplotlib.patches as mpatches
import numpy as np

# ── Colour palette ────────────────────────────────────────────────────────────
C_ZERO   = "#9ecae1"   # light blue  — zero/noop (idle)
C_0SUB   = "#4292c6"   # mid blue    — 0 subscribers
C_1SUB   = "#f6a623"   # amber       — 1 subscriber
C_5SUB   = "#d62728"   # red         — 5 subscribers
C_PARSE  = "#6baed6"   # blue-grey   — parse stage
C_SAMPLE = "#74c476"   # green       — sample() stage
C_HOOK   = "#fd8d3c"   # orange      — hook overhead

# ── Benchmark data (arm64 Linux, go test -bench -benchmem -benchtime=500ms) ──
# All values in nanoseconds.

# TimeSampler: amortised per-sample cost from batch32_publish.
# overhead = (batch_ns - noop_ns) / 32
ts_noop  = (2626 - 2626) / 32   # 0.0 ns
ts_0sub  = (2682 - 2626) / 32   # 1.75 ns
ts_1sub  = (2827 - 2626) / 32   # 6.3 ns
ts_5sub  = (3363 - 2626) / 32   # 23.0 ns

# no-agg worker: overhead per batch of 32 (allocates when active).
na_noop  = 2.17 - 2.14          # ≈ 0 ns
na_0sub  = 2.17 - 2.14          # ≈ 0 ns
na_1sub  = 1544 - 2.14          # 1542 ns
na_5sub  = 2300 - 2.14          # 2298 ns

# CheckSampler: overhead per check metric sample.
cs_noop  = 2.17 - 2.14          # ≈ 0 ns
cs_0sub  = 2.17 - 2.14          # ≈ 0 ns
cs_1sub  = 405  - 2.14          # 403 ns
cs_5sub  = 975  - 2.14          # 973 ns

# DogStatsD per-point CPU breakdown (4 tags).
parse_ns  = 272.8
sample_ns = 82.0

# ── Figure ────────────────────────────────────────────────────────────────────
fig = plt.figure(figsize=(13, 9))
fig.patch.set_facecolor("white")

gs = fig.add_gridspec(
    2, 2,
    left=0.07, right=0.97,
    top=0.92,  bottom=0.08,
    hspace=0.45, wspace=0.35,
)

# ── Panel A: hook overhead by pipeline (log scale) ────────────────────────────
ax_a = fig.add_subplot(gs[0, :])

pipelines = ["TimeSampler\n(DogStatsD pre-agg)\namortised per sample",
             "no-agg worker\nper batch of 32",
             "CheckSampler\nper check metric"]

overhead = {
    "noop / 0 sub": [max(ts_0sub, 0.05), max(na_0sub, 0.05), max(cs_0sub, 0.05)],
    "1 subscriber":  [ts_1sub, na_1sub, cs_1sub],
    "5 subscribers": [ts_5sub, na_5sub, cs_5sub],
}
colours = [C_0SUB, C_1SUB, C_5SUB]

x = np.arange(len(pipelines))
n_groups = len(overhead)
w = 0.24
offsets = np.linspace(-(n_groups - 1) / 2, (n_groups - 1) / 2, n_groups) * w

for (label, vals), colour, offset in zip(overhead.items(), colours, offsets):
    bars = ax_a.bar(x + offset, vals, width=w, color=colour, label=label,
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
        ax_a.text(
            bar.get_x() + bar.get_width() / 2,
            bar.get_height() * 1.25,
            txt,
            ha="center", va="bottom", fontsize=7.5, color="#333333",
        )

ax_a.set_yscale("log")
ax_a.set_ylim(0.03, 12000)
ax_a.set_xticks(x)
ax_a.set_xticklabels(pipelines, fontsize=9)
ax_a.set_ylabel("Overhead vs idle (ns, log scale)", fontsize=9)
ax_a.set_title("A — Hook overhead per pipeline × subscriber count\n"
               "(noop and 0-subscriber bars are identical — idle cost is one atomic read, ~2 ns)",
               fontsize=9.5, loc="left")
ax_a.yaxis.grid(True, which="both", linestyle="--", alpha=0.4, zorder=0)
ax_a.set_axisbelow(True)
ax_a.legend(fontsize=8.5, loc="upper left", framealpha=0.9)
ax_a.spines[["top", "right"]].set_visible(False)

# Annotate the alloc note on no-agg and check
for xpos, note in zip([1, 2], ["2 allocs/batch\n(no accumulator)", "2 allocs/sample\n(no accumulator)"]):
    ax_a.annotate(
        note,
        xy=(xpos + offsets[1], overhead["1 subscriber"][xpos]),
        xytext=(xpos + offsets[1] + 0.35, overhead["1 subscriber"][xpos] * 1.8),
        fontsize=7, color="#555555",
        arrowprops=dict(arrowstyle="->", color="#888888", lw=0.8),
    )

# ── Panel B: DogStatsD per-point breakdown (4 tags) ───────────────────────────
ax_b = fig.add_subplot(gs[1, 0])

modes  = ["0 sub", "1 sub", "5 sub"]
hooks  = [ts_0sub, ts_1sub, ts_5sub]
x2     = np.arange(len(modes))
bw     = 0.45

b_parse  = ax_b.bar(x2, [parse_ns]*3,  bw, label="Parse & enrich", color=C_PARSE,  zorder=3)
b_sample = ax_b.bar(x2, [sample_ns]*3, bw, bottom=[parse_ns]*3,
                    label="sample()", color=C_SAMPLE, zorder=3)
b_hook   = ax_b.bar(x2, hooks, bw, bottom=[parse_ns + sample_ns]*3,
                    label="Hook overhead", color=C_HOOK, zorder=3)

for bar, h in zip(b_hook, hooks):
    pct = h / (parse_ns + sample_ns + h) * 100
    label = f"+{h:.1f} ns\n({pct:.1f}%)" if h > 0.5 else "~0 ns\n(<0.1%)"
    ax_b.text(
        bar.get_x() + bar.get_width() / 2,
        bar.get_height() + bar.get_y() + 4,
        label,
        ha="center", va="bottom", fontsize=7.5, color="#333333",
    )

ax_b.set_xticks(x2)
ax_b.set_xticklabels(modes, fontsize=9)
ax_b.set_ylabel("Latency per metric point (ns)", fontsize=9)
ax_b.set_title("B — DogStatsD pipeline breakdown\n(4 tags, TimeSampler path)",
               fontsize=9.5, loc="left")
ax_b.yaxis.grid(True, linestyle="--", alpha=0.4, zorder=0)
ax_b.set_axisbelow(True)
ax_b.legend(fontsize=8, loc="upper left", framealpha=0.9)
ax_b.spines[["top", "right"]].set_visible(False)
ax_b.set_ylim(0, 430)

# ── Panel C: TimeSampler — sample_only alloc proof ────────────────────────────
ax_c = fig.add_subplot(gs[1, 1])

modes_c = ["noop", "0 sub", "1 sub", "5 sub"]
ns_c    = [82.35, 82.48, 81.77, 81.93]
cols_c  = [C_ZERO, C_0SUB, C_1SUB, C_5SUB]

bars_c = ax_c.bar(modes_c, ns_c, color=cols_c, zorder=3,
                  edgecolor="white", linewidth=0.5)
for bar, v in zip(bars_c, ns_c):
    ax_c.text(bar.get_x() + bar.get_width() / 2, bar.get_height() + 0.3,
              f"{v:.1f} ns\n0 allocs",
              ha="center", va="bottom", fontsize=8, color="#333333")

ax_c.set_ylim(78, 88)
ax_c.set_ylabel("ns per sample()", fontsize=9)
ax_c.set_title("C — sample() cost: hook append is free\n"
               "(all modes within measurement noise, 0 allocs/op)",
               fontsize=9.5, loc="left")
ax_c.yaxis.grid(True, linestyle="--", alpha=0.4, zorder=0)
ax_c.set_axisbelow(True)
ax_c.spines[["top", "right"]].set_visible(False)

# ── Title ─────────────────────────────────────────────────────────────────────
fig.suptitle(
    "pkg/hook — Pipeline observation overhead  ·  arm64 Linux  ·  go test -bench -benchmem",
    fontsize=10.5, y=0.97, color="#222222",
)

import os
out = os.path.join(os.path.dirname(os.path.abspath(__file__)), "overhead.png")
plt.savefig(out, dpi=150, bbox_inches="tight", facecolor="white")
print(f"Saved {out}")
