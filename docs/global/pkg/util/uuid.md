# pkg/util/uuid

## Purpose

`pkg/util/uuid` provides a single function that retrieves a stable, platform-specific UUID for
the host. The UUID is cached in memory after the first successful lookup so that subsequent calls
are effectively free.

This UUID is used to uniquely identify the agent instance in inventory and metadata payloads sent
to the Datadog backend.

## Key elements

**`GetUUID() string`**

Returns the host UUID. The result is cached via `pkg/util/cache` (key
`"agent/host/utils/uuid"`) so the OS call is only made once per agent lifetime.

Returns an empty string if the UUID cannot be determined (the error is logged but not returned to
callers).

The implementation is platform-specific:

| Platform | Source |
|---|---|
| Linux / macOS | `gopsutil/v4/host.Info().HostID` — reads `/etc/machine-id` (Linux), `IOPlatformUUID` (macOS), or similar OS facility |
| Windows | `HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid` registry value |

`GetUUID` is exposed as a package-level variable (not a direct function reference) so that tests
can substitute a stub:

```go
var GetUUID = getUUID
```

## Usage

`GetUUID()` is called by every metadata payload component that needs to tag the agent instance:

```go
// comp/metadata/inventoryagent/inventoryagentimpl/inventoryagent.go
Payload{
    UUID: uuid.GetUUID(),
    ...
}
```

It appears in the following metadata payloads:

- `inventoryagent` — `datadog_agent` inventory payload
- `inventoryhost` — host inventory payload
- `inventorychecks` — checks inventory payload
- `hostsysteminfo` — host system info payload
- `clusterchecks` — cluster-checks metadata
- `packagesigning` — package signing metadata
- `host` — general host metadata (`comp/metadata/host`)

It also supplies the UUID for security agent policy evaluation rules:

```go
// pkg/security/rules/engine.go
"uuid:" + uuid.GetUUID()
```

## Notes

- The UUID is a property of the host OS, not of the agent binary. It remains stable across agent
  restarts and upgrades as long as the host OS installation is unchanged.
- On Linux the value comes from `/etc/machine-id` (or `/var/lib/dbus/machine-id` as a fallback)
  via gopsutil. If that file is missing or empty, `GetUUID()` returns `""`.
- The cache key is built with `cache.BuildAgentKey("host", "utils", "uuid")` — do not bypass the
  cache by calling the internal `getUUID` function directly in production code.

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `comp/metadata/inventoryhost` | [inventoryhost.md](../../comp/metadata/inventoryhost.md) | Embeds `GetUUID()` in the top-level `Payload.UUID` field of the structured `host_metadata` inventory payload sent to `/api/v2/host_metadata`. This is the primary consumer of the UUID for inventory reporting. |
| `comp/metadata/host` | [host.md](../../comp/metadata/host.md) | Embeds `GetUUID()` in `utils.CommonPayload.uuid` which is part of the legacy "v5" host metadata payload (`/intake`). The UUID is included as the `uuid` field alongside `apiKey`, `agentVersion`, and `internalHostname`. |
