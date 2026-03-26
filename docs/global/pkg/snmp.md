# pkg/snmp — SNMP shared utilities

## Purpose

`pkg/snmp` provides the shared data types, configuration parsing, protocol helpers, and device-management utilities used by every SNMP-related component in the agent: the SNMP check (`pkg/collector/corechecks/snmp`), the SNMP scan manager (`comp/snmpscanmanager`), the SNMP traps component (`comp/snmptraps`), and the `snmpwalk` CLI command.

The package has **no external SNMP network logic of its own**; it is a library of building blocks consumed by higher-level components.

---

## Key elements

### Root package (`pkg/snmp`)

Parses and holds the SNMP autodiscovery (listener) configuration.

| Symbol | Description |
|--------|-------------|
| `ListenerConfig` | Top-level struct for `network_devices.autodiscovery` / `snmp_listener`. Controls workers, discovery interval, allowed failures, loader, topology/VPN collection, deduplication, and global defaults for all subnet configs. |
| `Config` | Per-subnet configuration: network CIDR, port, SNMP version, credentials, loader, namespace, ignored IPs, `InterfaceConfigs`, and `PingConfig`. |
| `UnmarshalledConfig` | Raw mapstructure from `datadog.yaml`; used only during `NewListenerConfig()`. Supports legacy field names (e.g. `community` → `community_string`). |
| `Authentication` | A single set of SNMP credentials (version, community, USM user/keys/protocols, context). A `Config` may carry multiple `Authentications` to try in order. |
| `NewListenerConfig()` | Reads `network_devices.autodiscovery` or `snmp_listener` from the agent config, resolves legacy field names, fills defaults (port 161, inheriting global timeout/retries), and returns a ready-to-use `ListenerConfig`. Returns `ErrNoConfigGiven` when neither key is present. |
| `Config.Digest(address)` | FNV-64 hash of all `Authentication` fields plus port, loader, namespace, and ignored IPs. Used to detect configuration changes for a device. |
| `Config.LegacyDigest(address)` | Pre-v7.76 variant of `Digest` using a flat credential set. Keep until Agent 7.76+. |
| `Config.IsIPIgnored(ip)` | Returns true if `ip` is in `IgnoredIPAddresses`. |
| `Authentication.BuildSNMPParams(deviceIP, port)` | Constructs a `*gosnmp.GoSNMP` ready to use. Infers the SNMP version from the fields (community → v2c, user → v3) and maps auth/priv protocol strings to gosnmp constants via `gosnmplib`. |

### `gosnmplib/`

Low-level helpers wrapping the `github.com/gosnmp/gosnmp` library.

| Symbol | Description |
|--------|-------------|
| `GetAuthProtocol(s)` | Converts a protocol name string (`"md5"`, `"sha"`, `"sha256"`, …) to `gosnmp.SnmpV3AuthProtocol`. |
| `GetPrivProtocol(s)` | Converts a privacy protocol name (`"des"`, `"aes"`, `"aes256c"`, …) to `gosnmp.SnmpV3PrivProtocol`. Supports both Blumenthal and Reeder variants of AES-192/256. |
| `GetValueFromPDU(pdu)` | Converts a `gosnmp.SnmpPDU` to a Go native type: `[]byte` for OctetString/BitString, `float64` for all integer types, `string` for IPs and OIDs (leading `.` stripped). |
| `StandardTypeToString(v)` | Converts the output of `GetValueFromPDU` to a string; binary data is hexified in pysnmp-compatible format. |
| `PacketAsString(packet)` | Serializes a `*gosnmp.SnmpPacket` to a JSON-like string for debug/trace logging. |
| `PDU` struct | A serializable wrapper for a single SNMP PDU (OID + ASN.1 type + value). OctetString values are base64-encoded in `Value`. |
| `PDUFromSNMP(pdu)` | Wraps a `gosnmp.SnmpPDU` into a `PDU`. |
| `PDU.RawValue()` | Decodes `PDU.Value` back to the appropriate Go type. |
| `ConditionalWalk(ctx, session, rootOID, useBulk, …, walkFn)` | A context-aware OID walk (GET-NEXT or GET-BULK). The callback can return a next OID to jump to, enabling row-skipping. Detects infinite cycles. |
| `SkipOIDRowsNaive(oid)` | Returns the OID of the next table column (without a MIB). Used by `ConditionalWalk` callbacks to skip over repeated table rows. |
| `OIDToInts(oid)` | Parses a dotted OID string to `[]int`. |
| `CmpOIDs(oid1, oid2)` | Returns an `OIDRelation` (EQUAL, GREATER, LESS, PARENT, CHILD) for OID ordering comparisons. |
| `OIDRelation.IsAfter()` / `IsBefore()` | Convenience methods on `OIDRelation`. |
| `NewConnectionError(err)` | Wraps a network error so callers can distinguish connection failures from protocol errors. |

