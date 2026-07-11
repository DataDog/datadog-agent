---
name: locate-config-setting
description: Find where an Agent config setting or section is defined in the YAML schemas, and print its schema node, using `dda inv schema.locate`
allowed-tools: Bash, Read
argument-hint: "[setting.path]"
model: sonnet
---

Locate a Datadog Agent configuration setting or section in the schema source and
print its definition plus the exact file and line where it lives.

Use this whenever you need to answer "where/how is config key `X` defined?" — its
type, default, env vars, description, or which schema file to edit. It is much
faster and more reliable than grepping the ~20k lines of schema YAML by hand,
because the schema is **split across files** (`core_schema.yaml` plus per-section
sub-files referenced via `$ref`), so a naive grep often points at the wrong file.

## Command

```bash
dda inv -- schema.locate <setting.path>
```

`<setting.path>` is the **dotted logical path** exactly as a user writes it in
`datadog.yaml` / `system-probe.yaml` — e.g. `api_key`, `proxy.https`,
`apm_config.enabled`. (Use `--` so invoke treats the path as a positional arg.)

It can also be a **pattern**: any argument containing a character outside
`[A-Za-z0-9_.]` (e.g. `*`, `$`, `[`) is matched against *every* full dotted path
in the schema instead of looked up exactly. The pattern is treated as a regular
expression (`re.search`); if it isn't valid regex it falls back to shell-style
glob (`fnmatch`). So `'*enabled'` lists every setting whose path ends with
`enabled`. Pattern matches are printed as a compact, sorted
`[<schema>] <path>  ->  <file>:<line>` list (one line per match) rather than full
node blocks; pass `--json` for the full `{schema, path, file, line, node}` array.

### Flags

| Flag | Effect |
|---|---|
| `--target core` / `--target system-probe` | Restrict to one schema (default: search **both**). |
| `--json` | Emit a JSON array of `{schema, path, file, line, node}` instead of human text. Use this when you want to parse the result programmatically. |

## What it prints

- **Setting (leaf):** the full schema node — `node_type`, `type`, `default`,
  `env_vars`, `visibility`, `description`, `tags`.
- **Section:** the section's own metadata, with `properties` collapsed to the
  **sorted list of immediate child key names** (so a big section like `apm_config`
  doesn't dump thousands of lines). Drill into a child to see its full node.
- A clickable location header per match: `[<schema>] <file>:<line>`.
- If a key exists in **both** schemas (e.g. `log_level`), it prints one block per
  schema, labeled `[core]` and `[system-probe]`.

## Location semantics (important)

- A **top-level split section** (e.g. `apm_config`, `logs_config`) is reported at
  its `$ref:` line in `pkg/config/schema/yaml/core_schema.yaml` — that's where it's
  wired in.
- A setting **inside** a split section (e.g. `apm_config.enabled`) is reported in
  the **sub-file** (`pkg/config/schema/yaml/apm_config.yaml:<line>`) — that's where
  its real definition is, and the file you'd edit.
- A setting/section that lives inline in the top file (e.g. `api_key`,
  `proxy.https`) is reported directly in `core_schema.yaml`.

## Behavior notes

- Reads the YAML **source** under `pkg/config/schema/yaml/` — no build step or
  agent binary needed; works straight from a checkout. Line numbers are real.
- Only traverses named settings and sections (`properties`). Array `items` and
  `patternProperties` internals are out of scope.
- Not found → prints an error and exits non-zero (no fuzzy suggestions).

## Examples

```bash
# Top-level setting → core_schema.yaml + full node
dda inv -- schema.locate api_key

# Setting inside a split section → resolves into the sub-file
dda inv -- schema.locate apm_config.enabled

# Bare split section → $ref site in core_schema.yaml, child names only
dda inv -- schema.locate apm_config

# Restrict to one schema and get machine-readable output
dda inv -- schema.locate process_config.enabled --target core --json

# Pattern (glob): every setting whose full path ends with 'enabled'
dda inv -- schema.locate '*enabled'

# Pattern (regex): every path containing 'proxy' (a bare word like 'proxy' is an
# exact lookup, so add metacharacters to force a pattern/contains-match)
dda inv -- schema.locate '.*proxy'
```

## Related

- The task lives in `tasks/schema/locate.py` (registered in `tasks/schema/__init__.py`).
- Schema source: `pkg/config/schema/yaml/`. To regenerate it, see `dda inv schema.generate`.
- To **add** a new config field (not just locate one), use the `create-config-field` skill.
