# CLI Commands (`dda inv schema.*`)

The schema tooling is exposed through a set of `dda inv schema.*` invoke tasks
(defined in `tasks/schema/`). This page documents each command, its arguments,
and when you would run it.

These commands are aimed at **Agent developers** and **AI tooling running on the
repository**. Most of them run automatically as part of the build and CI
pipelines, so you rarely need to invoke them by hand — but they are useful when
adding or editing settings, debugging schema problems, or regenerating output
locally.

## How the commands fit together

## `schema.generate`

Generate the enriched schema files for the core Agent and system-probe.

```bash
dda inv schema.generate --agent-bin=./bin/agent/agent
```

The command:

1. Runs the Agent binary (`createschema`) to produce the base schemas for both
   the `core` and `system-probe` targets.
2. Enriches them with documentation pulled from `pkg/config/config_template.yaml`
   and `pkg/config/system-probe_template.yaml`.
3. Applies OS-specific fixes and code-extracted comments.
4. Writes the result to `pkg/config/schema/yaml/` — large top-level sections of
   the core schema are split into sibling `<section>.yaml` files referenced via
   `$ref`.

| Argument | Required | Default | Description |
| --- | --- | --- | --- |
| `--agent-bin` | yes | — | Path to a built Agent binary. Build one first with `dda inv agent.build`. |
| `--output-dir` | no | `pkg/config/schema/yaml` | Directory where the schema files are written. |

!!! warning "Build the Agent first"
    `schema.generate` runs the Agent binary to extract the live configuration.
    If the binary is missing it exits with an error pointing you to
    `dda inv agent.build`. Rebuild after changing any `pkg/config/setup` code so
    the schema reflects your changes.

---

## `schema.lint`

Validate the schema against the schema quality rules and exit
non-zero on any violation. This is the check enforced in CI
(`lint_config_schema`).

```bash
dda inv schema.lint
```

| Argument | Required | Default | Description |
| --- | --- | --- | --- |
| `--schema-dir` | no | `pkg/config/schema/yaml` | Directory containing the schema files to lint. |
| `--exceptions-file` | no | `tasks/schema/lint_exceptions.yaml` | YAML file listing paths exempt from specific checks. |

---

## `schema.template`

Render a single config template (one build type, one OS) from the schema file. Useful for inspecting what a setting will
look like in the generated example without running the whole build.

```bash
dda inv schema.template \
  --schema=./pkg/config/schema/yaml/core_schema.yaml \
  --build-type=agent-py3 \
  --os-target=linux \
  --output=/tmp/datadog.yaml.example
```

| Argument | Required | Description |
| --- | --- | --- |
| `--schema` | yes | Path to the enriched schema YAML file. |
| `--build-type` | yes | One of: `agent-py3`, `iot-agent`, `system-probe`, `dogstatsd`, `dca`, `dcacf`. |
| `--os-target` | yes | One of: `windows`, `linux`, `darwin`. |
| `--output` | yes | Path to write the rendered template. |

Only nodes with `visibility: public` are rendered. The build type selects which
template sections are included; the OS target controls which platform-specific
defaults and `platform_only` settings are emitted.

---

## `schema.template-all`

Render every combination of build type × OS target from the core and
system-probe schemas at once. This is what the build pipeline uses to keep the
shipped `*.yaml.example` files in sync.

```bash
dda inv schema.template-all \
  --core-schema=./pkg/config/schema/yaml/core_schema.yaml \
  --sysprobe-schema=./pkg/config/schema/yaml/system-probe_schema.yaml \
  --output-dir=/tmp/templates
```

| Argument | Required | Description |
| --- | --- | --- |
| `--core-schema` | yes | Path to the enriched core Agent schema. |
| `--sysprobe-schema` | yes | Path to the enriched system-probe schema. |
| `--output-dir` | yes | Directory where `<build_type>_<os_target>.yaml` files are written. |

---

## Typical workflows

**I edited a setting in `pkg/config/setup` and want the schema to reflect it:**

```bash
dda inv agent.build                                  # rebuild the binary
dda inv schema.generate --agent-bin=./bin/agent/agent  # regenerate the schema
dda inv schema.lint                                  # validate it
```

**I want to preview the generated example for one platform:**

```bash
dda inv schema.template \
  --schema=./pkg/config/schema/yaml/core_schema.yaml \
  --build-type=agent-py3 --os-target=linux --output=/tmp/datadog.yaml.example
```

## See also

- [Introduction](index.md) — what the schema is and why it exists.
- [Keyword Reference](keywords.md) — the keywords these commands read and write.
- [FAQ](faq.md) — adding, documenting, and publishing settings.
