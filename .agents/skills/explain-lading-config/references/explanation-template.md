# Explanation template

Using the lading source as ground truth, provide a structured explanation
with the sections below.

## Generator summary

For each generator entry, include the fields relevant to that generator type:

- Type and protocol/variant
- Target endpoint (`addr`, `path`, `target_uri`) — if network-based
- Throughput (`bytes_per_second`) — if network-based
- Parallel connections or sender count — if applicable
- Payload characteristics (contexts, tag counts, metric type weights, body
  sizes, `kind_weights`) — if applicable
- Operation rates (e.g. `open_per_second`, `rename_per_second`) — for
  filesystem generators
- Container churn rate = `number_of_containers / max_lifetime_seconds` — for
  container generators (report containers recycled per second, since that's
  what the agent sees)
- Default values for any omitted fields. **Always resolve the default to a
  concrete value**, not just the function name — the user wants to know what
  actually runs. Follow `#[serde(default = "default_foo")]` → the body of
  `fn default_foo()`, or the `impl Default` block, and report the literal
  (e.g. `block_cache_method: Fixed (via lading_payload::block::default_cache_method)`).
  If the default is a nested struct with its own defaults, recurse one level;
  cite further nested defaults by path rather than expanding the whole tree.
- Cache config (`maximum_prebuild_cache_size_bytes`, `block_cache_method`) —
  if applicable

Skip fields that don't exist on the generator type. The Rust `Config` struct
is authoritative for which fields exist.

## Aggregate load

Summarize total load across all generators. Pick the right unit for the
generator type — don't invent a bytes/s number for non-network load:

- **Network** (`http`, `tcp`, `udp`, `unix_*`, `grpc`, `splunk_hec`): sum
  `bytes_per_second` across generators.
- **Filesystem** (`file_tree`, `file_gen`): report operation rates
  (`open_per_second`, `rename_per_second`) or the load profile.
- **Container** (`container`): report the churn rate
  (`number_of_containers / max_lifetime_seconds` containers recycled per
  second); throughput isn't meaningful.
- **Mixed**: report each dimension separately.

## Blackhole sinks

What endpoints absorb target output, any simulated latency.

## Target metrics

What telemetry is scraped from the target (if configured). Per scraper: type
(`prometheus`, `expvar`, …), URI, and any tags (e.g. `sub_agent`).
**Do not enumerate large var lists verbatim** — when `vars:` has many entries
(common for `expvar`), summarize by category (forwarder, serializer, writers,
`memstats/*`, etc.) and cite the line range for follow-up. A config with no
`generator` but heavy `target_metrics` is typically an idle-baseline
experiment measuring the agent's self-cost.

## Source references

Cite the specific lading source files read, with relative paths from
`~/dd/lading/`, so the user can dig deeper.
