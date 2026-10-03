# Add and document Agent settings

Read the [schema overview](../../architecture/agent-schema/index.md) for node types and the [keyword reference](../../reference/agent-schema/keywords.md) for supported fields. Use the [interactive wizard](../../reference/agent-schema/cli.md#schemaadd-setting) to add a setting, or follow the manual steps below.

## How do I add a new setting?

1. **Determine the node type.** If the setting holds a value (string, number, boolean, list, or dict that *is* the value), it is a setting node. If it groups other settings, it is a section node.

1. **Choose the correct schema file.**

    - Settings for `datadog.yaml` go in <<<repo("pkg/config/schema/yaml/core_schema.yaml")>>>.
    - Settings for `system-probe.yaml` go in <<<repo("pkg/config/schema/yaml/system-probe_schema.yaml")>>>.

1. **Find the correct parent.** Locate the `properties` block of the parent section node under which the new setting belongs.

1. **Add the node** with the mandatory keywords:

    - Setting nodes require `type` and either `default` or `platform_default`.
    - Section nodes require `node_type: section` and `properties`.

1. **Add optional keywords** as appropriate:

    - `description` — strongly encouraged for all settings.
    - `env_vars` — list explicit env var names if the default (`DD_` + uppercase path) is wrong or if aliases are needed.
    - `visibility: public` — add when the setting should appear in configuration examples and public documentation (see [How do I make a setting public?](#how-do-i-make-a-setting-public)).

Minimal example:

```yaml
my_new_setting:
  type: boolean
  default: false
  description: Enables the new feature.
```

---

## How do I deprecate a setting?

/// note
An automated setting deprecation workflow is still under development.
///

---

## How do I make a setting public?

1. **Write a `description`** that explains what the setting does, what the default means, and any caveats. Every public setting must have a description. The description is aimed at users not Agent developers.

1. **Add `visibility: public`** to the setting or section node.

1. **Make every parent section public too.** A setting node with `visibility: public` nested inside a non-public section is invalid. Every section in the path from the root to the setting must also have `visibility: public` and a `description`. Without this the schema will be rejected.

    ```yaml
    my_section:
      node_type: section
      visibility: public
      description: Configuration for my feature.
      properties:

        my_setting:
          node_type: setting
          type: boolean
          default: false
          description: Enables my feature.
          visibility: public
    ```

The setting will appear in the configuration examples (ex: `datadog.yaml.example`) the next time the file is regenerated.

---

## How do I document a public setting?

1. **Write a clear `description`** covering:

    - What the setting does.
    - What the default value means in practice.
    - Any important caveats or links to external documentation.

1. **Use the YAML `|` block scalar** for multi-line descriptions:

    ```yaml
    flush_timeout:
      type: number
      default: 5
      description: |
        Maximum time in seconds the Agent waits before flushing metrics
        to the intake. Lower values reduce latency but increase request
        volume. The default of 5 seconds is suitable for most deployments.
      visibility: public
    ```

1. **Set `visibility: public`** to include the setting in generated output.

## Validate and regenerate

Follow the [schema workflows](workflows.md) to validate your changes and regenerate Go code and configuration examples.
