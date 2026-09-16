# Reading the lading source

Source paths below use `~/dd/lading/` for readability; substitute `$LADING_DIR`
(from the checkout-validation step) if the user overrode the location.

Do NOT rely on embedded knowledge — always read the source.

If an expected source file doesn't exist (lading may have renamed or
restructured), fall back to `grep -rln 'pub struct Config' ~/dd/lading/lading/src/generator/`
(or `blackhole/`, `target_metrics/`) to locate the current file, then proceed as
normal. Mention the rename in the explanation so the user knows the skill's
default paths are out of date.

First, parse the config to see which sections are populated (`generator`,
`blackhole`, `target_metrics`). Only read source files for sections that
actually exist. In particular: **if `generator: []`, skip the generator
source reads entirely** — there is nothing to ground.

## What to read per section

1. If `generator` has entries: read `~/dd/lading/lading/src/generator.rs` to
   identify generator types, then for each type used read
   `~/dd/lading/lading/src/generator/<type>.rs` (config struct, field
   meanings, defaults).
2. If a payload variant is referenced (e.g. `dogstatsd`,
   `opentelemetry_metrics`), find its module. The variant-to-module mapping
   lives in `~/dd/lading/lading_payload/src/lib.rs` — grep for the variant's
   PascalCase enum name (e.g. `OpentelemetryMetrics`) and follow the
   `crate::…` path it points to. Common mappings:
   - `dogstatsd` → `lading_payload/src/dogstatsd.rs`
   - `opentelemetry_metrics` → `lading_payload/src/opentelemetry/metric.rs`
   - `opentelemetry_logs` → `lading_payload/src/opentelemetry/log.rs`
   - `datadog_logs` → `lading_payload/src/datadog_logs.rs`

   **Variant serialization forms:**
   - `variant: "syslog5424"` (plain string) — the enum variant carries no
     config fields (unit/empty struct). There are no knobs to explain; the
     module itself encodes all behaviour.
   - `variant: { opentelemetry_metrics: {} }` (mapping with empty body) —
     the variant has a `Config` struct and is using `Config::default()`.
     Follow `impl Default for Config` and any nested `Default` impls.
   - `variant: { dogstatsd: { contexts: …, kind_weights: … } }` — explicit
     field overrides; report them alongside the defaults for any omitted
     sibling fields.
3. If blackholes are configured, read:
   - `~/dd/lading/lading/src/blackhole.rs` — blackhole enum
   - `~/dd/lading/lading/src/blackhole/<type>.rs` — per-blackhole config structs
4. If `target_metrics` has entries, read:
   - `~/dd/lading/lading/src/target_metrics/prometheus.rs` — `uri`, `metrics`, `tags`
   - `~/dd/lading/lading/src/target_metrics/expvar.rs` — `uri`, `vars`, `tags`

   (Other scrapers live alongside in `target_metrics/`.)

Read these files in parallel where possible.

## Reading strategy: grep before Read

Lading's source files can be hundreds of lines. To ground defaults without
reading whole files, use this invariant: every default in lading follows the
pattern `#[serde(default = "default_foo")]` → `fn default_foo() -> T { ... }`.
`Default` impls (for payload types like `KindWeights`, `MetricWeights`) are
adjacent to their struct definitions.

Efficient approach for a generator/blackhole/target_metrics type:

```bash
# Locate top-level Config + all named defaults + nested enum variants in one pass
grep -n 'pub struct Config\|pub enum Config\|pub enum\|fn default_\|impl Default for\|#\[serde(default' \
    ~/dd/lading/lading/src/generator/<type>.rs
```

Include `pub enum` — some generators' top-level `Config` is an enum
(e.g. `file_gen::Config` discriminates on `traditional` / `logrotate` /
`logrotate_fs`), and several structs hold nested enums (`http::Method`,
`blackhole::datadog::Variant`) that the YAML maps into with nested keys.

Then `Read` only the line ranges that matter (struct/enum body + default fns).
Reserve full-file reads for cases where the struct body references types you
still need to understand.
