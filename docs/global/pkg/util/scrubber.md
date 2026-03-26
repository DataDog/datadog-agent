# pkg/util/scrubber

## Purpose

`pkg/util/scrubber` removes sensitive credentials from arbitrary text before it is logged or
included in a diagnostic flare. It understands plain text, YAML, and JSON, and ships with a
large set of built-in patterns covering Datadog API/APP keys, passwords, bearer tokens, TLS
certificates, SNMP community strings, and many common HTTP header credential names. New keys
can be registered at runtime from agent configuration or the secrets backend.

## Key elements

### Core type: `Scrubber`

```go
type Scrubber struct { ... }
```

Holds two ordered lists of `Replacer` values: single-line replacers (applied line-by-line)
and multi-line replacers (applied to the whole text after single-line processing). Comment
lines (starting with `#`) and blank lines are dropped during single-line processing.

**Construction**

| Function | Description |
|---|---|
| `New()` | Empty scrubber with no replacers |
| `NewWithDefaults()` | Scrubber pre-loaded with all default replacers (same set as `DefaultScrubber`) |
| `AddDefaultReplacers(s *Scrubber)` | Add default replacers to an existing scrubber |

**Scrubbing methods**

| Method | Input | Notes |
|---|---|---|
| `ScrubFile(path)` | File path | Opens and processes the file |
| `ScrubBytes(data)` | `[]byte` | General-purpose; runs both pass types |
| `ScrubLine(line)` | Single `string` | Only single-line replacers; safe for URLs |
| `ScrubYaml(data)` | `[]byte` YAML | Parses YAML first, scrubs the object graph, re-serialises, then applies byte-level pass |
| `ScrubJSON(data)` | `[]byte` JSON | Same two-phase approach as `ScrubYaml` |
| `ScrubDataObj(data *interface{})` | Parsed Go object | In-place walk for already-decoded YAML/JSON |

**Configuration hooks**

| Method | Description |
|---|---|
| `AddReplacer(kind ReplacerKind, r Replacer)` | Register a custom replacer |
| `SetShouldApply(func(Replacer) bool)` | Gate replacers conditionally (used by flare to skip replacers newer than the flare version) |
| `SetPreserveENC(bool)` | When true, single-line replacers skip matches that contain an `ENC[...]` secret reference so that encrypted values survive scrubbing |

### `Replacer`

```go
type Replacer struct {
    Regex        *regexp.Regexp
    YAMLKeyRegex *regexp.Regexp
    ProcessValue func(data interface{}) interface{}
    Hints        []string
    Repl         []byte
    ReplFunc     func(b []byte) []byte
    LastUpdated  *version.Version
}
```

- `Regex` is used for byte-level scrubbing (single- and multi-line passes).
- `YAMLKeyRegex` is used when walking a decoded YAML/JSON object; it matches the map key.
- `ProcessValue` allows custom logic when a YAML key matches (e.g. keep last 4 chars of an API key).
- `Hints` is an optional list of substrings: the regex is only tested if at least one hint appears in the data, which speeds up the common case where a credential is absent.
- `LastUpdated` records the agent version that introduced the replacer; `SetShouldApply` uses this to skip newer replacers when scrubbing older flares.

**`ReplacerKind`** — `SingleLine` or `MultiLine`.

### `DefaultScrubber` and package-level helpers

`DefaultScrubber` is a package-level `*Scrubber` initialised with all default replacers on
`init()`. Package-level functions delegate to it:

| Function | Equivalent to |
|---|---|
| `ScrubFile(path)` | `DefaultScrubber.ScrubFile` |
| `ScrubBytes(data)` | `DefaultScrubber.ScrubBytes` |
| `ScrubString(data)` | `DefaultScrubber.ScrubBytes` (string in/out) |
| `ScrubLine(line)` | `DefaultScrubber.ScrubLine` |
| `ScrubYaml(data)` | `DefaultScrubber.ScrubYaml` |
| `ScrubYamlString(data)` | `DefaultScrubber.ScrubYaml` (string in/out) |
| `ScrubJSON(data)` | `DefaultScrubber.ScrubJSON` |
| `ScrubJSONString(data)` | `DefaultScrubber.ScrubJSON` (string in/out) |
| `ScrubDataObj(data)` | `DefaultScrubber.ScrubDataObj` |

### Default credential patterns

The default replacers cover:

