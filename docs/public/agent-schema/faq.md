# FAQ

---

## How do I add a new setting?

1. **Determine the node type.** If the setting holds a value (string, number,
   boolean, list, or dict that *is* the value), it is a setting node. If it groups
   other settings, it is a section node.

2. **Choose the correct schema file.**
   - Settings for `datadog.yaml` go in `pkg/config/schema/core_schema.yaml`.
   - Settings for `system-probe.yaml` go in
     `pkg/config/schema/system-probe_schema.yaml`.

3. **Find the correct parent.** Locate the `properties` block of the parent
   section node under which the new setting belongs.

4. **Add the node** with the mandatory keywords:
   - Setting nodes require `type` and either `default` or `platform_default`.
   - Section nodes require `node_type: section` and `properties`.

5. **Add optional keywords** as appropriate:
   - `description` — strongly encouraged for all settings.
   - `env_vars` — list explicit env var names if the default
     (`DD_` + uppercase path) is wrong or if aliases are needed.
   - `visibility: public` — add when the setting should appear in
     configuration examples and public documentation (see
     [How do I make a setting public?](#how-do-i-make-a-setting-public)).

Minimal example:

```yaml
my_new_setting:
  type: boolean
  default: false
  description: Enables the new feature.
```

---

## How do I deprecate a setting?

> **Note:** [WIP] Automated solution coming soon.

---

## How do I make a setting public?

1. **Write a `description`** that explains what the setting does, what the default means, and any caveats. Every public
   setting must have a description. The description is aimed at users not Agent developers.

2. **Add `visibility: public`** to the setting or section node.

3. **Make every parent section public too.** A setting node with
   `visibility: public` nested inside a non-public section is invalid. Every
   section in the path from the root to the setting must also have
   `visibility: public` and a `description`. Without this the schema will be
   rejected.

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

The setting will appear in the configuration examples (ex: `datadog.yaml.example`) the next time the file is
regenerated.

---

## How do I document a public setting?

1. **Write a clear `description`** covering:
   - What the setting does.
   - What the default value means in practice.
   - Any important caveats or links to external documentation.

2. **Use the YAML `|` block scalar** for multi-line descriptions:

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

3. **Set `visibility: public`** to include the setting in generated output.

---

## How do I generate `datadog.yaml.example`?

The generation of `datadog.yaml.example` from the schema is wired into the
Agent's build pipeline and runs automatically. Developers do not normally need
to trigger it manually.

If you want to validate the generated example you can use `dda inv schema.template-all` command.

---

## What is the difference between `datadog.yaml.example` and `system-probe.yaml.example`?

Each configuration file has its own schema and its own generated example file:

- **`datadog.yaml.example`** is generated from
  `pkg/config/schema/core_schema.yaml` and covers the main Agent configuration —
  metrics, logs, APM, processes, and more.

- **`system-probe.yaml.example`** is generated from
  `pkg/config/schema/system-probe_schema.yaml` and covers only the settings
  specific to the system-probe component: eBPF probes, network performance
  monitoring, universal service monitoring, and related features.

Both files include only nodes with `visibility: public`. The separation mirrors
the fact that system-probe runs as a separate binary (`system-probe`) with its
own configuration file, independently of the main Agent process.
