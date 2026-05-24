---
name: explain-lading-config
description: Explains a lading.yaml config file from the regression test suite, using the lading Rust source as ground truth for field meanings and defaults.
user_invocable: true
argument-hint: "[experiment name]"
---

# explain-lading-config

Explain what a lading regression test config does, grounded in lading source code.

## Quick Start

```bash
# 1. Verify the lading checkout exists and is on a known branch
bash .claude/skills/explain-lading-config/scripts/validate-lading-checkout.sh

# 2. Resolve $ARGUMENTS to a lading.yaml path (exact/substring/glob/path)
bash .claude/skills/explain-lading-config/scripts/resolve-lading-config.sh "$ARGUMENTS"

# 3. Read the resolved file, then grep source structs in parallel:
grep -n 'pub struct Config\|pub enum Config\|pub enum\|fn default_\|impl Default for\|#\[serde(default' \
    ~/dd/lading/lading/src/generator/<type>.rs
```

Then explain with defaults resolved to concrete values (not function names).
Full workflow below.

## Step 1: Validate lading checkout

Run `.claude/skills/explain-lading-config/scripts/validate-lading-checkout.sh`.

- Exit 0: script prints the current branch on stdout. If it is not `main`, warn
  the user that explanations are grounded in a non-main branch, then continue.
- Exit non-zero: the script prints a suggested `git clone` command on stderr.
  Relay that to the user and stop.

Override the checkout location with `LADING_DIR` if needed.

## Step 2: Determine target file

Use `.claude/skills/explain-lading-config/scripts/resolve-lading-config.sh` to
avoid ad-hoc matching. The script enumerates experiments under
`test/regression/cases/` (active) and `test/regression/x-disabled-cases/`
(disabled). Each experiment is a `<case>/lading/lading.yaml` addressed by its
case-directory name; disabled rows are flagged with a trailing `(disabled)`
column in the listing. `ebpf/cases/` (split-mode) and
`ebpf/config-only/cases/` are intentionally out of scope; if a user asks about
one, tell them this skill doesn't cover it yet.

The script handles path-like inputs, substring case names, and shell
globs (`*`, `?`).

**If `$ARGUMENTS` is provided:** run `resolve-lading-config.sh "$ARGUMENTS"`.
- Exit 0: stdout is the resolved absolute path; read it.
- Exit 3 (ambiguous): stderr lists candidates.
  - **≤ 4 candidates:** use `AskUserQuestion` to pick one, then read that
    path.
  - **> 4 candidates** (a broad substring like `i` can match 20+): do not
    try to force them into `AskUserQuestion`. Print the experiment names
    as a short bulleted list and ask the user to narrow the query and
    re-invoke `/explain-lading-config <name>`.
- Exit 2 (not found): stderr may include "did you mean?" suggestions — if
  present, offer the suggestions to the user via `AskUserQuestion` (up to
  4 options) or as a short list; if not, relay the error and stop.
- Exit 4 (wrong repo): the script is being run from outside the agent repo.
  Relay the error verbatim and stop — the user needs to `cd` into the repo.

**If the resolved path contains `/x-disabled-cases/`**, flag this explicitly
in the explanation — the experiment exists on disk but is not currently
executed by SMP. Otherwise a user may assume it's live.

**Reading very large configs:** multi-sender configs (e.g.
`uds_dogstatsd_20mb_12k_contexts_20_senders`, ~870 lines) are usually
block-copies of one template with a few fields varying (typically only
`seed`). Before a full `Read`, check size and duplication:

```bash
wc -l <path>                                    # scale check
grep -c '^  - ' <path>                          # top-level list entries
yq '.generator | length' <path> 2>/dev/null     # if yq is present
```

For highly-duplicated configs, `Read` only the first block (plus the
blackhole/target_metrics sections) and report the generator as
"N identical copies, seed differs" instead of walking every block. Spot-
check one later block to confirm uniformity.