- **Datadog keys**: API keys (32 hex chars), APP keys (40 hex chars), `ddapp_`-prefixed app keys, Remote Config manager keys (`DDRCM_*`). API/APP keys are partially revealed (last 4 chars kept).
- **Passwords**: URL passwords (`user:pass@host`), YAML `password`/`pwd`/`pass` keys, command-line `--password=` arguments.
- **Tokens**: YAML keys ending in `_token` or `_jwt`, Bearer tokens in HTTP headers.
- **Secrets**: YAML keys ending in `_secret`, `_secret_id`, `access_key`.
- **SNMP**: `community_string`, `community_strings` (multiline list), `auth_key`, `priv_key`, `authorization`.
- **TLS certificates**: PEM blocks (`-----BEGIN ... -----` ... `-----END ... -----`).
- **OAuth**: `consumer_key`, `token_id`.
- **HTTP headers**: `x-*-key`, `x-*-token`, `x-*-auth`, `x-*-secret` patterns, plus specific well-known header names.

### Runtime key registration

**`AddStrippedKeys(keys []string)`** adds YAML key patterns to `DefaultScrubber` at runtime
and records them in a global list so that scrubbers created afterwards also inherit them:

```go
// pkg/config/setup/config.go
scrubber.AddStrippedKeys(flareStrippedKeys)          // from flare_stripped_keys config
scrubber.AddStrippedKeys(scrubberAdditionalKeys)     // from additional_endpoints keys

// comp/core/secrets/impl/secrets.go — after resolving secrets
scrubber.AddStrippedKeys([]string{resolvedSecretKey})
```

### Helpers

**`HideKeyExceptLastFourChars(key string) string`** — replaces all but the last 4 characters
with `*`. Returns `"********"` for unrecognised lengths.

**`IsEnc(str string) bool`** — reports whether a string is an `ENC[...]` reference (used
internally to avoid double-scrubbing encrypted values).

## Usage

### Logging

`pkg/util/log` stores `scrubber.ScrubBytes` as its scrub function so that every log message
is automatically cleaned before being written:

```go
// pkg/util/log/log.go
scrubBytesFunc = scrubber.ScrubBytes
```

### Flares

The flare builder uses a dedicated scrubber with `SetPreserveENC(true)` and
`SetShouldApply(...)` to skip replacers newer than the flare's agent version, then calls
`ScrubYaml` on each file added to the archive.

### Check output and agent CLI

```go
// pkg/cli/subcommands/check/command.go
scrubbed, err := scrubber.ScrubBytes(checkFileOutput.Bytes())

// pkg/collector/check/metadata.go
scrubber.ScrubYamlString(c.InstanceConfig())
```

### Metadata payloads

Components that build inventory payloads pre-scrub YAML string fields with
`scrubber.ScrubYamlString` before storing them, because the later `ScrubJSON` pass operates
on JSON key names and cannot inspect opaque string values embedded in the JSON.

### Custom scrubber

For cases where the default replacers are unsuitable, create a fresh instance and add only
what is needed:

```go
s := scrubber.New()
s.AddReplacer(scrubber.SingleLine, scrubber.Replacer{
    Regex: regexp.MustCompile(`my_secret=\S+`),
    Repl:  []byte("my_secret=********"),
})
cleaned, err := s.ScrubBytes(data)
```

## Related packages

- [`pkg/util/log`](log.md) — the agent-wide logging library. Every log message
  is passed through `scrubber.ScrubBytes` before being written. The scrub
  function is stored in the package-level `scrubBytesFunc` variable so tests
  can substitute a no-op scrubber without importing the full package.
- [`comp/core/flare`](../../comp/core/flare.md) — the flare builder wraps
  `pkg/util/scrubber` to clean every file added to the archive. It creates a
  dedicated scrubber with `SetPreserveENC(true)` (to keep `ENC[...]` secret
  handles intact) and `SetShouldApply(...)` (to skip replacers introduced after
  the flare's agent version). See `comp/core/flare/builder` for the
  `FlareBuilder` interface.
- [`pkg/redact`](../redact.md) — a *separate* credential-removal package
  targeted at Kubernetes objects, process command lines, and Custom Resource
  manifests. While `pkg/util/scrubber` focuses on agent config files, YAML/JSON
  payloads, and log output, `pkg/redact` is designed for the
  orchestrator/process collector pipeline. The two packages are not
  interchangeable: use `pkg/util/scrubber` for agent-internal data and
  `pkg/redact` for live Kubernetes/process data.
