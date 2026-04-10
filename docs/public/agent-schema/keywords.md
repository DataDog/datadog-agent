# Keyword Reference

This page documents keywords supported by the Agent configuration schema.
Keywords are grouped into two sections: standard JSON Schema keywords and
Datadog Agent extensions.

---

## Standard JSON Schema keywords

The following keywords come from the [JSON Schema standard](https://json-schema.org/understanding-json-schema/keywords)
and are understood by all JSON Schema tooling. This section describes how each
keyword is used **specifically in the Agent schema**.

> **Note:** not all keywords define in the JSON Schema standard are explain here, only the most commonly used.

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

Complex types are composed by combining these primitives — for example, an
`array` whose `items` are `object`s.

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

- **Mandatory:** yes, for all setting nodes. Mutually exclusive with `platform_default` (one or the other must be
  specified).

The value must match the `type` of the setting.

```yaml
check_runners:
  node_type: setting
  type: number
  default: 4
```

#### Relative defaults

Some default value are resolved at runtime base on the agent install parameter. Default value are often relative to the
"install path", "log directory" ...

Those concept can be express using the `${}` notation with one of the following variables. The value below show the
default but could be change by the users. For example, the default configuration directory for Windows is `c:/programdata/datadog` but can be changed at install time. The correct value will be use at startup by the Agent.

Path are all express using `/` and will be translated to the correct OS version.

Existing variagbles:
- Configuration directory:
  - `windows`: `c:/programdata/datadog`
  - `linux`; `/etc/datadog-agent`
  - `darwin`: `/opt/datadog-agent/etc`
- Installation directory:
  - `windows`: `c:/program files/datadog/datadog agent`
  - `linux`; `/opt/datadog-agent`
  - `darwin`: `/opt/datadog-agent`
- Log directory:
  - `windows`: `c:/programdata/datadog/logs`
  - `linux`; `/var/log/datadog`
  - `darwin`: `/opt/datadog-agent/logs`
- Run directory (where the Agent starts from):
  - `windows`: `/opt/datadog-agent/run`
  - `linux`; `c:/programdata/datadog/run`
  - `darwin`: `/opt/datadog-agent/run`


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

A short heading used to generate section banners in `datadog.yaml.example`.

- **Available on:** section nodes only.

> **Note:** The Agent schema's use of `title` differs from the JSON Schema standard, where `title` is a
> concise human-readable summary of a schema. Here, `title` is specifically a banner label applied only
> to section nodes that begin a new conceptual group in the generated config file.

When a section has a `title`, the config example generator produces a banner like this:

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

Use this for developer-oriented notes that are not meant to be user-facing — for example, explaining
why an unusual default was chosen, referencing a bug or design decision, or noting a deprecation plan.

```yaml
hostname_fqdn:
  node_type: setting
  type: boolean
  default: false
  comment: "Maintain backward compatibility with Agent5 behavior"
```

---

### `items` and `properties`

These keywords describe the structure of complex types and are required by code generation tooling.
They are listed here separately from the validation keywords below because they define the *shape* of
a setting's value, not merely constraints on it.

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

> **Note:** `properties` on a **section** node defines its child settings (Agent schema nodes), not
> value sub-schemas. The two uses look identical in YAML but have different meanings depending on
> `node_type`.

---

### Validation keywords

Each JSON Schema type comes with built-in validation keywords. The most useful
ones in the Agent schema are listed below. Validation rules can be arbitrarily
nested — see [Examples](examples.md#example-3-complex-nested-type-cel_workload_exclude)
for a real-world demonstration.

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

For the complete specification of these keywords, see
[Understanding JSON Schema](https://json-schema.org/understanding-json-schema).

---

## Datadog Agent extensions

The following keywords extend JSON Schema to meet the Agent's specific needs.
They are **not** used for config validation — any standard JSON Schema library
can validate a customer config without them. They are used by the Agent itself
and by Datadog internal tooling.

---

### `node_type`

Declares whether a node is a *section* (group of settings) or a *setting*
(individual setting).

- **Available on:** all nodes.
- **Mandatory:** yes.
- **Accepted values:** `section`, `setting`.

This keyword marks the boundary between the schema structure and a setting's
value. For example, `docker_labels_as_tags` has `type: object` — its value is an
object of strings to strings. That object is the setting's *value*, so the node is a setting, not
a section. A node is a section only when its `properties` represent *child
settings*, not a setting with an object typed value.

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
- **Mandatory:** yes, unless `default` is specified. `platform_default` must cover every platform — either by listing
  `linux`, `windows`, and `darwin` explicitly, or by including an `other` catch-all.
- **Validation:** values must match the `type` of the setting.

Supported platform keys: `linux`, `windows`, `darwin`, `container`, `other`.

**Container fallback logic:** because container environments currently share many
defaults with Linux, `container` is optional. When resolving the default for a
container, the Agent applies the following fallback chain:

1. Use `container` if present.
2. Fall back to `linux` if present.
3. Fall back to `other` if present.

```yaml
# Explicit entry for every platform:
confd_path:
  node_type: setting
  type: string
  platform_default:
    windows: "C:\\ProgramData\\Datadog\\conf.d"
    linux: "/etc/datadog-agent/conf.d"
    darwin: "/opt/datadog-agent/etc/conf.d"
    container: "/conf.d"

# Using 'other' as a catch-all:
gui_port:
  node_type: setting
  type: number
  platform_default:
    linux: -1
    other: 5002             # covers windows and darwin
```

> **Note**: [Relative Path](keywords.md#relative-defaults) is also available for `platform_default`.

---

### `sensitive`

Marks a setting as containing sensitive data.

- **Available on:** setting nodes only.
- **Mandatory:** no.
- **Default:** `false`.
- **Accepted values:** `true`, `false`.

When `true`, the Agent scrubs the value from logs and diagnostic output. Fleet
Automation treats the setting as a secret — it will not expose the raw value in
its UI or API responses. Other services that consume the schema may use this flag
in the future to apply additional protection.

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

Controls whether a setting is publicly documented and included in
`datadog.yaml.example`.

- **Available on:** all nodes (setting and section).
- **Mandatory:** no.
- **Default:** `undocumented`.
- **Accepted values:** `public`, `undocumented`.

Any node with `visibility: public` is included in the generated config examples
and any public-facing configuration website. Nodes without this keyword (or with
`visibility: undocumented`) are internal and will not be surfaced.

> **Note:** It's not because a setting is undocumented that it's not known or used by customers.

```yaml
api_key:
  node_type: setting
  type: string
  default: ""
  description: "Your Datadog API key."
  visibility: public
```

> **Note:** the configuration examples are generated following the schema order. Where you add you setting in the schema
> will determine where is will appear in the examples.

---

### `env_vars`

The list of environment variables that can override this setting node's value.

- **Available on:** setting nodes only.
- **Mandatory:** no.

**If omitted**, the Agent uses a default env var derived from the setting's full
dotted path: `DD_` + the path in upper case with dots replaced by underscores.
For example, `logs_config.enabled` defaults to `DD_LOGS_CONFIG_ENABLED`.

When multiple env vars are listed, they are checked in order and the first match
wins.

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

Defines how the env var value is parsed into the setting's type. This is only require when loading complex types from
the environment (JSON, maps, ...).

- **Available on:** setting nodes only.
- **Mandatory:** no. Most scalar types (`boolean`, `number`, `string`) are parsed automatically.

| Value | Behaviour |
| --- | --- |
| `comma_separated` | Splits on commas |
| `space_separated` | Splits on spaces |
| `json` | Parses the entire env var value as a JSON payload matching the setting's type |
| `comma_or_space_separated` | Splits on commas if the value contains a comma, otherwise on spaces. Used by APM for `apm_config.ignore_resources`. **Should not be used for new settings**. |
| `comma_and_space_separated` | Both commas and spaces act as separators. Used by OTEL for `otelcollector.converter.features`. **Should not be used for new settings**. |
| `space_or_json` | Splits on spaces, or parses as JSON if the value starts with `[`. **Should not be used for new settings**. |

```yaml
tags:
  node_type: setting
  type: array
  items:        # describes the value — each element must be a string
    type: string
  default: []
  env_vars:
    - DD_TAGS
  env_parser: space_or_json
```

---

### `renamed_from`

> **Note:** [WIP] Not yet implemented. The migration behaviour described below is planned.

Lists previous names this setting was known by.

- **Available on:** setting nodes only.
- **Mandatory:** no.
- **Validation:** must be a list of strings.

When a setting has `renamed_from`, the config system looks for any of the
previous names and migrates the value automatically. Previous names take
priority over the canonical name when both are present. A deprecation warning
is emitted at runtime whenever a previous name is used.

This provides a single, consistent mechanism for setting renames across all
teams, replacing ad-hoc solutions that previously produced inconsistent
behaviour.

```yaml
target_traces_per_second:
  node_type: setting
  type: number
  default: 10
  renamed_from:
    - max_traces_per_second
```

---

### `tags`

An arbitrary list of strings for metadata. Used by internal tooling to slice
and filter settings — for example, to produce different variants of
`datadog.yaml.example`.

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

- `template_section`: controls the different flavor of the configuration example we generate. This is directly inherited
  from the way we used to generate example from Go templates.
- `TODO:fix-no-default`: flag that this legacy setting has no default.
- `TODO:fix-missing-type`: flag that this legacy setting has no type.
- `golang_type`: flag that this setting should use a different type when generating go code. Usage of `golang_type` tag
  is often a sign of an issue. The agent code should be easily configurable from YAML types.
  - `golang_type:duration`: will use a `time.duration`.
  - `golang_type:float64`: will use a `float64`.
  - `golang_type:map[string]float64`: will used a `map[string]float64{}`.
  - `golang_type:map[string]interface{}`: will used a `golang_type:map[string]interface{}{}`.
  - `golang_type:nil`: will use `nil` from Go (should not be used for any new setting).
- `no-env`: mark the settings as not configurable through en vars (should not be used by new settings).

> **Note**: except from `template_section`, all the other tags exists to support legacy behavior and should not be used
> for new settings.
