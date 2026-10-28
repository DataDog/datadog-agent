# Agent configuration schema

The Datadog Agent configuration schema is a YAML-based [JSON Schema](https://json-schema.org/) that centrally describes every configuration setting for the Agent. It is written in YAML for readability but is fully compatible with JSON Schema tooling.

## Why it exists

Previously, Agent configuration was defined through a combination of imperative Go code (`BindEnvAndSetDefault` calls in `pkg/config/setup`), YAML example files maintained by-hand, and scattered validation logic. This made it hard to understand what a setting does, what values it accepts, or what its default is — without reading source code.

The schema replaces this with a **single source of truth**. All information about a setting — its type, default value, documentation, environment variables, validation rules, and visibility — lives in one place.

## What it enables

- **Config validation without running the Agent** — any JSON Schema library can validate a customer's `datadog.yaml` against the schema.
- **IDE autocompletion** — the schema can be published to [SchemaStore](https://www.schemastore.org/) so editors like VS Code automatically validate and complete configuration files.
- **Automatic generation of `datadog.yaml.example` and `system-probe.yaml.example`** — the example file shipped with the Agent is generated directly from the schema, ensuring it stays in sync.
- **Type-safe code generation** — Go configuration code can be generated from the schema, removing the imperative setup code entirely.
- **Runtime validation**  — The user configuration can be validated at runtime by the Agent allowing automatic fallback to default upon an invalid value.

## Terminology

- **Path** — a string of terms separated by dots (`"."`) that represents a series of nodes in the config tree.
- **Setting** — a configurable value that can be located using a path (example `apm_config.enabled`).
- **Section** — a grouping of settings under a common name (example `apm_config`).
- **Object** — a data type of key-value pairs. Also called a map (in Go) or dict (in Python), but called an object in JSON Schema since it comes from JavaScript.
- **Default** — the value that will be retrieved for a given setting if none is specified by the user's file-based configuration or environment variable (or other source).

## Node types

The schema tree is composed of two types of nodes:

- **Setting nodes** represent individual settings. They have a type and a value. For example, `apm_config.enabled` is a setting node of type `boolean`.
- **Section nodes** represent a group of settings. Sections do not have values themselves; instead they group related setting nodes together. For example, `apm_config` is a section node containing `enabled` and many other setting nodes. Section nodes are identified by the `node_type: section` keyword.

The distinction matters when a setting's value is an object. For example, `docker_labels_as_tags` is a `type: object` setting node — its value is a dict of strings, but that dict is the setting's *value*, not a group of child settings. It is a setting node, and **not** a section node.

## One schema per configuration file

The Agent ships with multiple configuration files, each with its own schema:

| Config file | Schema file |
| --- | --- |
| `datadog.yaml` | <<<repo("pkg/config/schema/yaml/core_schema.yaml")>>> |
| `system-probe.yaml` | <<<repo("pkg/config/schema/yaml/system-probe_schema.yaml")>>> |

The main Agent schema covers metrics, logs, APM, processes, and other Agent features. The system-probe schema covers settings for its separate binary, including eBPF probes, network performance monitoring, and universal service monitoring.

Each schema generates its corresponding example configuration file. Only nodes with `visibility: public` appear in those examples. Both schemas use the same [keywords](../../reference/agent-schema/keywords.md).

## JSON Schema foundation

The Agent schema builds on [JSON Schema draft 2020-12](https://json-schema.org/). This documentation focuses on how keywords are used in the Agent schema specifically. For a general introduction to JSON Schema, see [Understanding JSON Schema](https://json-schema.org/understanding-json-schema).

## How the commands fit together

The schema files under `pkg/config/schema/yaml/` are the source of truth and are edited by hand (or through `schema.add-setting`). Everything else is derived from them:

- `schema.lint` validates the hand-written schema.
- `schema.codegen` generates the Go files that register the settings into `pkg/config/setup/generatedsettings_local`, which in-repo builds use. Bazel runs the same logic automatically via `//pkg/config/setup/generatedsettings_local:codegen_settings`.
- `schema.codegen-snapshot` refreshes `pkg/config/setup/generatedsettings`, the committed snapshot of those files that external consumers of `pkg/config/setup` (such as the OTel Collector) build against. See [the generated settings snapshot](#the-generated-settings-snapshot).
- `schema.template` renders the shipped `datadog.yaml.example` and `system-probe.yaml.example` files.
- `schema.produce-embedded` / `schema.compress` build the artifact embedded into the Agent binary; `schema.produce-jsonschema` builds the pure JSON Schema for external consumers.
- `schema.locate` finds where a setting is declared.

## The generated settings snapshot

The generated Go code lives in its own module, `github.com/DataDog/datadog-agent/pkg/config/setup/generatedsettings`, which `pkg/config/setup` imports. The module has two copies:

- `pkg/config/setup/generatedsettings_local` is the copy in-repo builds use: `go.work` and every in-repo `replace` directive point to it, through the `workspace_path` attribute of the module in `modules.yml`. Its generated `*.go` files are gitignored and regenerated by every `dda inv` build, test, lint, and tidy, and by Bazel, so they always match the schema. Its `go.mod` and `go.sum` are committed and maintained by `dda inv tidy`, along with a hand-written `doc.go` that keeps the package and its dependencies resolvable on a fresh checkout, before codegen has run.
- `pkg/config/setup/generatedsettings` is a committed snapshot. In-repo builds never use it: it is what external consumers download at a released tag, since they get no codegen step. It only changes when someone runs `dda inv schema.codegen-snapshot`.

Run `dda inv schema.codegen-snapshot` and commit the result when a schema change must reach external consumers, for example a new setting the OTel Collector needs. Otherwise there is no need to refresh it.

## Related documentation

- Read the [annotated examples](examples.md) to understand settings, sections, and nested values.
- Follow the [settings how-to](../../how-to/agent-schema/settings.md) to add, document, and publish settings.
- Use the [schema workflows](../../how-to/agent-schema/workflows.md) to validate changes, regenerate output, and locate settings.
- Consult the [keyword reference](../../reference/agent-schema/keywords.md) and [CLI reference](../../reference/agent-schema/cli.md) for supported fields and command arguments.
