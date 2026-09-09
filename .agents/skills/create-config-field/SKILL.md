---
name: create-config-field
description: Add a new configuration field to the Datadog Agent (datadog.yaml) by declaring it in the config schema
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, AskUserQuestion
argument-hint: "[config.key.name]"
model: sonnet
---

Add a new configuration setting to a Datadog Agent config file by declaring it in
the **configuration schema**.

The schema is the single source of truth for every Agent setting: its type,
default, documentation, environment variables, validation rules and visibility
all live in one YAML node. Do **not** add `BindEnvAndSetDefault` calls by hand —
the `pkg/config/setup/*_settings.go` files are generated from the schema, and so
are `datadog.yaml.example` / `system-probe.yaml.example`, the JSON Schema
published to SchemaStore, and the runtime config validation.

Full reference: `docs/public/agent-schema/` — [index](../../../docs/public/agent-schema/index.md),
[keywords](../../../docs/public/agent-schema/keywords.md),
[examples](../../../docs/public/agent-schema/examples.md),
[cli](../../../docs/public/agent-schema/cli.md),
[faq](../../../docs/public/agent-schema/faq.md).

## Where settings live

One schema per config file, all under `pkg/config/schema/yaml/`:

| Config file | Schema | `--schema` value |
|---|---|---|
| `datadog.yaml` | `pkg/config/schema/yaml/core_schema.yaml` | `core` (default) |
| `system-probe.yaml` | `pkg/config/schema/yaml/system-probe_schema.yaml` | `system-probe` |

Large top-level sections are **split into sibling files** referenced via `$ref`,
so the node for `apm_config.enabled` lives in `apm_config.yaml`, not in
`core_schema.yaml`.

Never grep for the file by hand — `dda inv -- schema.locate` resolves the `$ref`
for you (see Step 2).

## Instructions

### Step 1: Gather information from the user

Use `AskUserQuestion` to collect the following. If `$ARGUMENTS` provides the
setting path, skip that question.

1. **Target schema**: `datadog.yaml` (core) or `system-probe.yaml`?
2. **Setting path** (dot-separated, e.g. `my_feature.enabled`).
3. **Type**: `boolean`, `string`, `number`, `integer`, `array`, or `object`.  For `array`, also the element type
   (`items.type` is mandatory).
4. **Default**: a single `default`, or per-platform `platform_default`.
5. **Visibility**: `public` (appears in the generated `*.yaml.example` and public docs) or undocumented (the default —
   internal, no keyword emitted).
6. **Description**: mandatory for `public`, strongly encouraged otherwise. Written for **users**, not Agent developers.
   This should explain what the settings does and how to use it.
7. **Description for each ancestor section**: if a setting is public, each parent section must be public too with their
   own description. Ask for a **description per section newly made public**, separately from the setting's. Never reuse
   or copy the setting's description into its parent section — a section describes what the group of settings is *for*,
   a setting describes its own value.
8. **Comment**: an optional description aimed at developers.

### Step 2: Check the setting does not already exist

```bash
dda inv -- schema.locate my_feature.enabled          # exact path
dda inv -- schema.locate '.*my_feature'              # pattern (regex/glob)
```

This also tells you which file to edit. See the `locate-config-setting` skill for
the full flag set.

### Step 3: Add the node to the schema

Preferred — the interactive wizard, which routes split sections to the right
sub-file, preserves the file's hand-curated ordering, makes ancestor sections
public when needed, and lints at the end:

```bash
dda inv schema.add-setting                       # core schema
dda inv schema.add-setting --schema=system-probe # system-probe schema
```

The wizard is interactive (it reads from stdin), so when you cannot drive a TTY,
edit the YAML directly instead. Read a neighbouring node first and match its
style:

```yaml
my_feature:
  node_type: section
  type: object
  visibility: public
  description: Configuration for my feature.
  properties:

    enabled:
      node_type: setting
      type: boolean
      default: false
      description: Enables my feature.
      visibility: public
```

