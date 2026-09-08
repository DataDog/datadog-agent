# `metric_filterlist_mixed_10k`

Measures what a **`metric_filterlist` that is mostly prefix rules, each one
carrying exceptions** costs the Agent:

| case | entries | exceptions |
|---|---|---|
| `metric_filterlist_mixed_10k` | 1,250 prefix + 5,000 exact | 10,000 |

**Every prefix entry carries 8 exceptions**, so the prefix rules are entirely made
of *guarded* rules: `Matcher` searches them in a separate arm from the
unconditional prefixes, and a hit there has to consult that entry's own
exception matcher before the metric can be dropped.

This case holds **1,250 prefix entries** (`<name>*`), each carrying 8
exceptions (10,000 in total), and **5,000 exact entries**.

This was originally one tier of a three-tier sweep (10k/50k/100k, sharing a
byte-identical `lading/lading.yaml`), with 10,000 total entries split evenly
between prefix and exact. The 50k and 100k tiers were dropped, and the traffic
was reduced from 60 MiB/s to 20 MiB/s: every tier crashed at the original rate.

That traffic cut alone did not fix it: this case has never actually run
through the real datadog-agent CI/SMP infrastructure (it does not exist on
`main`), and every replicate crashed with **zero target log output** —
before or during startup, not under sustained load. As a further diagnostic,
the prefix rule count was quartered by hand (5,000 → 1,250; exact entries
left at 5,000) and `memory_allotment` dropped from 8192 MiB to 4096 MiB (see
`experiment.yaml`), to check whether config size/rule count or the oversized
memory request — not traffic — is what's actually crashing it.

## Why

`pkg/util/metricname.Matcher` has three arms: `exact`, scanned by binary
search; `prefixes`, the unconditional prefixes, scanned by binary search plus
`strings.HasPrefix`; and `guarded`, the prefixes carrying exceptions, scanned
the same way but followed by a search of the matching entry's exception matcher.
A hit pays for one arm, a miss pays for all of them. Per-entry `*` prefixes and
their exceptions are new in this branch, so nothing measures either under load
yet.

Because every prefix entry here has exceptions, `prefixes` is empty and the
whole prefix half lives in `guarded`. That is the worst layout for the feature:
no prefix can be dropped as unconditional, and every prefix hit pays for an
exception lookup.

## Resources (cpu_allotment: 4, memory_allotment: 2048 MiB)

Dropped from the inherited `cpu_allotment: 6` / `memory_allotment: 8192 MiB`,
which were tuned in a separate `smp-playground` harness and never validated
against this repo's actual SMP runner hosts (8-vCPU `c6i.2xlarge`).

This isn't a guess: this branch already hit and fixed exactly this crash once
before, on the pre-exceptions version of this same case, at the same 20 MiB/s
traffic -- every replicate failed with **zero captured telemetry** (no
`total_pss_bytes`, no logs, nothing), consistent with a scheduling/capacity
failure (a 6-vCPU ask leaves little room to bin-pack alongside other
experiments' replicates on the same node pool), not an application crash. The
fix was `cpu_allotment: 4` / `memory_allotment: 2048 MiB` -- matching
`dsd_uds_10mb_3k_timestamped_contexts_*` in this same suite -- after which the
case ran to completion and was removed as done
(`git show b1437902bf8fb3041f467da34a3c075666be8a72^:test/regression/cases/metric_filterlist_mixed_10k/experiment.yaml`).
That case was re-added with exceptions but the resource allotment regressed
back to the original, pre-fix values; this restores the proven-safe ones.

## Traffic (20 MiB/s total)

| generator | rate | names | path exercised |
|---|---|---|---|
| `background` | 16 MiB/s | random, as `quality_gate_metrics_logs` | evaluate, forward |
| `prefix_hit` | 1.5 MiB/s | `…bench.prefix.<idx><pad>.hit` | guarded arm + exception lookup, dropped |
| `exact_hit` | 1.5 MiB/s | `…bench.exact.<idx><pad>` | exact arm, dropped |
| `miss` | 1 MiB/s | `…bench.nomatch.<idx><pad>` | **all three** arms, forwarded |

This was reduced from the original 48/4/4/4 MiB/s (60 MiB/s total), which
crashed every tier of this sweep. See `lading/lading.yaml` for the rationale
behind the specific split.

Every name on the wire is exactly 88 characters in all four namespaces, so
hit-vs-miss differences cannot come from name length. Prefix *entries* are 84
characters: `prefix_hit` appends `.hit`, so a prefix-hit name equals no entry
and its drop can only have come from the guarded-prefix arm.

The two groups live in **disjoint namespaces** (`…bench.prefix` /
`…bench.exact`) because `NewMatcher` silently drops prefixes covered by shorter
prefixes and exact entries covered by a prefix — otherwise this case would
compile to fewer live rules than it claims.

Exceptions live in a third disjoint namespace, `…bench.except`, for two reasons.
Nothing lading sends is ever excepted, so every generator's drop/forward
outcome is exactly what it was before exceptions existed and
`lading/lading.yaml` did not have to change; and every exception lookup is a
full miss, which searches both arms of the exception matcher to completion
instead of returning early. Within one entry no exception covers another, so
none is compacted away and each entry really does carry 8.

The exceptions are **written out per entry rather than shared through a YAML
anchor**: the Agent's YAML decoder rejects a document whose alias expansion
exceeds 10% of decoded nodes, which at scale allows only about one aliased
exception. That is what caps the list at 8 — see
`test/regression/scripts/add_filterlist_exceptions.py`.

## Reading the results

Compare this case's comparison run against its own baseline. The Regression
Detector's baseline is the merge base of the base branch, which has neither
prefix nor exception support and cannot even parse the object form an entry
with exceptions is written in: it filters only the exact half and does
strictly less work, so baseline-vs-comparison is "feature off vs on", not a
like-for-like regression.

## Regenerating

Generated by the smp-playground harness, not hand-authored:

```
cd experiments/regression/agent/metric_filterlist_scaling
python3 scripts/generate_filterlist_cases.py --mixed
# then copy cases/quality_gate_metric_filterlist_mixed_10k here, dropping the
# quality_gate_ prefix (this is a measurement case, not a quality gate)
```

That harness also validates every generated case, including against the real
production matcher (`scripts/validate_mixed_fixtures.sh`): compiled rule count,
every `prefix_hit`/`exact_hit` name dropped, every `miss` name kept.

Then re-apply the exceptions, which the harness does not know about yet:

```
python3 test/regression/scripts/add_filterlist_exceptions.py
```

Re-apply the traffic reduction, the prefix-count quartering, and the
`memory_allotment` drop documented above after regenerating, since the harness
still renders the original 60 MiB/s, 5,000 prefix entries, and 8192 MiB.

## Local run

```
smp local-run --experiment-dir test/regression \
  --case metric_filterlist_mixed_10k \
  --target-image <an image built from this branch>
```
