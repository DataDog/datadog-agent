# pkg/util/dmi

## Purpose

`pkg/util/dmi` exposes a small set of read-only helpers that retrieve
hardware identity information from the DMI/SMBIOS subsystem exposed by the
Linux kernel under `/sys/devices/virtual/dmi/id/` and from the Xen hypervisor
UUID at `/sys/hypervisor/uuid`. The package is used primarily to detect
whether the agent is running on an AWS EC2 instance without making an HTTP
call to the instance metadata service.

## Key elements

### Platform split

| Build constraint | File | Behaviour |
|---|---|---|
| `!windows && !serverless` | `dmi_nix.go` | Reads from sysfs; live implementation |
| `windows \|\| serverless` | `no_dmi.go` | Returns empty strings for all functions |

There is no macOS-specific implementation. On macOS the `dmi_nix.go` file
compiles (build constraint does not exclude it) but will silently return empty
strings because the sysfs paths do not exist.

### Functions

All functions are package-level, take no arguments, and return a `string`.
An empty string is returned on any error (file not found, permission denied,
etc.).

| Function | sysfs path | Description |
|---|---|---|
| `GetProductUUID()` | `/sys/devices/virtual/dmi/id/product_uuid` | SMBIOS product UUID; present on most bare-metal and virtualised Linux hosts |
| `GetHypervisorUUID()` | `/sys/hypervisor/uuid` | Xen hypervisor UUID; available on Xen-based instances (older EC2 instance types) |
| `GetBoardVendor()` | `/sys/devices/virtual/dmi/id/board_vendor` | Board/chassis manufacturer string (e.g. `"Amazon EC2"`) |
| `GetBoardAssetTag()` | `/sys/devices/virtual/dmi/id/board_asset_tag` | Board asset tag; on AWS Nitro instances this equals the EC2 instance ID (e.g. `"i-0abcdef1234567890"`) |

Trailing newlines are stripped automatically.

### Mocking

`dmi_mock.go` and `no_dmi_mock.go` expose package-level variables
(`boardAssetTag`, `boardVendor`, `productUUID`, `hypervisorUUID`) that can be
overwritten in tests to inject fixture values without touching the filesystem.

## Usage

### AWS EC2 instance detection (`pkg/util/ec2`)

The primary consumer is `pkg/util/ec2/dmi.go`, which gates on the
`ec2_use_dmi` configuration key.

**Instance ID from DMI (Nitro instances):**

```go
// pkg/util/ec2/dmi.go
if dmi.GetBoardVendor() == DMIBoardVendor {   // "Amazon EC2"
    tag := dmi.GetBoardAssetTag()
    if strings.HasPrefix(tag, "i-") {
        return tag, nil                        // e.g. "i-0abc123def456"
    }
}
```

**EC2 UUID heuristic (Xen / older instance types):**

```go
// dmi.GetProductUUID() or dmi.GetHypervisorUUID()
// If the UUID starts with "ec2" (case-insensitive), the host is EC2.
// Handles little-endian SMBIOS UUID encoding by swapping bytes if needed.
uuidData := dmi.GetProductUUID()
if uuidData == "" {
    uuidData = dmi.GetHypervisorUUID()
}
if strings.HasPrefix(strings.ToLower(uuidData), "ec2") {
    // confirmed EC2
}
```

### Inventory host metadata (`comp/metadata/inventoryhost`)

`comp/metadata/inventoryhost/inventoryhostimpl/inventoryhost.go` includes DMI
fields in the `host_metadata` payload sent to the Datadog backend, giving
Datadog visibility into the underlying hardware platform.

## Notes

- Reading sysfs DMI files requires that the agent run as root or that the
  files are world-readable. In restrictive container environments the files
  may be absent or permission-denied; all functions degrade gracefully to
  empty strings.
- `ec2_use_dmi` defaults to `true`. Set it to `false` to disable DMI-based
  EC2 detection entirely (e.g. in environments where sysfs is unavailable or
  misleading).
- The package has no dependencies on any other agent package and is safe to
  import anywhere.

## Related packages

| Package / component | Relationship |
|---|---|
| [`pkg/util/ec2`](ec2.md) | The primary consumer of this package. `pkg/util/ec2/dmi.go` calls all four DMI functions to drive `IsRunningOn` (board vendor / product UUID / hypervisor UUID checks) and `GetInstanceID` (board asset tag on Nitro instances). DMI is consulted after IMDS fails or when `ec2_imdsv2_transition_payload_enabled` forces a DMI fallback path. |
| [`comp/metadata/inventoryhost`](../../comp/metadata/inventoryhost.md) | Includes DMI fields directly in the `host_metadata` inventory payload: `hypervisor_guest_uuid` (from `GetHypervisorUUID`), `dmi_product_uuid` (from `GetProductUUID`), `dmi_board_asset_tag` (from `GetBoardAssetTag`), and `dmi_board_vendor` (from `GetBoardVendor`). These fields give Datadog visibility into the underlying hardware platform for every inventoried host. |
