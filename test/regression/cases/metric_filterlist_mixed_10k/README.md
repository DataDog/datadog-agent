# `metric_filterlist_mixed_10k`

Measures the cost of exception-bearing metric prefix rules.

| case | entries | exceptions |
|---|---|---|
| `metric_filterlist_mixed_10k` | 1,250 prefix + 5,000 exact | 10,000 |

Every prefix entry carries 8 exceptions, so the prefix side stays in
`Matcher.rules` (the linear exception-bearing path), not in the compacted bare
`prefixes` path. Exact entries stay under `metric_filterlist`; prefix rules use
`metric_filterlist_prefix`.

## ADP compatibility

Agent Data Plane only accepts a flat string list under `metric_filterlist`.
Object-form prefix rules live under `metric_filterlist_prefix`, which ADP does
not read, so the flat list remains ADP-safe.

## Fixture shape

Generated from the smp-playground `metric_filterlist_scaling` harness and copied
here. This PR keeps only the former 10k mixed tier, reduces traffic from
60 MiB/s to 20 MiB/s, and quarters prefix rules from 5,000 to 1,250. Exact rules
remain at 5,000.

Exceptions live in the disjoint `…bench.except` namespace. Lading never sends
excepted names, so each exception lookup is a miss and the send/forward outcome
stays unchanged. Exceptions are written per entry, not with YAML anchors: the
Agent YAML decoder rejects large alias expansions.

## Resources

`cpu_allotment: 4` and `memory_allotment: 2048 MiB` match the footprint that let
the sibling pre-exceptions case complete. The inherited 6 CPU / 8192 MiB request
failed with zero target telemetry, consistent with SMP scheduling/capacity
rather than an Agent crash.

## Traffic

| generator | rate | names | path exercised |
|---|---|---|---|
| `background` | 16 MiB/s | random, as `quality_gate_metrics_logs` | evaluate, forward |
| `prefix_hit` | 1.5 MiB/s | `…bench.prefix.<idx><pad>.hit` | `rules` + exception miss, dropped |
| `exact_hit` | 1.5 MiB/s | `…bench.exact.<idx><pad>` | exact arm, dropped |
| `miss` | 1 MiB/s | `…bench.nomatch.<idx><pad>` | all arms, forwarded |

All wire names are 88 characters, so hit/miss differences are not name-length
artifacts. Prefix entries are 84 characters; `prefix_hit` appends `.hit`, so
matches come only from prefix matching, not equality.

## Reading results

The baseline image does not support `metric_filterlist_prefix`, so it filters
only the exact half. Deltas are "feature off vs on", not a like-for-like
regression.

## Regenerating

```bash
cd experiments/regression/agent/metric_filterlist_scaling
python3 scripts/generate_filterlist_cases.py --mixed
# copy cases/quality_gate_metric_filterlist_mixed_10k here and drop quality_gate_
python3 test/regression/scripts/add_filterlist_exceptions.py
```

After regeneration, re-apply the traffic cut, prefix-count quartering, resource
allotment, and `metric_filterlist`/`metric_filterlist_prefix` split above.

## Local run

```bash
smp local-run --experiment-dir test/regression \
  --case metric_filterlist_mixed_10k \
  --target-image <image built from this branch>
```