Rules that the linter enforces:

- Every node needs `node_type: section` or `node_type: setting`.
- Every setting needs a `type` and exactly one of `default` / `platform_default`.
- `platform_default` must cover every platform — list `linux`, `windows`,
  `darwin`, `aix` explicitly, or add an `other` catch-all. `container` /
  `fargate` are optional and fall back to `linux` then `other`.
- An `array` setting must declare `items.type`.
- A `public` node needs a non-empty `description`, **and every ancestor section
  must also be `public` with a description** — its own description, gathered in
  Step 1, not a copy of the child setting's.
- A section needs at least one child; a public section needs at least one direct
  public child.
- Set `node_type: setting` — not `section` — when the value *is* an object
  (e.g. `docker_labels_as_tags`: `type: object`, `default: {}`). A section is only
  for grouping child settings.

**Placement matters**: the generated config examples follow schema order, so
insert the node where it belongs logically, not at the end of the file.

### Step 4: Lint and preview

```bash
dda inv schema.lint
```

## Keyword quick reference

Full up-to-date details in `docs/public/agent-schema/keywords.md`.

| Keyword | Where | Notes |
|---|---|---|
| `node_type` | all | `section` or `setting`. Mandatory. |
| `type` | setting | `boolean`, `number`, `integer`, `string`, `array`, `object`. |
| `default` | setting | Must match `type`. Mutually exclusive with `platform_default`. |
| `platform_default` | setting | Keys: `linux`, `windows`, `darwin`, `aix`, `container`, `fargate`, `other`. |
| `description` | all | Mandatory when `public`. Use the `\|` block scalar for multi-line. |
| `visibility` | all | `public` or `undocumented` (default). |
| `env_vars` | setting | Overrides the derived `DD_*` name; first match wins. |
| `env_parser` | setting | `comma_separated`, `space_separated`, `json`. Needed for complex types. |
| `sensitive` | setting | Scrubs the value from logs, flare and Fleet Automation. |
| `items` | setting | Mandatory for `array`. |
| `properties` | section / object setting | Child settings on a section; value sub-schema on an object setting. |
| `title` | section | Banner heading in the generated example. |
| `comment` | all | Developer-only note; never rendered to users. |
| `example` | setting | Overrides the value shown on the rendered example line. |
| `tags` | all | See below. |

**Relative defaults**: use `${conf_path}`, `${install_path}`, `${log_path}`,
`${run_path}` with `/` separators rather than hardcoding per-OS paths — e.g.
`default: "${conf_path}/conf.d"`. Valid in `default` and `platform_default`.

### Tags

Three are usable for new settings:

- `template_section:<name>` — selects which config-example flavors include the
  setting. Omit it and the setting renders in every build type.
- `platform_only:<os>[,<os>]` — restricts the setting to the listed OSes
  (`windows`, `linux`, `darwin`); it is dropped from the examples generated for
  any other `--os-target`.
- `generate_const:<Name>` — emits a Go constant `<Name>` in `pkg/config/setup`
  holding this setting's default. Use it instead of hardcoding a default (port,
  timeout, path) in Go code, so the two can never drift.

`golang_type:*`, `no-env` and the legacy `env_parser` values
(`comma_and_space_separated`, `traces_span`, `csv_comma_separated`,
`comma_then_space_separated`, `json_list_or_*`) exist only to support existing
settings — do not use them for new ones.

## Reading the setting from Go

```go
pkgconfigsetup.Datadog().GetBool("my_feature.enabled")
pkgconfigsetup.SystemProbe().GetInt("system_probe_config.max_conns")
```

In components, prefer the injected `config.Component` over the global accessor.

## Related

- `locate-config-setting` — find where an existing setting is defined.
- `create-release-note` — a user-visible new setting needs a reno note.

## Usage

- `/create-config-field` — interactive: prompts for all details
- `/create-config-field my_feature.enabled` — pre-fills the setting path