### `snmpintegration/`

Shared integration-level configuration types, consumed by both the SNMP check autodiscovery and the listener.

| Symbol | Description |
|--------|-------------|
| `InterfaceConfig` | Per-interface overrides: `match_field`, `match_value`, speed overrides (`in_speed`, `out_speed`), tags, and a `disabled` flag. |
| `PingConfig` | ICMP ping settings bundled with a device: `Enabled`, `Interval`, `Timeout`, `Count`, and `Linux.UseRawSocket`. |
| `PackedPingConfig` | A `PingConfig` that can unmarshal from either a YAML map or a JSON-encoded string (needed for autodiscovery template annotations). |

### `snmpparse/`

Utilities for extracting `SNMPConfig` values from the running agent (used by CLI tools such as `snmpwalk`).

| Symbol | Description |
|--------|-------------|
| `SNMPConfig` | Flat, human-readable config struct covering all SNMP versions; mirrors the YAML check instance format. |
| `SetDefault(sc)` | Fills standard defaults (port 161, timeout 2, retries 3). |
| `ParseConfigSnmp(c integration.Config)` | Extracts all `SNMPConfig` instances from an autodiscovery `integration.Config` by YAML-unmarshalling each instance blob. |
| `GetConfigCheckSnmp(conf, client)` | Queries the local agent's `/agent/config-check` endpoint and returns all active SNMP configs (both autodiscovery and listener subnets). |
| `GetIPConfig(ip, configs)` | Finds the best `SNMPConfig` for a given IP: exact match first, then subnet containment. |
| `GetParamsFromAgent(deviceIP, conf, client)` | One-call helper that retrieves the config for a device IP from the running agent, including namespace resolution. |

### `devicededuper/`

Prevents the listener from registering the same physical device twice when it has multiple IP addresses.

| Symbol | Description |
|--------|-------------|
| `DeviceDeduper` interface | `MarkIPAsProcessed(ip)`, `AddPendingDevice(device)`, `GetDedupedDevices() []PendingDevice`, `ResetCounters()`. |
| `NewDeviceDeduper(config)` | Creates an implementation that pre-initializes per-IP atomic counters for every IP in every configured subnet. |
| `DeviceInfo` | The fingerprint of a device: `Name`, `Description`, `SysObjectID`, `BootTimeMs`. Two `DeviceInfo` values are equal if all string fields match and boot times differ by less than `uptimeDiffToleranceMs` (5 s). |
| `PendingDevice` | Pairs a `DeviceInfo` with its `Config`, `IP`, auth index, and failure count. |
| `AddPendingDevice(device)` | Inserts a device into the pending set, keeping only the lowest IP address among duplicates. |
| `GetDedupedDevices()` | Returns devices whose lower-numbered IPs have all been scanned, removing them from the pending set. Devices are promoted only after all preceding IPs in the subnet have been processed. |
| `MarkIPAsProcessed(ip)` | Decrements the counter for `ip`; triggers promotion of pending devices once all earlier IPs are seen. |
| `ResetCounters()` | Resets all counters and the known-device list for the next discovery cycle. |
| `IncrementIP(ip net.IP)` | Increments a `net.IP` in-place by 1; used by `initializeCounters` to walk subnets. |

### `constants.go` / `utils/`

Minor shared constants and utility functions (e.g. `firstNonEmpty`, `firstNonNil`).

---

## Usage

### Building a GoSNMP session

```go
// pkg/collector/corechecks/snmp/internal/session/session.go (pattern)
snmpParams, err := authentication.BuildSNMPParams(deviceIP, config.Port)
if err != nil {
    return err
}
snmpParams.Connect()
defer snmpParams.Conn.Close()
```

### Walking an OID tree

