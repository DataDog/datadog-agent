# Schema keywords

This page documents keywords supported by the Agent configuration schema. Keywords are grouped into two sections: standard JSON Schema keywords and Datadog Agent extensions.

---

## Standard JSON Schema keywords

The following keywords come from the [JSON Schema standard](https://json-schema.org/understanding-json-schema/keywords) and are understood by all JSON Schema tooling. This section describes how each keyword is used **specifically in the Agent schema**.

/// note
This reference covers the JSON Schema keywords most commonly used by the Agent.
///

### `type`

The data type of a setting node's value.

- **Mandatory:** yes, for all setting nodes. Not valid on section nodes.

Supported types:

| Type | Description |
| --- | --- |
| `boolean` | `true` or `false` |
| `number` | integer or floating-point number |
| `string` | text value |
| `array` | ordered list of values |
| `object` | key/value pairs |

Complex types are composed by combining these primitives — for example, an `array` whose `items` are `object`s.

```yaml
network_devices:
  node_type: setting
  type: array
  default: []
  items:       # describes types of the array elements, here all elements must be an object
    type: object
```

---

### `default`

The default value for a setting node.

- **Mandatory:** yes, for all setting nodes. Mutually exclusive with `platform_default` (one or the other must be specified).

The value must be a valid value of the setting (its `type`, `pattern`, `enum`, ...), as checked by the schema unit tests.

```yaml
check_runners:
  node_type: setting
  type: number
  default: 4
```

/// note
The JSON Schema published for external tools (`dda inv schema.produce-jsonschema`) declares no `default`, like most published schemas. Declared defaults make IDEs insert every default when completing a section, and a default depending on the platform can't be expressed in a schema serving every platform. Instead, the default of each **public** setting is added at the end of its `description`, where IDEs show it on hover, e.g. `Default: 4.` (in plain text: IDEs don't render descriptions as Markdown). Settings that aren't public get nothing added. The schema embedded in the Agent keeps `default`.
///

#### Relative defaults

Some default value are resolved at runtime base on the agent install parameter. Default value are often relative to the "install path", "log directory" ...

Those concept can be express using the `${}` notation with one of the following variables. The value below show the default but could be change by the users. For example, the default configuration directory for Windows is `c:/programdata/datadog` but can be changed at install time. The correct value will be use at startup by the Agent.

Path are all express using `/` and will be translated to the correct OS version.

Existing variables:

- Configuration directory (`${conf_path}`):

    - `windows`: `c:/programdata/datadog`
    - `linux`: `/etc/datadog-agent`
    - `darwin`: `/opt/datadog-agent/etc`
    - `aix`: `/etc/datadog-agent`

- Installation directory (`${install_path}`):

    - `windows`: `c:/program files/datadog/datadog agent`
    - `linux`: `/opt/datadog-agent`
    - `darwin`: `/opt/datadog-agent`
    - `aix`: `/opt/datadog-agent`

- Log directory (`${log_path}`):

    - `windows`: `c:/programdata/datadog/logs`
    - `linux`: `/var/log/datadog`
    - `darwin`: `/opt/datadog-agent/logs`
    - `aix`: `/var/log/datadog`

- Run directory, where the Agent starts from (`${run_path}`):

    - `windows`: `c:/programdata/datadog/run`
    - `linux`: `/opt/datadog-agent/run`
    - `darwin`: `/opt/datadog-agent/run`
    - `aix`: `/opt/datadog-agent/run`

The above variables are available for `default` and `platform_default` keywords.

Example:
```
confd_path:
  node_type: setting
  type: string
  default: "${conf_path}/conf.d"
```

---

### `description`

Human-readable documentation for a node.

- **Mandatory:** yes, for any node with `visibility: public`. Optional otherwise, but every setting should eventually have one — a developer should not need to read source code to understand what a setting does.
- **Available on:** setting and section nodes.

Used to generate the `datadog.yaml.example` file shipped with the Agent.

Use the YAML `|` block scalar for multi-line descriptions:

```yaml
api_key:
  node_type: setting
  type: string
  default: ""
  description: |
    The Datadog API key used by your Agent to submit metrics and events
    to Datadog.

    Create a new API key here: https://app.datadoghq.com/organization-settings/api-keys
```

---

