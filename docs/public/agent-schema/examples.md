# Examples

This page walks through three annotated examples taken from or inspired by the
real schema files. Each example builds on the concepts introduced in the
[Keyword Reference](keywords.md).

---

## Example 1 — Simple public setting node (`api_key`)

```yaml
api_key:
  node_type: setting    # (1)
  type: string          # (2)
  default: ""           # (3)
  description: |        # (4)
    The Datadog API key used by your Agent to submit metrics and events
    to Datadog.

    Create a new API key here: https://app.datadoghq.com/organization-settings/api-keys
  env_vars:             # (5)
    - DD_API_KEY
    - DATADOG_API_KEY
  visibility: public    # (6)
```

1. **`node_type: setting`** — this node is an individual setting that holds a value.
2. **`type: string`** — the setting holds a text value.
3. **`default: ""`** — the default is an empty string. Mandatory for all setting
   nodes unless `platform_default` is used instead.
4. **`description`** — free-text documentation. Multi-line descriptions use the
   YAML `|` block scalar. This text appears in `datadog.yaml.example` and any
   public-facing configuration website.
5. **`env_vars`** — lists the environment variables that set this value. Both
   `DD_API_KEY` and `DATADOG_API_KEY` are accepted; they are checked in order
   and the first match wins. Without this field, the Agent would derive a
   default env var from the setting path (`DD_API_KEY` in this case, which
   matches the automatically derived name in this case).
6. **`visibility: public`** — includes this setting in `datadog.yaml.example`
   and public documentation. Without this, the setting is considered internal.

---

## Example 2 — Section node with mixed children (`apm_config`)

This example shows a section node taken from `pkg/config/schema/core_schema.yaml`
with several child setting nodes, including one with a `platform_default`.

```yaml
apm_config:
  node_type: section                        # (1)
  title: "Trace Collection Configuration"   # (2)
  description: |                            # (3)
    Enter specific configurations for your trace collection.
    Uncomment this parameter and the one below to enable them.
    See https://docs.datadoghq.com/agent/apm/
  visibility: public                        # (4)
  properties:                               # (5)

    enabled:
      node_type: setting
      type: boolean
      default: false                        # (6)
      description: Enable the APM agent.
      env_vars:
        - DD_APM_ENABLED
      visibility: public

    receiver_socket:
      node_type: setting
      type: string
      platform_default:                     # (7)
        linux: /var/run/datadog/apm.socket
        other: ""
      description: |
        Accept traces through Unix Domain Sockets.
        Set to "" to disable the UDS receiver.
      env_vars:
        - DD_APM_RECEIVER_SOCKET
      visibility: public

    apm_dd_url:
      node_type: setting
      type: string
      default: ""
      description: |
        Define the endpoint and port to hit when using a proxy for APM.
        The traces are forwarded in TCP, so the proxy must handle TCP connections.
      env_vars:
        - DD_APM_DD_URL
      visibility: public
```

1. **`node_type: section`** — this node groups child settings/section rather than holding a value itself.
2. **`title`** — generates a section banner in `datadog.yaml.example`:
   ```
   ####################################
   ## Trace Collection Configuration ##
   ####################################
   ```
3. **`description`** — section-level documentation that appears below the banner.
4. **`visibility: public`** — marks the section as public. Child nodes still need their own `visibility: public` to
   appear in the generated output.
5. **`properties`** — the child settings. Each key is a setting name; each value is a setting or nested section node.
6. **`default: false`** — a simple boolean default on a regular cross-platform setting node.
7. **`platform_default`** — used instead of `default` when the correct default depends on the operating system. Here,
   Linux uses a Unix socket path while all other platforms default to an empty string. The special key `other` acts as a
   fallback for any platform not explicitly listed.

---

## Example 3 — Complex nested type (`cel_workload_exclude`)

This example shows how JSON Schema's composable
validation handles a non-trivial type. `cel_workload_exclude` corresponds to
the following Go struct:

```go
type Product string
type ResourceType string

type RuleBundle struct {
    Products []Product
    Rules    map[ResourceType][]string
}
// cel_workload_exclude is []RuleBundle
```

`cel_workload_exclude` is a single **setting node** whose value is an array. The
`items`, `properties`, and `additionalProperties` keywords below describe the
*shape of the value* — they are JSON Schema validation keywords, not Agent
schema nodes.

```yaml
cel_workload_exclude:
  node_type: setting     # a single setting whose value is an array
  type: array            # (1)
  default: []

  # Everything below describes the value, not new schema nodes:
  items:                # (2)
    type: object        # (3)

    properties:         # (4)
      products:
        type: array     # (5)
        items:
          type: string  # each product is a string

      rules:
        type: object                  # (6)
        additionalProperties:         # (7)
          type: array
          items:
            type: string              # each rule expression is a string

    additionalProperties: false       # (8)
```

1. **`type: array`** — the setting's value is a list.
2. **`items`** — defines the schema that every element of the list must
   satisfy. This is a value constraint, not a child node.
3. **`type: object`** — each element of the list is a dict.
4. **`properties`** — the allowed keys of each dict: `products` and `rules`.
   These are keys in the *value*, not Agent schema nodes.
5. **`type: array` / `items: {type: string}`** — `products` is a list of
   strings.
6. **`type: object`** — `rules` is itself a dict.
7. **`additionalProperties: {type: array, items: {type: string}}`** — every
   *value* in the `rules` dict must be a list of strings. The *keys* are
   unconstrained (they are resource type names).
8. **`additionalProperties: false`** on the outer object — no keys other than
   `products` and `rules` are allowed in each element.

A valid config value that matches this schema:

```yaml
cel_workload_exclude:
  - products:
      - metrics
    rules:
      kube_service:
        - service.name == 'yaml-service'
      pod:
        - pod.namespace == 'yaml-ns'
      container:
        - container.name == 'yaml-test'
```

This demonstrates a key strength of JSON Schema: complex, deeply nested types
are validated with the same small set of composable keywords used for simple
scalars. No custom validation code is needed.