```go
err := gosnmplib.ConditionalWalk(ctx, snmpSession, ".1.3.6.1.2.1", true, 0, 0,
    func(pdu gosnmp.SnmpPDU) (string, error) {
        val, err := gosnmplib.GetValueFromPDU(pdu)
        // process val ...
        return "", err
    },
)
```

### Deduplicating discovered devices

```go
deduper := devicededuper.NewDeviceDeduper(listenerConfig)
// for each scanned IP:
deduper.MarkIPAsProcessed(ip)
if deviceFound {
    deduper.AddPendingDevice(devicededuper.PendingDevice{IP: ip, Info: info, Config: cfg})
}
// at end of subnet sweep:
for _, d := range deduper.GetDedupedDevices() {
    activateDevice(d)
}
deduper.ResetCounters()
```

### Resolving a device IP from CLI tools

```go
cfg, namespace, err := snmpparse.GetParamsFromAgent(deviceIP, conf, ipcClient)
// cfg is ready to pass to Authentication.BuildSNMPParams
```

---

## Configuration keys

| Key | Description |
|-----|-------------|
| `network_devices.autodiscovery` | Primary key for listener config (preferred) |
| `snmp_listener` | Legacy key for listener config |
| `network_devices.namespace` | Default namespace for NDM resources |
| `network_devices.autodiscovery.use_deduplication` | Enable device deduplication |
| `network_devices.autodiscovery.use_remote_config_profiles` | Use remote profile configurations |

---

## Related documentation

| Document | Relationship |
|----------|-------------|
| [`pkg/networkdevice`](networkdevice/networkdevice.md) | Defines the metadata payload types (`NetworkDevicesMetadata`, `ProfileDefinition`, etc.) that the SNMP check produces after collecting data with the session helpers from `pkg/snmp`. `pkg/snmp` builds the session; `pkg/networkdevice` defines what gets sent. |
| [`comp/snmptraps/config`](../comp/snmptraps/config.md) | Parses and provides `TrapsConfig` for the SNMP traps listener. `TrapsConfig.BuildSNMPParams()` and `UserV3` mirror the credential model in `pkg/snmp`'s `Authentication` / `gosnmplib`. |
| [`comp/snmptraps/listener`](../comp/snmptraps/listener.md) | Consumes `TrapsConfig` (built on `pkg/snmp` gosnmp helpers) to open the trap UDP socket. Validates community strings using the same constant-time comparison pattern recommended for v1/v2c credentials in `pkg/snmp`. |

### Package dependency map

```
pkg/snmp
  ├─ gosnmplib/          ← low-level gosnmp wrappers (PDU conversion, OID walk)
  ├─ snmpintegration/    ← InterfaceConfig, PingConfig (shared by check + listener)
  ├─ snmpparse/          ← CLI/snmpwalk helpers (reads running agent configs)
  └─ devicededuper/      ← deduplication for autodiscovery subnet sweeps

pkg/collector/corechecks/snmp   ← main consumer (session, walk, profiledefinition)
comp/snmptraps/                 ← trap listener / forwarder (credentials, gosnmp params)
comp/snmpscanmanager/           ← uses ListenerConfig subnets for scan scheduling
pkg/networkdevice/              ← profile definitions, metadata types, sender
```

### Relationship between `pkg/snmp` and `comp/snmptraps`

`pkg/snmp` and the `comp/snmptraps` subsystem share the same underlying gosnmp library but serve different protocols:

- `pkg/snmp` handles **polling** (SNMP GET/GETBULK/GETNEXT) initiated by the agent against devices.
- `comp/snmptraps` handles **trap reception** — unsolicited UDP datagrams pushed by devices to the agent.

Both use `gosnmplib.GetAuthProtocol` / `GetPrivProtocol` for USM auth/priv protocol resolution, and both use the `network_devices.namespace` value (set at the `ListenerConfig` or `TrapsConfig` level) to tag all resources consistently.

To configure both polling and trap reception for the same device, a typical `datadog.yaml` fragment looks like:

```yaml
network_devices:
  namespace: prod
  autodiscovery:
    workers: 100
    discovery_interval: 3600
    configs:
      - network_address: 10.0.0.0/24
        snmp_version: "2"
        community_string: "public"
  snmp_traps:
    enabled: true
    port: 9162
    community_strings:
      - "public"
```
