---
name: explain-lading-config
description: Explains a lading.yaml config file from the regression test suite, using the lading Rust source as ground truth for field meanings and defaults.
user_invocable: true
---

# explain-lading-config

Explain what a lading regression test config does, grounded in lading source code.

## Step 1: Validate lading checkout

Check that `~/dd/lading` exists by running `ls ~/dd/lading`. If the directory does not exist (ls fails), stop and ask the user to clone it:

```
git clone git@github.com:DataDog/lading.git ~/dd/lading
```

If it exists, run `git -C ~/dd/lading branch --show-current`. If not `main`, warn the user but continue.

## Step 2: Determine target file

**If `$ARGUMENTS` is provided:** it may be a full file path or a case/experiment name.
- If it contains `/` or ends in `.yaml`, treat as a file path and read it directly.
- Otherwise, treat as a case name: glob for `**/lading.yaml` and find the match where the case directory name contains `$ARGUMENTS`. If multiple matches, list them and ask. If one match, read it.

**If `$ARGUMENTS` is omitted:**
1. Glob for `**/lading.yaml` across the repository.
2. List all found configs, showing the experiment/case name extracted from the path (the directory two levels above `lading/lading.yaml`).
3. Use `AskUserQuestion` to ask which experiment to explain.
4. Read the selected file.

## Step 3: Read the lading codebase for context

Before explaining, read the relevant source files from `~/dd/lading/` to understand config fields. Do NOT rely on embedded knowledge — always read the source.

1. Read `~/dd/lading/lading/src/generator.rs` to identify generator types.
2. Parse the config to identify which generator type(s) it uses (e.g. `unix_datagram`, `http`, `tcp`).
3. For each generator type used, read:
   - `~/dd/lading/lading/src/generator/<type>.rs` — config struct, field meanings, defaults
4. If a payload variant is referenced (e.g. `dogstatsd`, `opentelemetry_metrics`), read:
   - `~/dd/lading/lading_payload/src/<variant>.rs` or the relevant module under `~/dd/lading/lading_payload/src/`
5. If blackholes are configured, read:
   - `~/dd/lading/lading/src/blackhole.rs` — blackhole enum
   - `~/dd/lading/lading/src/blackhole/<type>.rs` — per-blackhole config structs

Read these files in parallel where possible.

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
- Default values for any omitted fields (cite the Rust struct and its `#[serde(default)]` or `Default` impl)
- Cache config (`maximum_prebuild_cache_size_bytes`, `block_cache_method`) — if applicable

Skip fields that don't exist on the generator type. The Rust `Config` struct is authoritative for which fields exist.

### Aggregate load

Summarize total load across all generators. For network generators, sum `bytes_per_second`. For filesystem generators, describe operation rates. For mixed configs, report both.

### Blackhole sinks

What endpoints absorb target output, any simulated latency.

### Target metrics

What telemetry is scraped from the target (if configured).

### Source references

Cite the specific lading source files read, with relative paths from `~/dd/lading/`, so the user can dig deeper.

## Step 5: Offer follow-up

After explaining, offer to:
- Show the relevant Rust struct definitions from `~/dd/lading/` for any type used
- Compare this config with another experiment's config