**If `$ARGUMENTS` is omitted:** run `resolve-lading-config.sh` with no
argument. It emits `<experiment>\t<path>` lines for every discovered config.

Print the experiment names as a plain bulleted list to the user (preserving
the `(disabled)` markers) and ask them to type the name (or re-invoke the
skill with `/explain-lading-config <name>`).

## Step 3: Read the lading codebase for context

Before explaining, read the relevant source files from the lading checkout
to understand config fields. Do NOT rely on embedded knowledge — always
read the source. Source paths below use `~/dd/lading/` for readability;
substitute `$LADING_DIR` (from Step 1) if the user overrode the location.

If an expected source file doesn't exist (lading may have renamed or
restructured), fall back to `grep -rln 'pub struct Config' ~/dd/lading/lading/src/generator/` (
or `blackhole/`, `target_metrics/`) to locate the current file, then proceed as normal.
Mention the rename in the explanation so the user knows the skill's default paths are out of date.

First, parse the config to see which sections are populated (`generator`,
`blackhole`, `target_metrics`). Only read source files for sections that
actually exist. In particular: **if `generator: []`, skip the generator
source reads entirely** — there is nothing to ground.

1. If `generator` has entries: read `~/dd/lading/lading/src/generator.rs` to
   identify generator types, then for each type used read
   `~/dd/lading/lading/src/generator/<type>.rs` (config struct, field
   meanings, defaults).
2. If a payload variant is referenced (e.g. `dogstatsd`,
   `opentelemetry_metrics`), find its module. The variant-to-module
   mapping lives in `~/dd/lading/lading_payload/src/lib.rs` — grep for
   the variant's PascalCase enum name (e.g. `OpentelemetryMetrics`) and
   follow the `crate::…` path it points to. Common mappings:
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

### Reading strategy: grep before Read

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
Reserve full-file reads for cases where the struct body references types
you still need to understand.

## Step 4: Explain the config

Using the lading source as ground truth, provide a structured explanation:

### Generator summary

For each generator entry, include the fields relevant to that generator type:
- Type and protocol/variant
- Target endpoint (`addr`, `path`, `target_uri`) — if network-based
- Throughput (`bytes_per_second`) — if network-based
- Parallel connections or sender count — if applicable
- Payload characteristics (contexts, tag counts, metric type weights, body sizes, `kind_weights`) — if applicable
- Operation rates (e.g. `open_per_second`, `rename_per_second`) — for filesystem generators
- Container churn rate = `number_of_containers / max_lifetime_seconds` — for container generators (report containers recycled per second, since that's what the agent sees)
- Default values for any omitted fields. **Always resolve the default to a
  concrete value**, not just the function name — the user wants to know
  what actually runs. Follow `#[serde(default = "default_foo")]` → the body
  of `fn default_foo()`, or the `impl Default` block, and report the literal
  (e.g. `block_cache_method: Fixed (via lading_payload::block::default_cache_method)`).
  If the default is a nested struct with its own defaults, recurse one level;
  cite further nested defaults by path rather than expanding the whole tree.
- Cache config (`maximum_prebuild_cache_size_bytes`, `block_cache_method`) — if applicable

Skip fields that don't exist on the generator type. The Rust `Config` struct is authoritative for which fields exist.

### Aggregate load

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

### Blackhole sinks

What endpoints absorb target output, any simulated latency.

### Target metrics

What telemetry is scraped from the target (if configured). Per scraper:
type (`prometheus`, `expvar`, …), URI, and any tags (e.g. `sub_agent`).
**Do not enumerate large var lists verbatim** — when `vars:` has many
entries (common for `expvar`), summarize by category (forwarder,
serializer, writers, `memstats/*`, etc.) and cite the line range for
follow-up. A config with no `generator` but heavy `target_metrics` is
typically an idle-baseline experiment measuring the agent's self-cost.

### Source references

Cite the specific lading source files read, with relative paths from `~/dd/lading/`, so the user can dig deeper.
