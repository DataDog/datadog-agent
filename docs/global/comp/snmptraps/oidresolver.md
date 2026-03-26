> **TL;DR:** `comp/snmptraps/oidresolver` translates numeric SNMP OIDs into human-readable trap names and variable metadata (including enum mappings) by loading MIB database files at startup, with user-provided files overriding Datadog's built-in databases.

# comp/snmptraps/oidresolver

**Package:** `github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver`
**Team:** network-device-monitoring-core

## Purpose

The OID resolver component translates numeric SNMP Object Identifiers (OIDs) into human-readable names and metadata extracted from MIB databases. This enrichment lets the formatter produce self-describing JSON payloads (e.g., `"snmpTrapName": "linkDown"` instead of `"snmpTrapOID": "1.3.6.1.6.3.1.1.5.3"`) and expand integer-coded variable values into symbolic strings.

The component loads one or more trap database files at startup and holds the result in memory for fast synchronous lookups during trap processing.

## Key elements

### Key interfaces

```go
// comp/snmptraps/oidresolver/component.go
type Component interface {
    GetTrapMetadata(trapOID string) (TrapMetadata, error)
    GetVariableMetadata(trapOID string, varOID string) (VariableMetadata, error)
}
```

- `GetTrapMetadata(trapOID)` — returns the name, MIB name, and description for a known trap OID.
- `GetVariableMetadata(trapOID, varOID)` — returns the name, description, and optional enum/bits mappings for a variable bound to a specific trap. The `trapOID` argument is used to select the correct MIB file when the same variable OID appears in multiple databases, ensuring variable definitions are paired with the trap definition from the same file.

Both methods return an error if the OID is not found.

### Key types

Defined in `comp/snmptraps/oidresolver/traps_db.go`:

```go
type TrapMetadata struct {
    Name            string
    MIBName         string
    Description     string
    VariableSpecPtr VariableSpec   // pointer to variables from the same file
}

type VariableMetadata struct {
    Name               string
    Description        string
    Enumeration        map[int]string   // integer → symbolic name (e.g., 1 → "up")
    Bits               map[int]string   // bit position → symbolic name
    IsIntermediateNode bool             // internal: OID is a tree node, not a leaf
}

type TrapDBFileContent struct {
    Traps     TrapSpec     // map[OID string]TrapMetadata
    Variables VariableSpec // map[OID string]VariableMetadata
}
```

Helper functions also defined here:

- `NormalizeOID(value string) string` — strips a leading dot, converting `.1.2.3` to `1.2.3`. All OIDs are stored and compared in the normalized (relative) form.
- `IsValidOID(value string) bool` — validates that a string contains only digits and dots with no trailing dot or adjacent dots.

### Key functions

**`multiFilesOIDResolver`** — production implementation (`oidresolverimpl/oid_resolver.go`).

### Configuration and build flags

**Database file loading**

At construction, the resolver scans `<confd_path>/snmp.d/traps_db/` for JSON and YAML files (optionally gzip-compressed, with a `.gz` suffix). Files are processed in a specific priority order:

1. Files whose name starts with `dd_traps_db` (Datadog-shipped databases) are loaded first, in alphabetical order.
2. User-provided files are loaded afterwards, in alphabetical order.

This ordering means user-provided files can override Datadog's built-in definitions for the same OID.

**Conflict resolution**

When the same trap OID appears in multiple files, the later-loaded file wins (user files over Datadog files). A debug-level log message is emitted for each conflict.

Variable OID conflicts are resolved transitively: a trap's `VariableSpecPtr` always points to the variable spec parsed from the same file as the trap, so a variable OID that has different meanings in two different MIB files will be resolved using the definition from the file that also defined the current trap.

**OID tree climbing**

`GetVariableMetadata` performs a "tree climbing" lookup: if the exact variable OID is not found, it strips the last component and retries. This handles cases where a received OID is a sub-node of a known variable (e.g. `1.3.6.1.2.1.2.2.1.1.2` resolving via `1.3.6.1.2.1.2.2.1.1`). The climb stops when it finds a match or when it reaches an OID marked as an intermediate node (known to be a tree branch rather than a leaf).

**Intermediate node detection**

When loading a file, all variable OIDs are sorted lexicographically and compared with their successor. If OID B starts with `<OID_A>.`, then OID A is marked as an intermediate node and will never be returned as a `GetVariableMetadata` result.

### Mock

`oidresolverimpl/mock.go` provides `MockOIDResolver`, which returns configurable responses. It is intended for unit tests that need to exercise the formatter or forwarder without loading real trap DB files.

## Usage

The OID resolver is consumed by the formatter component only:

```
listener → forwarder → formatter
                           ↑
                       oidresolver.GetTrapMetadata / GetVariableMetadata
```

It is registered as part of the SNMP traps server's inner fx application in `comp/snmptraps/server/serverimpl`:

```go
oidresolverimpl.Module()
```

**Adding custom MIB data**

Place a YAML or JSON file (optionally `.gz`) in the agent's `conf.d/snmp.d/traps_db/` directory. Files must not be prefixed with `dd_traps_db` to be treated as user-provided and loaded after Datadog's built-in database. The file format mirrors `TrapDBFileContent`:

```yaml
traps:
  "1.3.6.1.4.1.12345.1":
    name: myTrap
    mib: MY-MIB
    descr: "An example trap"
vars:
  "1.3.6.1.4.1.12345.1.1":
    name: myVariable
    descr: "Status variable"
    enum:
      1: "active"
      2: "inactive"
```
