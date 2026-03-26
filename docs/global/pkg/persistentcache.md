# pkg/persistentcache

### Purpose

A minimal on-disk key-value cache that lets checks and components persist string state across Agent restarts. Values are stored as plain files under the Agent's `run_path` directory. There is no expiration, no locking, and no serialization beyond raw string content — the caller is responsible for encoding (typically JSON).

### Key elements

All four exported functions share the same key-to-file mapping logic via `GetFileForKey`.

**`GetFileForKey(key string) (string, error)`**

Derives a filesystem path from a cache key. Keys containing `:` are split on the first colon:

- The part before `:` becomes a subdirectory under `run_path` (created if absent, mode `0700`).
- The part after `:` becomes the filename.

Keys without `:` map directly to a file in `run_path`. In both cases, invalid characters (anything outside `[a-zA-Z0-9_-]`) are stripped before constructing the path.

Check IDs follow the convention `$check_name:$hash`, so e.g. `snmp:abc123` maps to `<run_path>/snmp/abc123`.

**`Write(key, value string) error`**

Writes `value` to the file at `GetFileForKey(key)` with mode `0600`.

**`Read(key string) (string, error)`**

Returns the file content as a string, or `""` if the file does not exist (not an error). Returns an error only on I/O failure.

**`Exists(key string) bool`**

Returns `true` if the cache file exists.

**`Rename(oldKey, newKey string) error`**

Atomically renames the cache file. Used to migrate legacy cache keys without losing data.

### Usage

The cache is used by several components to persist state across restarts:

- **SNMP autodiscovery** (`comp/core/autodiscovery/listeners/snmp.go`): caches discovered subnet device lists. Legacy keys (without `:`) are migrated to the new `snmp:<subnet>` format via `Rename` on first access.
- **SNMP scan manager** (`comp/snmpscanmanager/impl/snmpscanmanager.go`): persists scan results and reads them back on startup.
- **Logon duration** (`comp/logonduration/impl`): stores the last recorded boot time on Windows and macOS to compute session duration across restarts.
- **Python checks** (`pkg/collector/python/datadog_agent.go`): exposes `persistentcache.Read` and `persistentcache.Write` to Python checks via the rtloader C bindings, allowing Python-based checks to use the same cache.

Typical pattern:

```go
// Write
if err := persistentcache.Write("snmp:192.168.1.0_24", string(jsonBytes)); err != nil {
    log.Warnf("failed to persist cache: %v", err)
}

// Read (empty string means no cached data)
cached, err := persistentcache.Read("snmp:192.168.1.0_24")
if err != nil || cached == "" {
    // no prior state
}
```

The cache location is determined by the `run_path` configuration key (typically `/opt/datadog-agent/run` on Linux). No cleanup of stale files is performed automatically; callers must manage their own keys.

### Related packages

| Package | Relationship |
|---------|-------------|
| [`pkg/snmp`](snmp.md) | The SNMP autodiscovery listener (`comp/core/autodiscovery/listeners/snmp.go`) caches discovered subnet device lists using keys of the form `snmp:<subnet>`. At first access it migrates legacy flat keys (no colon) to the namespaced format via `Rename`. The SNMP scan manager (`comp/snmpscanmanager/impl`) separately persists scan results and reads them back on startup using the same package. |
| [`pkg/jmxfetch`](jmxfetch.md) | JMXFetch itself does not use `pkg/persistentcache` directly, but the JMX check instances that pass through the collector may be Python-based and could use the cache via the Python binding described below. |
| [`pkg/logonduration`](logonduration.md) | `comp/logonduration/impl` stores the last recorded boot time on Windows and macOS using `persistentcache.Write` / `persistentcache.Read`. This lets the component compute session duration correctly even after an agent restart. |
| [`pkg/collector/python`](collector/python.md) | `pkg/collector/python/datadog_agent.go` exposes `read_persistent_cache` and `write_persistent_cache` to Python checks via the `datadog_agent` rtloader callback module. This allows Python integration checks to persist arbitrary string state (typically JSON-encoded) across agent restarts using the same on-disk store as Go components. |
