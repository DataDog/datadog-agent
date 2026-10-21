# Resource fixture

The Linux stage profiler consumes a directory with two files:

- `series.json`: an array of 258 objects with `name` (string) and `tags` (array of
  strings). Array position identifies the source series.
- `points.bin`: consecutive 20-byte little-endian records: signed int64 timestamp
  in seconds, float64 value, and uint32 index into `series.json`.

Points must be in nondecreasing timestamp order and reference valid series.
The recorded fixture has 681,271 points and covers 11 hours. Counters were already
converted to rates. Each source series is replicated until there are 1,200 series,
with a replica tag distinguishing identities. Storage retains 120 points per series;
the detector advances once per data second. This is accelerated replay, not an
11-hour wall-clock test.

The stages end at elapsed seconds 60, 17,940, 18,060, 21,540, 21,660 and 39,600.
The short qualification and activation windows straddle the five- and six-hour
boundaries, allowing for series start offsets. Stage snapshots force GC outside
the timed stage. Background GC samples may lack stage labels; do not assign them
to a particular seasonal operation based only on a filtered pprof percentage.

The fixture is external because it contains recorded telemetry. The checked-in
results contain aggregate measurements only. Use your own appropriately prepared
fixture for new workloads; expect different qualification counts and costs.
