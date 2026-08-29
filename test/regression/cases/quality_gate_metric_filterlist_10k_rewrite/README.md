# quality_gate_metric_filterlist_10k_rewrite

Worst-case companion to `quality_gate_metric_filterlist_10k`. Everything is held
identical — 10,000 filter list entries, three generators at 18/1/1 MiB/s, 4 CPUs,
2 GiB — except that **every metric name sent needs normalising**.

## Why it exists

`Matcher.Test` (`pkg/util/metricname/matcher.go`) normalises the name it is given
so that filtering happens in the same name space the intake stores, which is the
space users see in Datadog. Names that are already normalised take an
allocation-free fast path; the rest are rewritten into a stack buffer.

The base case does **not** exercise the rewrite path much. Its background
generator, 90% of the traffic, uses lading's random names, and lading's
`RandomStringPool` alphabet is letters and digits only:

```
abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789
```

A normalised name must start with a letter, and 10 of those 62 characters are
digits, so only ~16% of generated names need rewriting. This case forces 100%.

## How

Every name carries `-` characters, which are not legal in a stored metric name.
The intake rewrites each `-` to `_`, so the name the Agent holds and the name
Datadog stores differ:

```
lading sends    smp-filterlist-bench-metric-aaaaabxxxx…   (88 chars, 4 hyphens)
Agent computes  smp_filterlist_bench_metric_aaaaabxxxx…
filter list has smp_filterlist_bench_metric_aaaaabxxxx…   <- the normalised form
```

Because the filter list holds the normalised spelling, this doubles as a
functional check: if normalisation were removed from `Test`, nothing would be
filtered and `dogstatsd__listener_filtered_points` would sit at zero.

Traffic mix by byte budget, same as the base case:

| generator | share | namespace | path exercised |
|---|---|---|---|
| background | 90% | `smp-filterlist-bench-bg-*` (13,520 names) | normalise, then **miss** — the dominant real cost |
| match | 5% | `smp-filterlist-bench-metric-*` (676) | normalise, then **hit** → dropped |
| nomatch | 5% | `smp-filterlist-bench-nomatch-*` (676) | normalise, then **miss**, same name length as match |

## Reading the results

Compare **CPU** against `quality_gate_metric_filterlist_10k`. The delta is the
cost of the rewrite path over the fast path, at 100% versus ~16% incidence.

Do **not** compare memory against the base case. All three generators here use
explicit `metric_names` pools, because lading's random alphabet cannot produce
`-`, so background contributes 13,520 distinct names instead of the base case's
`contexts: 1000..10000` random ones. That changes the aggregator's context count
independently of anything this case is trying to measure.

For reference, `BenchmarkMatcherTestPaths` in
`pkg/util/metricname/matcher_scaling_test.go` measures the same paths in
isolation, with fixture shape held constant (4096 probes, 88-char names, 10k
list):

```
already-normalized    146-170 ns/op   0 B/op   0 allocs/op
hyphenated            180-199 ns/op   0 B/op   0 allocs/op   <- this case
leading-digit         187-195 ns/op   0 B/op   0 allocs/op
trailing-hyphen       251-259 ns/op   0 B/op   0 allocs/op
```

So the rewrite path costs ~18% more than the fast path per lookup, and no
allocations either way — `Matcher.Test` normalises into a stack buffer. Filtering
is a small fraction of per-sample work, so expect the end-to-end CPU delta here
to be well under that 18%, and no memory delta attributable to allocation.

(`trailing-hyphen` is worse because the fast-path check has to scan the entire
name before rejecting it on the last byte, and the rewrite then rescans it. This
case's names fail on byte 4, so they are cheaper than that worst case.)

## Constraints baked into the fixtures

- **lading >= 0.32.0** is required for `{{a-z}}` pattern expansion, pinned in
  `../../config.yaml`. On 0.31.x the pattern is sent verbatim as a single literal
  metric name and nothing is filtered, silently.
- **Character ranges only, never numeric.** lading re-derives numeric padding
  from the parsed integers, so `{{000-499}}` expands to 3-digit values and would
  not match these 88-character names.
- The `-` characters **outside** `{{...}}` are data; the `-` **inside** is the
  range separator. This is the one thing not verified offline — confirm on the
  first run that `dogstatsd__listener_filtered_points` is non-zero, which proves
  lading emitted literal hyphens and the match corpus reached the filter.
- Fixtures are generated, not hand-authored. Verified before commit: all names
  exactly 88 characters, none already normalised, all 676 match names normalise
  into the filter list, zero overlap between the nomatch/background namespaces
  and the list after normalisation, all expansions under lading's 15,000 cap, and
  no entry ending in a histogram aggregate suffix (so the histogram subset stays
  empty and all filtering happens on the ingest path).
