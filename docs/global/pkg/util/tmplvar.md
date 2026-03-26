# pkg/util/tmplvar

## Purpose

`pkg/util/tmplvar` implements the `%%variable%%` template variable system used by the Autodiscovery subsystem to inject runtime values — container IP, port, PID, hostname, Kubernetes metadata, and environment variables — into check configuration templates. It has two distinct responsibilities:

1. **Parsing** (`parse.go`): extract `%%var%%` tokens from raw byte slices without resolving them.
2. **Resolving** (`resolver.go`): walk a YAML or JSON configuration tree and replace all `%%var%%` tokens with live values fetched from a runtime service.

## Key elements

### Parsing (`parse.go`)

```go
type TemplateVar struct {
    Raw, Name, Key []byte
}

func Parse(b []byte) []TemplateVar
func ParseString(s string) []TemplateVar
```

`Parse` uses the regex `%%.+?%%` to find all template variable occurrences. For each match it splits on `_` to separate the variable name (e.g. `host`) from an optional key/index (e.g. a network name or port index).

### Resolving (`resolver.go`)

#### `Resolvable` interface

Any service entity passed to the resolver must implement:

```go
type Resolvable interface {
    GetServiceID() string
    GetHosts() (map[string]string, error)
    GetPorts() ([]workloadmeta.ContainerPort, error)
    GetPid() (int, error)
    GetHostname() (string, error)
    GetExtraConfig(key string) (string, error)
}
```

#### `TemplateResolver`

```go
type TemplateResolver struct { ... }

func NewTemplateResolver(parser Parser, postProcessor func(interface{}) error, supportEnvVars bool) *TemplateResolver
func (t TemplateResolver) ResolveDataWithTemplateVars(data []byte, res Resolvable) ([]byte, error)
```

`ResolveDataWithTemplateVars` unmarshals `data` (YAML or JSON, controlled by `Parser`), walks the resulting tree, and replaces every `%%var%%` string in place. The `%%` delimiter is temporarily substituted with the `‰` character before unmarshalling to keep the YAML parser from rejecting percent signs in unquoted values.

If the resolved string consists of a single `%%var%%` token and the replacement value parses as an integer or boolean (except for `%%env_*%%`), the output node takes that native type rather than remaining a string.

IPv6 addresses returned by `%%host%%` are automatically wrapped in square brackets when the surrounding string parses as a URL (e.g. `http://%%host%%:80/`).

#### Built-in variable getters

| Template variable | Getter function | Description |
|---|---|---|
| `%%host%%` or `%%host_<network>%%` | `GetHost` | Container IP for the named network, falling back to bridge or the only available network. |
| `%%port%%` or `%%port_<idx/name>%%` | `GetPort` | Exposed port by index or name; defaults to the last port. |
| `%%pid%%` | `GetPid` | Container process ID. |
| `%%hostname%%` | `GetHostname` | Container hostname. |
| `%%extra_<key>%%` / `%%kube_<key>%%` | `GetAdditionalTplVariables` | Listener-specific or Kubernetes-specific key–value data via `GetExtraConfig`. |
| `%%env_<VAR>%%` | `GetEnvvar` | Agent process environment variable (requires `supportEnvVars: true`; controlled by `ad_disable_env_var_resolution` and `ad_allowed_env_vars`). |

#### `Parser` / pre-built parsers

```go
type Parser struct {
    Marshal   func(interface{}) ([]byte, error)
    Unmarshal func([]byte, interface{}) error
}

var JSONParser Parser  // json.Marshal / json.Unmarshal
var YAMLParser Parser  // yaml.Marshal / yaml.Unmarshal
```

#### Error types

`NoResolverError` is returned when a variable that requires a resolver is used but `res` is `nil`. Callers can type-assert against it to distinguish "no resolver configured" from other resolution failures.

## Usage

`pkg/util/tmplvar` is used in two primary places:

