> **TL;DR:** Removes sensitive data (credentials, API keys, tokens) from process command lines, Kubernetes pod/container specs, and Custom Resource manifests before they are sent to the Datadog backend — purpose-built for the orchestrator and process collector pipelines.

# pkg/redact

## Purpose

`pkg/redact` removes sensitive data from process command lines, Kubernetes pod/container
specs, and Custom Resource manifests before the data is sent to the Datadog backend. It is
distinct from `pkg/util/scrubber`, which targets agent configuration, YAML files, and log
output. `pkg/redact` is specifically designed for the orchestrator/process collector
pipeline, where live Kubernetes objects contain environment variables, CLI arguments, HTTP
probe headers, and annotations that may embed credentials.

## Key elements

### Key types

### `DataScrubber`

```go
type DataScrubber struct {
    Enabled                  bool
    RegexSensitivePatterns   []*regexp.Regexp
    LiteralSensitivePatterns []string
    // unexported fields ...
}
```

The central type. Holds two sets of patterns:

- `LiteralSensitivePatterns` — plain strings matched case-insensitively as substrings (e.g. `"password"`, `"api_key"`).
- `RegexSensitivePatterns` — compiled `*regexp.Regexp` values; patterns must only use `[a-zA-Z0-9_*]` characters and wildcards.

**Default sensitive words** (loaded by `NewDefaultDataScrubber`):

```
password, passwd, mysql_pwd, access_token, auth_token,
api_key, apikey, pwd, secret, credentials, stripetoken
```

**Construction**

| Function | Description |
|---|---|
| `NewDefaultDataScrubber()` | Returns an enabled scrubber pre-loaded with default sensitive words and their annotation regexps |

**Extending the scrubber**

| Method | Description |
|---|---|
| `AddCustomSensitiveWords(words []string)` | Appends literal words and rebuilds annotation regexps |
| `AddCustomSensitiveRegex(words []string)` | Compiles wildcard-compatible patterns and appends them |

**Core methods**

| Method | Description |
|---|---|
| `ContainsSensitiveWord(s string)` | Returns `true` if `s` contains any literal pattern (case-insensitive). Exempts entries in `knownSafeEnvVars` (e.g. `DD_AUTH_TOKEN_FILE_PATH`) |
| `ScrubSimpleCommand(cmd, args []string) ([]string, []string, bool)` | Redacts values following sensitive flag names in a split command line. Handles both `--flag=value` and `--flag value` forms. Also applies regex patterns over the joined string. Returns scrubbed cmd, scrubbed args, and a boolean indicating whether anything changed |
| `ScrubAnnotationValue(annotationValue string)` | Replaces values of sensitive JSON keys inside an annotation string with `"********"` |

**Redaction constants** (unexported, but affect observable output)

| Constant | Value |
|---|---|
| `redactedSecret` | `"********"` — replaces scrubbed CLI/env values |
| `redactedAnnotationValue` | `"-"` — replaces whole sensitive annotation/label values |

### Key functions

### Kubernetes scrubbing functions (build tag `orchestrator`)

### Configuration and build flags

The `orchestrator` build tag gates all Kubernetes-specific scrubbing functions (`ScrubPod`, `ScrubPodTemplateSpec`, `ScrubCRManifest`, `RemoveSensitiveAnnotationsAndLabels`).

The following functions are only compiled when the `orchestrator` build tag is present.

**`ScrubPod(p *v1.Pod, scrubber *DataScrubber)`**
Scrubs a Kubernetes `Pod` in-place: redacts env vars, container command/args, HTTP probe
headers, and annotation values.

**`ScrubPodTemplateSpec(template *v1.PodTemplateSpec, scrubber *DataScrubber)`**
Same as `ScrubPod` but operates on a `PodTemplateSpec` (used for Deployments, StatefulSets, etc.).

**`ScrubCRManifest(r *unstructured.Unstructured, scrubber *DataScrubber)`**
Recursively walks the `spec` of a Custom Resource manifest. Redacts any string value whose
key matches a sensitive word, and scrubs `env` arrays following the same name-based logic as
container scrubbing.

**`RemoveSensitiveAnnotationsAndLabels(annotations, labels map[string]string)`**
Replaces the values of known globally-sensitive annotation/label keys with `"-"`. Built-in
sensitive keys are:
- `kubectl.kubernetes.io/last-applied-configuration`
- `consul.hashicorp.com/original-pod`

**`UpdateSensitiveAnnotationsAndLabels(keys []string)`** / **`GetSensitiveAnnotationsAndLabels() []string`**
Thread-safe accessors to extend the global sensitive annotation/label list at runtime.

## Usage

### Orchestrator check (Kubernetes object collection)

`pkg/collector/corechecks/cluster/orchestrator/processors/k8s/` imports `pkg/redact` for
every Kubernetes resource type it collects. Each processor creates or receives a
`*DataScrubber` (configured from agent YAML via `pkg/orchestrator/config`) and passes it to
the relevant scrub function before serializing the manifest.

```go
// Example from processors/k8s/pod.go
redact.ScrubPod(pod, scrubber)
```

Custom sensitive words and regex patterns are read from the orchestrator configuration:

```go
// pkg/orchestrator/config/config.go
scrubber.AddCustomSensitiveWords(cfg.customSensitiveWords)
scrubber.AddCustomSensitiveRegex(cfg.sensitiveRegexList)
```

### Flare (Kubernetes debug archives)

`pkg/flare/archive_k8s.go` uses `pkg/redact` to clean Kubernetes manifests before
including them in a support flare.

### Process check

The process agent uses `ScrubSimpleCommand` to redact process command lines before
reporting them, preventing accidental credential exposure in the process list.

### Relationship to `pkg/process/procutil.DataScrubber`

`pkg/process/procutil` ships its own `DataScrubber` (in `data_scrubber.go`) with a
`ScrubProcessCommand` method that caches results by `(pid, createTime)` and flushes every
25 check cycles. That scrubber is purpose-built for the process check's hot path (many PIDs
per tick). `pkg/redact.DataScrubber` is the analogous type for the Kubernetes orchestrator
pipeline, where scrubbing is done once per manifest rather than once per tick.

---

## Cross-references

| Topic | Document |
|---|---|
| `pkg/util/scrubber` — agent configuration, log, and flare scrubbing (YAML/JSON key-based) | [`pkg/util/scrubber`](util/scrubber.md) |
| `pkg/process/procutil` — process-check-specific `DataScrubber` with per-PID caching | [`pkg/process/procutil`](process/procutil.md) |
| `pkg/obfuscate` — APM span sensitive-data removal (SQL literals, HTTP URLs, Redis args) | [`pkg/obfuscate`](obfuscate.md) |

### Choosing the right scrubber

| Scenario | Package to use |
|---|---|
| Redact sensitive env vars / CLI args / HTTP probe headers inside a Kubernetes `Pod` or `PodTemplateSpec` | `pkg/redact` (`ScrubPod`, `ScrubPodTemplateSpec`) |
| Redact Custom Resource manifest fields | `pkg/redact` (`ScrubCRManifest`) |
| Redact process command-line arguments in the process check hot path (with PID-level caching) | `pkg/process/procutil` (`DataScrubber.ScrubProcessCommand`) |
| Redact agent configuration YAML, log lines, or flare file contents | `pkg/util/scrubber` |
| Remove sensitive literals from SQL queries, Redis commands, or HTTP URLs in APM spans | `pkg/obfuscate` |