### `title`

A short, human-readable label for a section. IDEs and other JSON Schema tools show it when hovering or completing the section, and the config example generator also renders it as the section's banner in `datadog.yaml.example`.

- **Available on:** section nodes only. Settings must use [`banner`](#banner) to start a banner: `schema.lint` rejects a `title` on a setting.

/// note
In JSON Schema, `title` labels the value of the node it's on. On a setting that starts a group of settings, the group's name would become that one setting's label in IDEs: hovering `api_key` would show "Basic Configuration". A section's `title` names the section itself, so it's a correct label and a correct banner.
///

When a section has a `title` (and no `banner`), the config example generator produces a banner like this:

```
####################################
## Trace Collection Configuration ##
####################################

## @param apm_config - custom object - optional
## Enter specific configurations for your trace collection.
## Uncomment this parameter and the one below to enable them.
## See https://docs.datadoghq.com/agent/apm/
#
# apm_config:

#   # @param enabled - boolean - optional - default: true
#   # @env DD_APM_ENABLED - boolean - optional - default: true
#   # Set to true to enable the APM Agent.
#
#   enabled: true
```

Full section node example:

```yaml
apm_config:
  node_type: section
  title: "Trace Collection Configuration"
  description: |
    Enter specific configurations for your trace collection.
    Uncomment this parameter and the one below to enable them.
    See https://docs.datadoghq.com/agent/apm/
  visibility: public
  properties:

    enabled:
      node_type: setting
      type: boolean
      default: true
      description: Enable the APM agent.
```

---

### `comment`

Internal comments about the field. Not rendered into any user-facing artifacts.

- **Available on:** setting and section nodes.
- **Mandatory:** no, optional.

Use this for developer-oriented notes that are not meant to be user-facing — for example, explaining why an unusual default was chosen, referencing a bug or design decision, or noting a deprecation plan.

```yaml
hostname_fqdn:
  node_type: setting
  type: boolean
  default: false
  comment: "Maintain backward compatibility with Agent5 behavior"
```

---

### `items` and `properties`

These keywords describe the structure of complex types and are required by code generation tooling. They are listed here separately from the validation keywords below because they define the *shape* of a setting's value, not merely constraints on it.

#### `items`

Specify the type for that every element of an `array` must satisfy. Mandatory for all `array` setting nodes.

```yaml
tags:
  node_type: setting
  type: array
  default: []
  items:
    type: string    # every element must be a string
```

---

#### `properties`

Define the sub-schema for a `object`. Each keys in `properties` is an entry in that map and contains its own sub-schema.

```yaml
cel_workload_exclude:
  node_type: setting
  type: array
  default: []
  items:
    type: object
    properties:
      products:
        type: array
        items:
          type: string
```

/// note
`properties` on a **section** node defines its child settings (Agent schema nodes), not value sub-schemas. The two uses look identical in YAML but have different meanings depending on `node_type`.
///

---

### `format`

Marks a `string` setting whose values follow an Agent-specific syntax: `format: duration` or `format: url`.

/// warning
These are **custom Agent formats, not JSON Schema formats**. JSON Schema has no `url` format, and its `duration` means something else (see below). In the generated schemas, each format is **replaced by a regular expression**: the schema embedded in the Agent and the JSON Schema published for external tools (`dda inv schema.produce-jsonschema`) both turn `format: <name>` into `$ref: '#/$defs/<name>'`, a definition holding the regexp (`pattern`), written once per schema. The Agent, IDEs and any other validator therefore check these values the same way, with the same regexp.
///

In the published JSON Schema, each definition also carries an error message (`patternErrorMessage`): VS Code and its YAML extension show it instead of the regexp when a value doesn't match, e.g. for a duration *Expected a duration: a number with a unit (ns, us, ms, s, m, h), such as 30s, 1m30s or 1.5h.* Other tools ignore this editor keyword and show their own message.

- **Valid values:** `duration` or `url`, on a `string` setting only. `schema.lint` rejects any other format: it wouldn't be replaced, and most tools would silently not validate it.
- **Why a format instead of a `pattern`:** the regexp of each format is written once, in `AGENT_FORMATS` (`tasks/schema/produce_byproduct.py`), and once in each generated schema, so settings sharing a format never copy it.

**Adding a format:**

1. Write its regexp in `tasks/schema/produce_byproduct.py` and register it in `AGENT_FORMATS`, with the error message IDEs show for a value that doesn't match: what's expected, and examples of valid values. Keep it linear-time (no nested repetition): IDEs use backtracking regex engines, and a regexp that backtracks catastrophically freezes them on invalid input. The schema tests check every registered regexp for this.
2. Use `format: <name>` on string settings.
3. Document it below.

#### `format: duration`

Marks a `string` setting holding a time duration, read by the Agent as a Go `time.Duration`.

- **When to use:** on every `string` setting the Agent reads as a duration, so its default is generated as a `time.Duration` and external tools can validate its values.
- **Valid values:** any value Go's [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) accepts: an optional sign, then one or more `<number><unit>` pairs, or a bare `0`. The number can be fractional and the units are `ns`, `us` (or `µs`), `ms`, `s`, `m` and `h`.
- **Code generation:** the `default` is rendered as a Go `time.Duration` expression (`15s` becomes `15*time.Second`).

| Value | Duration |
| --- | --- |
| `15s` | 15 seconds |
| `1m0s` | 1 minute |
| `2h30m21s` | 2 hours, 30 minutes and 21 seconds |
| `1.5h` | 1 hour and 30 minutes |
| `500ms` | 500 milliseconds |

Units Go doesn't know are invalid: `30min` must be written `30m`, and `1d` must be written `24h`. A number without a unit, such as `30`, is invalid too (only `0` is accepted as-is).

```yaml
eval_interval:
  node_type: setting
  type: string
  default: 15s
  format: duration
```

/// note
JSON Schema also defines a `format: duration`, but as an [ISO 8601 duration](https://en.wikipedia.org/wiki/ISO_8601#Durations) (`PT15S`), which isn't what the Agent reads: validators checking formats (IDEs, `ajv-formats`, ...) would reject every valid Agent value like `15s`. The generated schemas replace it with a regexp matching exactly what `time.ParseDuration` accepts (`GO_DURATION_PATTERN` in `tasks/schema/produce_byproduct.py`), so the Agent and external tools validate durations the same way.
///

#### `format: url`

Marks a `string` setting holding the URL of an endpoint the Agent sends data to (`dd_url`, `logs_config.logs_dd_url`, ...).

- **Valid values:** a host, or an `http://` or `https://` URL, e.g. `intake.datadoghq.com`, `https://app.datadoghq.com`, `https://proxy.example.com:443/logs`, `https://[::1]:8443` or `http://user:password@proxy:3128`. The host can be a name or a bracketed IPv6 address, followed by an optional port, path, query string and fragment. An empty string is valid and means "unset": the Agent then builds the endpoint from `site`. Spaces and other schemes (`ftp://`, ...) are invalid. The check is deliberately lax: rejecting a working URL is worse than accepting a bad one.
- **Validation:** the generated schemas replace `format: url` with its regexp (`URL_PATTERN` in `tasks/schema/produce_byproduct.py`).

```yaml
dd_url:
  node_type: setting
  type: string
  default: https://app.datadoghq.com
  format: url
```

---

### Validation keywords

Each JSON Schema type comes with built-in validation keywords. The most useful ones in the Agent schema are listed below. Validation rules can be arbitrarily nested — see [Examples](../../architecture/agent-schema/examples.md#example-3-complex-nested-type-cel_workload_exclude) for a real-world demonstration.

| Keyword | Applies to | Description |
| --- | --- | --- |
| `minimum` / `maximum` | `number` | Numeric lower and upper bounds |
| `enum` | `string`, `number`, array items | Restricts the value to a fixed set |
| `pattern` | `string` | Regular expression the value must match |
| `minLength` / `maxLength` | `string` | String length bounds |
| `additionalProperties` | `object` | Control if keys not listed in `properties` are accepted or not. The JSON Schema default is `true` (unknown keys are allowed), so set to `false` explicitly to reject them. |
| `required` | `object` | List of keys that must be present |
| `minItems` / `maxItems` | `array` | Array length bounds |
| `uniqueItems` | `array` | Requires all elements to be distinct |

For the complete specification of these keywords, see [Understanding JSON Schema](https://json-schema.org/understanding-json-schema).

---

## Datadog Agent extensions

The following keywords extend JSON Schema to meet the Agent's specific needs. They are **not** used for config validation — any standard JSON Schema library can validate a customer config without them. They are used by the Agent itself and by Datadog internal tooling.

---

### `node_type`

Declares whether a node is a *section* (group of settings) or a *setting* (individual setting).

- **Available on:** all nodes.
- **Mandatory:** yes.
- **Accepted values:** `section`, `setting`.

This keyword marks the boundary between the schema structure and a setting's value. For example, `docker_labels_as_tags` has `type: object` — its value is an object of strings to strings. That object is the setting's *value*, so the node is a setting, not a section. A node is a section only when its `properties` represent *child settings*, not a setting with an object typed value.

```yaml
apm_config:
  node_type: section     # Within the JSON Schema`apm_config` is a 'object' type but for the Agent it's a section
  type: object
  title: "Trace Collection Configuration"
  properties:

    enabled:
      node_type: setting
      type: boolean
      default: false

docker_labels_as_tags:
  node_type: setting     # Within the JSON Schema`docker_labels_as_tags` is a 'object' type but for the Agent it's a setting
  type: object
  default: {}
```

---

### `platform_default`

Sets per-platform default value overrides. Mutually exclusive with `default` (One of them must be specify).

- **Available on:** setting nodes only.
- **Mandatory:** yes, unless `default` is specified. `platform_default` must cover every platform — either by listing `linux`, `windows`, `darwin`, and `aix` explicitly, or by including an `other` catch-all.
- **Validation:** every value must be a valid value of the setting (its `type`, `pattern`, `enum`, ...), as checked by the schema unit tests.

Supported platform keys: `linux`, `windows`, `darwin`, `aix`, `container`, `fargate`, `other`.

As much as possible, avoid assuming all platforms are known, and don't use `other` for a value which is specific to a given OS just because the other OSes have been set explicitly.

**Container fallback logic:** because container environments currently share many defaults with Linux, `container`/`fargate` is optional. When resolving the default for a setting, the Agent applies the following fallback chain:

1. Use `fargate` if present and running on ECS Farget.
1. Fall back to `container` if present running in a container.
1. Fall back to `linux` if present.
1. Fall back to `other` if present.

```yaml
# Explicit entry for every platform:
confd_path:
  node_type: setting
  type: string
  platform_default:
    windows: "C:\\ProgramData\\Datadog\\conf.d"
    linux: "/etc/datadog-agent/conf.d"
    darwin: "/opt/datadog-agent/etc/conf.d"
    aix: "/etc/datadog-agent/conf.d"
    container: "/conf.d"

# Using 'other' as a catch-all:
gui_port:
  node_type: setting
  type: number
  platform_default:
    darwin: 5002
    windows: 5002
    other: -1             # currently covers linux and aix
```

/// note
[Relative Path](keywords.md#relative-defaults) is also available for `platform_default`.
///

In the JSON Schema published for external tools, the per-platform values of a **public** setting are added at the end of its `description` (see the note in [`default`](#default)), for example:

```
Default: 5002 on Windows and macOS, -1 elsewhere.
```

Settings that aren't public get nothing added.

---

### `sensitive`

Marks a setting as containing sensitive data.

- **Available on:** setting nodes only.
- **Mandatory:** no.
- **Default:** `false`.
- **Accepted values:** `true`, `false`.

When `true`, the config's `IsSensitive(key)` reports the setting. Fleet Automation treats the setting as a secret — it will not expose the raw value in its UI or API responses. Other services that consume the schema may use this flag in the future to apply additional protection.

```yaml
api_key:
  node_type: setting
  type: string
  default: ""
  sensitive: true
  description: "Your Datadog API key."
  visibility: public
```

---

### `visibility`

Controls whether a setting is publicly documented and included in `datadog.yaml.example`.

- **Available on:** all nodes (setting and section).
- **Mandatory:** no.
- **Default:** `undocumented`.
- **Accepted values:** `public`, `undocumented`.

Any node with `visibility: public` is included in the generated config examples and any public-facing configuration website. Nodes without this keyword (or with `visibility: undocumented`) are internal and will not be surfaced.

/// note
It's not because a setting is undocumented that it's not known or used by customers.
///

```yaml
api_key:
  node_type: setting
  type: string
  default: ""
  description: "Your Datadog API key."
  visibility: public
```

/// note
the configuration examples are generated following the schema order. Where you add you setting in the schema will determine where is will appear in the examples.
///

---

### `banner`

The text of a banner rendered in `datadog.yaml.example` just before the node, to start a new group of settings.

- **Available on:** all nodes (setting and section). On a setting, it's the only way to get a banner.
- **Mandatory:** no.

Most top-level settings aren't in a section, so a group of them is started by putting a `banner` on its first setting. Every following setting belongs to the group until the next banner:

```yaml
api_key:
  node_type: setting
  banner: Basic Configuration
  type: string
  default: ""
  visibility: public
  description: The Datadog API key used by your Agent to submit metrics and events to Datadog.
```

renders as:

```
#########################
## Basic Configuration ##
#########################

## @param api_key - string - required
## @env DD_API_KEY - string - required
## The Datadog API key used by your Agent to submit metrics and events to Datadog.
```

A section uses its [`title`](#title) as its banner, so it rarely needs a `banner`. If it has both, the `banner` is rendered and the `title` stays the section's label for IDEs.

`banner` is only used to generate the config examples: it's removed from the schema embedded in the Agent and from the JSON Schema published for external tools.

/// note
Don't use `title` on a setting to get a banner. `title` is the standard JSON Schema label of a value, which IDEs show for that setting: `api_key` would be labelled "Basic Configuration". `schema.lint` rejects a `title` on a setting.
///

---

### `env_vars`

The list of environment variables that can override this setting node's value.

- **Available on:** setting nodes only.
- **Mandatory:** no.

**If omitted**, the Agent uses a default env var derived from the setting's full dotted path: `DD_` + the path in upper case with dots replaced by underscores. For example, `logs_config.enabled` defaults to `DD_LOGS_CONFIG_ENABLED`.

When multiple env vars are listed, they are checked in order and the first match wins.

```yaml
api_key:
  node_type: setting
  type: string
  default: ""
  env_vars:
    - DD_API_KEY
    - DATADOG_API_KEY
```

---

### `env_parser`

Defines how the env var value is parsed into the setting's type. This is only require when loading complex types from the environment (JSON, maps, ...).

- **Available on:** setting nodes only.
- **Mandatory:** no. Most scalar types (`boolean`, `number`, `string`) are parsed automatically.

| Value | Behaviour |
| --- | --- |
| `comma_separated` | Splits on commas |
| `space_separated` | Splits on spaces |
| `json` | Parses the entire env var value as a JSON payload matching the setting's type |

Legacy parsers, **do not use for new settings**:

| Value | Behaviour |
| --- | --- |
| `comma_and_space_separated` | Both commas and spaces act as separators. Used by OTEL for `otelcollector.converter.features`. |
| `traces_span` | Parses a comma-separated list of `service\|operation=rate` tokens into a map. Used by APM for `apm_config.analyzed_spans`. |
| `csv_comma_separated` | Parses a CSV-encoded list of strings (supports quoted fields and leading whitespace). Used by APM for `apm_config.ignore_resources`. |
| `comma_then_space_separated` | Splits on commas if the value contains a comma, otherwise on spaces. Mixing separators within a single value is not supported. Used by APM for `apm_config.features`. |
| `json_list_or_comma_separated` | Parses as a JSON array if the value starts with `[`, otherwise splits on commas. Used by `private_action_runner.restricted_shell.allowed_commands`, `private_action_runner.restricted_shell.allowed_paths`, `process_config.custom_sensitive_words`. |
| `json_list_or_space_separated` | Parses as a JSON array if the value starts with `[`, otherwise splits on spaces. Used by `apm_config.filter_tags.require`, `apm_config.filter_tags.reject`, `apm_config.filter_tags_regex.require`, `apm_config.filter_tags_regex.reject`, and `apm_config.obfuscation.credit_cards.keep_values`. |

```yaml
tags:
  node_type: setting
  type: array
  items:        # describes the value — each element must be a string
    type: string
  default: []
  env_vars:
    - DD_TAGS
  env_parser: json
```

---

### `renamed_from`

Maps every previous name this setting was known by to the Agent version that deprecated that name. Each key is a former **fully qualified** name (dotted full path from the root of the config).

- **Available on:** setting nodes only.
- **Mandatory:** no.
- **Validation:** must be a non-empty mapping. Keys are non-empty strings; values are quoted, full semantic Agent versions (for example `"7.71.0"`). Two former names of the same setting cannot share a version.

When a setting has `renamed_from`, the config system looks for any of the previous names in YAML and env vars and migrates the value to the new name automatically. Previous names take priority over the canonical name when both are present. A deprecation warning is emitted at runtime whenever a previous name is used. Previous names are sorted by Agent version in which they were deprecated (oldest to newest).

The code base **MUST** use the new name only. The old name is no longer accessible through the config but can still be used by customer in their configuration or env vars.

This provides a single, consistent mechanism for setting renames across all teams, replacing ad-hoc solutions that previously produced inconsistent behaviour.

```yaml
apm_config:
  node_type: section
  type: object
  properties:

    target_traces_per_second:
      node_type: setting
      type: number
      default: 10
      renamed_from:
        apm_config.max_traces_per_second: "7.71.0"
```

A setting renamed more than once lists every former name with the version that deprecated it:

```yaml
    target_traces_per_second:
      node_type: setting
      type: number
      default: 10
      renamed_from:

        # Settings can be moved from one section into another
        # Here the setting was moved from 'experimental.apm_config' into 'apm_config'
        experimental.apm_config.max_traces_per_second: "7.60.0"

        # Even if the settting stays in the same section the full
        # dotted name must be used
        apm_config.max_tps: "7.71.0"
```

/// note
if no `env_vars` are specified the agent will generate the correct ones from the name and all the deprecated ones. A warning will be emited if any deprecated env vars are used.

But, similar to BindEnvAndSetDefault, if `env_vars` is set for a setting, no extra env vars are generated and no warning will be emited. When renaming a setting with `env_vars` you **MUST** properly maintain the `env_vars` list. Remember that `env_vars` are sorted from highest to lowest priority.
///

---

### `tags`

An arbitrary list of strings for metadata. Used by internal tooling to slice and filter settings — for example, to produce different variants of `datadog.yaml.example`.

- **Available on:** all nodes (setting and section).
- **Mandatory:** no.
- **Validation:** must be a list of strings.

```yaml
internal_profiling:
  node_type: section
  tags:
    - internal_template_section:profiling
```

Existing tags:

- `template_section`: controls the different flavor of the configuration example we generate. This is directly inherited from the way we used to generate example from Go templates.
- `golang_type`: flag that this setting should use a different type when generating go code. Usage of `golang_type` tag is often a sign of an issue. The agent code should be easily configurable from YAML types.

    - `golang_type:duration`: will use a `time.duration`.
    - `golang_type:int64`: will use a `int64` (only available for `type: integer`).
    - `golang_type:map[string]float64`: will used a `map[string]float64{}`.
    - `golang_type:map[string]interface{}`: will used a `golang_type:map[string]interface{}{}`.

- `no-env`: mark the settings as not configurable through en vars (should not be used by new settings).
- `generate_const:<name>`: generate a Go constant from this setting's default value.

    Adding `generate_const:<name>` to a setting tells the code generator to emit a Go constant named `<name>` in the `pkg/config/setup/constants` package (in `constants/generated.go`) whose value is this setting's default. Reference that constant from your Go code instead of hardcoding the value, so the constant and the setting default can never drift apart — the schema stays the single source of truth.

    Use it when a Go constant in the Agent must always equal a setting's default (for example, a default port, timeout, or path that other code needs to read directly).

    - `<name>` is the Go constant name and must contain only letters (upper or lower case) and digits.
    - Only valid on **setting** nodes, never on sections.
    - The same `<name>` may be reused on several settings, but they must all share the same default — a constant can only have one value.

    ```yaml
    security_agent:
      node_type: section
      properties:
        cmd_port:
          node_type: setting
          type: integer
          default: 5010
          tags:
            - generate_const:DefaultSecurityAgentCmdPort   # emits: DefaultSecurityAgentCmdPort = 5010
    ```

/// note
except from `template_section` and `generate_const`, all the other tags exists to support legacy behavior and should not be used for new settings.
///
