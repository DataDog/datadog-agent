# pkg/collector/externalhost

**Import path:** `github.com/DataDog/datadog-agent/pkg/collector/externalhost`

## Purpose

Implements the External Host Tags metadata provider. Checks push hostname-to-tag mappings into a global cache; the metadata collector reads and clears that cache when building the `agent_check` metadata payload sent to the Datadog intake.

The model is push-based: checks call `SetExternalTags` at runtime rather than the agent polling checks for extra tags. See `docs/proposal/metadata/external-host-tags.md` for the original design rationale.

## Key Elements

### Types (`payload.go`)

| Type | Definition | Description |
|---|---|---|
| `ExternalTags` | `map[string][]string` | Maps a source type (e.g. `"vsphere"`) to a list of tag strings |
| `Payload` | `[]hostTags` | The full metadata payload — a list of `[hostname, ExternalTags]` pairs, JSON-serializable |
| `hostTags` | `[]interface{}` | Internal: `[hostname string, ExternalTags map]` tuple |

### Cache and functions (`externalhost.go`)

The cache is a two-level map `map[sourceType]map[hostname]ExternalTags`, protected by `cacheMutex`.

**`SetExternalTags(hostname, sourceType string, tags []string)`**
Upserts the tags for a `(sourceType, hostname)` pair. Callers:
- `pkg/collector/python/datadog_agent.go` — Python checks via the `datadog_agent` module
- `pkg/collector/corechecks/snmp/internal/devicecheck/devicecheck.go` — the SNMP core check

**`GetPayload() *Payload`**
Drains the entire cache into a `Payload` and resets it. Called by the metadata collector at each collection cycle so stale entries do not accumulate.

## Usage

Typical flow:

1. A check (Python or Go) calls `SetExternalTags("myhost.example.com", "vsphere", []string{"datacenter:dc1", "cluster:prod"})` after discovering host-level tags from an external source.
2. At metadata collection time (every 10 minutes by default), `comp/collector/collector/collectorimpl/agent_check_metadata.go` calls `GetPayload()` and embeds the result in the `agent_check` metadata payload under the `external_host_tags` key.
3. The payload is forwarded to the Datadog intake, where the external tags are attached to the relevant host.

The cache is cleared on every `GetPayload` call, so tags that are not re-submitted before the next collection cycle will disappear from the payload — checks are expected to re-push their external tags on each run.

## Related packages

| Package | Relationship |
|---------|-------------|
| [`pkg/collector/check`](check.md) | Checks (both Go and Python) call `SetExternalTags` during `Run()`. The call originates inside a `check.Check` implementation; `pkg/collector/externalhost` has no dependency on the `check` package itself — it is a pure push target that any code may call. |
| [`pkg/collector/python`](python.md) | `pkg/collector/python/datadog_agent.go` exposes `set_external_tags` to the Python runtime as a callback in the `datadog_agent` module. Python checks call `datadog_agent.set_external_tags([(host, {source: tags})])` which forwards to `externalhost.SetExternalTags`. |
| [`comp/metadata/host`](../../comp/metadata/host.md) | The `comp/metadata/host` component is responsible for the periodic host metadata payload sent to the Datadog intake. External host tags collected here are embedded under the `external_host_tags` key of the `agent_check` metadata payload by `comp/collector/collector/collectorimpl/agent_check_metadata.go`, which calls `GetPayload()` at each 10-minute collection cycle. |
