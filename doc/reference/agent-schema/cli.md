# Schema CLI commands

The schema tooling is exposed through a set of `dda inv schema.*` invoke tasks (defined in `tasks/schema/`). This page documents each command, its arguments, and when you would run it.

These commands are aimed at **Agent developers** and **AI tooling running on the repository**. Most of them run automatically as part of the build and CI pipelines, so you rarely need to invoke them by hand — but they are useful when adding or editing settings, debugging schema problems, or regenerating output locally.

See the [schema architecture](../../architecture/agent-schema/index.md#how-the-commands-fit-together) for how these commands fit together and the [schema workflows](../../how-to/agent-schema/workflows.md) for common tasks.

## `schema.add-setting`

Interactive wizard to add a new setting to the schema. It prompts for the setting name (dotted path), type, default, visibility, and description, then inserts the node into the correct schema file under `pkg/config/schema/yaml/` (routing split sections such as `apm_config` to their sub-file automatically).

```bash
dda inv schema.add-setting
```

| Argument | Required | Default | Description |
| --- | --- | --- | --- |
| `--schema` | no | `core` | Which schema to target: `core` or `system-probe`. |

The wizard preserves the file's existing ordering, asks for the element type of `array` settings, and — for `public` settings — makes every parent section public with a description. It runs `schema.lint` at the end so any remaining problems are visible.

---

## `schema.lint`

Validate the schema against the schema quality rules and exit non-zero on any violation.

```bash
dda inv schema.lint
```

| Argument | Required | Default | Description |
| --- | --- | --- | --- |
| `--schema-dir` | no | `pkg/config/schema/yaml` | Directory containing the schema files to lint. |
| `--exceptions-file` | no | `tasks/schema/lint_exceptions.yaml` | YAML file listing paths exempt from specific checks. |

---

## `schema.template`

Render a single config template (one build type, one OS) from the schema file. Useful for inspecting what a setting will look like in the generated example without running the whole build.

```bash
dda inv schema.template \
  --schema=./pkg/config/schema/yaml/core_schema.yaml \
  --build-type=datadog-agent \
  --os-target=linux \
  --output=/tmp/datadog.yaml.example
```

| Argument | Required | Description |
| --- | --- | --- |
| `--schema` | yes | Path to the enriched schema YAML file. |
| `--build-type` | yes | One of: `datadog-agent`, `iot-agent`, `system-probe`, `dogstatsd`, `dca`, `dcacf`. |
| `--os-target` | yes | One of: `windows`, `linux`, `darwin`. |
| `--output` | yes | Path to write the rendered template. |

Only nodes with `visibility: public` are rendered. The build type selects which template sections are included; the OS target controls which platform-specific defaults and `platform_only` settings are emitted.

---

## `schema.locate`

Find where a setting or section is defined in the schema source and print its node plus the exact file and line.

```bash
dda inv -- schema.locate apm_config.enabled
```

| Argument | Required | Description |
| --- | --- | --- |
| `setting` | yes | A dotted config path (`api_key`, `proxy.https`, `apm_config.enabled`) **or** a pattern (see below). Positional — use `--` so invoke does not treat a leading `-` or `*` as a flag. |
| `--target` | no | Restrict the search to a single schema: `core` or `system-probe`. |
| `--json` | no | Emit a JSON array of `{schema, path, file, line, node}` instead of human-readable text. |

**Exact paths** print the full node with a `[<schema>] <file>:<line>` header. A setting inside a split section (e.g. `apm_config.enabled`) is reported in its sub-file (`pkg/config/schema/yaml/apm_config.yaml:<line>`) — the file you would edit. A bare split section is reported at its `$ref:` line in `core_schema.yaml`, with its `properties` collapsed to the sorted list of child key names.

**Patterns** — any argument containing a character outside `[A-Za-z0-9_.]` is matched against *every* full dotted path in the schema instead of looked up exactly. The pattern is treated as a regular expression (`re.search`); if it is not valid regex it falls back to shell-style glob (`fnmatch`). Pattern matches print as a compact, sorted `[<schema>] <path> -> <file>:<line>` list (one line per match); use `--json` for the full node array.

```bash
# Exact: setting inside a split section → resolves into the sub-file
dda inv -- schema.locate apm_config.enabled

# Pattern (glob): every setting whose full path ends with "enabled"
dda inv -- schema.locate '*enabled'

# Pattern (regex): every path under apm_config ending in "enabled", as JSON
dda inv -- schema.locate 'apm_config\..*enabled' --json
```

---

## See also

- Read the [schema overview](../../architecture/agent-schema/index.md) for the concepts behind the commands.
- Consult the [keyword reference](keywords.md) for the fields these commands read and write.
- Follow the [settings how-to](../../how-to/agent-schema/settings.md) to add, document, and publish settings.