- **`comp/core/autodiscovery/configresolver`** — the main Autodiscovery config resolver calls
  `TemplateResolver.ResolveDataWithTemplateVars` on every check instance template before
  scheduling the check. The `Resolvable` argument is populated from the `listeners.Service`
  object that matched the template's `ADIdentifiers`. The resolved bytes replace the raw
  `integration.Config.Instances` and `integration.Config.InitConfig` data before the config
  is handed to the check scheduler. Template resolution happens inside the
  `configresolver.ConfigResolver` type; see
  [`comp/core/autodiscovery`](../../comp/core/autodiscovery.md) for the full AD lifecycle.
- **`comp/core/tagger/collectors/workloadmeta_extract.go`** and related tagger code — calls
  `ParseString` to detect which template variables are present in tag values, and
  `ResolveDataWithTemplateVars` to substitute them during tag extraction.

### Relationship with Autodiscovery template variables

The table of built-in variable getters in `pkg/util/tmplvar` maps directly to the template
variables documented in [`comp/core/autodiscovery`](../../comp/core/autodiscovery.md):

| `%%var%%` token | Getter | AD doc entry |
|---|---|---|
| `%%host%%` / `%%host_<net>%%` | `GetHost` | Primary container IP |
| `%%port%%` / `%%port_<n>%%` | `GetPort` | Exposed port by index/name |
| `%%pid%%` | `GetPid` | Container PID |
| `%%hostname%%` | `GetHostname` | Container hostname |
| `%%kube_<key>%%` | `GetAdditionalTplVariables` | Kubernetes annotation key–value |
| `%%env_<VAR>%%` | `GetEnvvar` | Agent process environment variable |

The `%%env_<VAR>%%` getter is only enabled when `supportEnvVars: true` is passed to
`NewTemplateResolver`. In production this flag is controlled by two agent config keys:
- `ad_disable_env_var_resolution` — set to `true` to disable all `%%env_*%%` substitution.
- `ad_allowed_env_vars` — if non-empty, only the listed environment variable names are
  substituted; any other `%%env_<VAR>%%` token remains unexpanded.

### Controlling environment variable resolution (security)

Because `%%env_<VAR>%%` exposes agent process environment variables (which may contain
secrets) into check configs written by container operators, the two config keys above provide
a defence-in-depth mechanism. Operators running untrusted workloads should set
`ad_disable_env_var_resolution: true` or populate `ad_allowed_env_vars` with a strict
allowlist.

### Typical call sequence

```go
resolver := tmplvar.NewTemplateResolver(tmplvar.YAMLParser, nil, true)
resolved, err := resolver.ResolveDataWithTemplateVars(templateBytes, serviceResolvable)
```

The `postProcessor` argument (second param to `NewTemplateResolver`) is an optional hook
called after the tree walk. The Autodiscovery config resolver uses it to validate the
resulting YAML against the check's JSON schema, if available.

## Related packages

- [`comp/core/autodiscovery`](../../comp/core/autodiscovery.md) — the primary caller of
  `ResolveDataWithTemplateVars`. The `configresolver` sub-package constructs a
  `TemplateResolver` at startup and calls it for every template matched to a service. The AD
  doc's "Template variables" section lists all supported tokens with their runtime semantics.
- [`comp/core/tagger`](../../comp/core/tagger.md) — uses `ParseString` and
  `ResolveDataWithTemplateVars` in `workloadmeta_extract.go` to expand template variables
  found in tag values during tag extraction. The `Resolvable` passed here is backed by the
  tagger's internal entity data rather than a listener `Service`.
- `comp/core/autodiscovery/providers/container` — generates `integration.Config` values from
  container labels and Kubernetes pod annotations. The raw instance configs it produces may
  contain `%%var%%` tokens; these are left unexpanded until the config resolver calls
  `tmplvar` at resolution time.
- `pkg/autodiscovery/listeners` — the `Service` interface returned by listeners (Docker,
  kubelet, ECS, etc.) must implement `tmplvar.Resolvable` so that `GetHosts`, `GetPorts`,
  `GetPid`, `GetHostname`, and `GetExtraConfig` can be called by `TemplateResolver`.
