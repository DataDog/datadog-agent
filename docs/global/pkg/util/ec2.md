# pkg/util/ec2

## Purpose

`pkg/util/ec2` provides everything the agent needs to interact with AWS EC2 Instance Metadata Service (IMDS) and the EC2 API. It is responsible for:

- Detecting whether the agent is running on EC2 (via IMDS, DMI, or product/hypervisor UUID).
- Fetching core instance metadata: hostname, instance ID, instance type, region, account ID, public IPv4, network/VPC information.
- Supporting both IMDSv1 and IMDSv2 (token-based) with a configurable preference, a DMI fallback, and a transition payload mode.
- Retrieving EC2 instance tags (via IMDS tag endpoint or the EC2 API with IAM role credentials).
- Providing utilities for ECS/Kubernetes integration: cluster name extraction, container instance ARN, host CCRID (ARN).
- Checking spot instance lifecycle and scheduled termination.

Results from IMDS are cached by individual `cachedfetch.Fetcher` instances to avoid repeated HTTP calls within the same agent run.

### Relationship to other packages

| Package | Role |
|---|---|
| [`pkg/util/cloudproviders`](cloudproviders.md) | The fan-out entry point. `cloudproviders.DetectCloudProvider` calls `ec2.IsRunningOn` and `ec2.GetAccountID`; `GetHostAliases`, `GetHostCCRID`, `GetInstanceType`, and `GetSpotTerminationTime` all delegate to `pkg/util/ec2` when `"AWS"` is the detected provider. |
| [`pkg/util/ecs`](ecs.md) | ECS-specific metadata (task ARN, cluster name via TMDS) is handled by `pkg/util/ecs`. `pkg/util/ec2/tags.GetContainerInstanceARN` (requires `docker` build tag) bridges the two packages by fetching the ECS container instance ARN from the EC2 introspection path. |
| [`pkg/util/aws`](aws.md) | `pkg/util/aws/creds` is a parallel AWS helper that provides IAM credential retrieval and SigV4 signing. It intentionally duplicates the IMDS HTTP layer (`ec2internal`) due to Go `internal` visibility rules; the two packages do not import each other. |
| [`pkg/util/dmi`](dmi.md) | Provides the DMI/SMBIOS sysfs helpers (`GetBoardVendor`, `GetBoardAssetTag`, `GetProductUUID`, `GetHypervisorUUID`) used by `ec2/dmi.go` for the DMI-based EC2 detection and instance-ID fallback path on Nitro instances. |

## Build tags

| Package / file | Build tag required |
|---|---|
| `pkg/util/ec2` (root) | none (always compiled) |
| `pkg/util/ec2/tags/ec2_tags.go` | `ec2` |
| `pkg/util/ec2/tags/ec2_no_tags.go` | `!ec2` (stub) |
| `pkg/util/ec2/tags/container_instance_arn.go` | `docker` |
| `pkg/util/ec2/tags/container_instance_arn_nodocker.go` | `!docker` (stub) |

The `ec2` build tag is required to compile the AWS SDK-backed tag fetching. Without it `GetTags` and `GetInstanceInfo` are no-ops.

## Key elements

### Constants and detection (`ec2.go`, `internal/helpers.go`)

| Identifier | Description |
|---|---|
| `CloudProviderName` | `"AWS"` — used by cloud provider detection logic |
| `DMIBoardVendor` | `"Amazon EC2"` — DMI board vendor string for EC2 detection |
| `MetadataURL` | `http://169.254.169.254/latest/meta-data` (internal) |
| `TokenURL` | `http://169.254.169.254/latest/api/token` (internal) |
| `InstanceIdentityURL` | `http://169.254.169.254/latest/dynamic/instance-identity/document/` (internal) |

```go
func IsRunningOn(ctx context.Context) bool
```

Returns `true` if the agent is on EC2. Checked in order: IMDS hostname reachable, DMI board vendor / product UUID matches EC2, `ECSEC2` or `ECSFargate` environment features are present.

### IMDSv1 / IMDSv2 versioning (`internal/imds_helpers.go`)

```go
type Ec2IMDSVersionConfig int  // ImdsV1 | ImdsAllVersions | ImdsV2

func UseIMDSv2() Ec2IMDSVersionConfig  // reads ec2_prefer_imdsv2 / ec2_imdsv2_transition_payload_enabled
func GetMetadataItem(ctx, endpoint, versions, updateSource) (string, error)
func GetMetadataItemWithMaxLength(ctx, endpoint, versions, updateSource) (string, error)
func DoHTTPRequest(ctx, url, versions, updateSource) (string, error)
```

`DoHTTPRequest` acquires an IMDSv2 token automatically when `V2Allowed()` is true. If the token fetch fails and `ImdsV1` is still permitted, it falls back silently. The best metadata source observed so far is tracked via `SetCloudProviderSource` / `GetSourceName` (values: `UUID`, `DMI`, `IMDSv1`, `IMDSv2`).

### Instance identity

```go
type EC2Identity struct {
    Region     string
    InstanceID string
    AccountID  string
}

func GetInstanceIdentity(ctx context.Context) (*EC2Identity, error)
func GetAccountID(ctx context.Context) (string, error)
func GetRegion(ctx context.Context) (string, error)
```

`GetInstanceIdentity` calls the `instance-identity/document` endpoint and parses the JSON result. `GetAccountID` and `GetRegion` are cached wrappers around it.

### Hostname / instance ID resolution (`ec2.go`)

```go
func GetHostname(ctx context.Context) (string, error)
func GetInstanceID(ctx context.Context) (string, error)
func GetIDMSv2InstanceID(ctx context.Context) (string, error)
func GetLegacyResolutionInstanceID(ctx context.Context) (string, error)
func GetHostID(ctx context.Context) string
func GetHostAliases(ctx context.Context) ([]string, error)
func GetInstanceType(ctx context.Context) (string, error)
func IsDefaultHostname(hostname string) bool
func IsDefaultHostnameForIntake(hostname string) bool
func IsWindowsDefaultHostname(hostname string) bool
```

`GetInstanceID` uses the configured IMDS version and falls back to DMI when the `ec2_imdsv2_transition_payload_enabled` flag is set. `IsDefaultHostname` recognises `ip-*`, `domu*`, and (optionally) `ec2amaz-*` prefixes that indicate an AWS-assigned hostname.

### Network helpers (`network.go`)

```go
func GetPublicIPv4(ctx context.Context) (string, error)
func GetNetworkID(ctx context.Context) (string, error)   // VPC ID
func GetSubnetForHardwareAddr(ctx context.Context, hwAddr net.HardwareAddr) (Subnet, error)
func GetVPCSubnetsForHost(ctx context.Context) ([]string, error)

type Subnet struct {
    ID   string
    Cidr string
}
```

`GetNetworkID` walks all MAC addresses via IMDS and returns the single VPC ID; it errors when the instance is multi-homed across VPCs.

### Spot instance support (`spot.go`)

```go
var ErrNotSpotInstance = errors.New("instance is not a spot instance")

func IsSpotInstance(ctx context.Context) (bool, error)
func GetSpotTerminationTime(ctx context.Context) (time.Time, error)
```

`GetSpotTerminationTime` first confirms the instance is a spot instance via `instance-life-cycle`, then checks the `spot/instance-action` IMDS endpoint for a scheduled termination. Returns `ErrNotSpotInstance` for on-demand instances and a 404-wrapping error when no termination is pending.

### Host CCRID (`ccrid_fetch.go`)

```go
func GetHostCCRID(ctx context.Context) (string, error)
```

Returns a full EC2 instance ARN: `arn:aws:ec2:<region>:<accountID>:instance/<instanceID>`. Used to identify the host as a Cloud Resource ID.

### DMI fallback (`dmi.go`)

On AWS Nitro instances the board asset tag contains the instance ID. `isBoardVendorEC2` checks the DMI board vendor, and `getInstanceIDFromDMI` reads the board asset tag (must start with `i-`). `isEC2UUID` checks that the product or hypervisor UUID starts with `ec2` (with little-endian byte-swap handling).

### Tags subpackage (`tags/`)

```go
// Build tag: ec2
func GetTags(ctx context.Context) ([]string, error)
func GetInstanceInfo(ctx context.Context) ([]string, error)
func GetClusterName(ctx context.Context) (string, error)
func GetContainerInstanceARN(ctx context.Context) (string, error)  // docker build tag
```

`GetTags` fetches EC2 resource tags as `key:value` strings. It first tries the IMDS tags endpoint (`/tags/instance`) when `collect_ec2_tags_use_imds` is set, then falls back to the EC2 API using IAM role credentials. Results are cached indefinitely (refreshed on failure from a stale cache).

`GetInstanceInfo` collects instance metadata as host tags: `region`, `instance-type`, `aws_account`, `image`, `availability-zone`, and `container_instance_arn` (on ECS EC2).

`GetClusterName` scans EC2 tags for a `kubernetes.io/cluster/<name>` key and extracts the cluster name from it.

#### Config keys

| Key | Description |
|---|---|
| `ec2_prefer_imdsv2` | Use IMDSv2 with IMDSv1 fallback |
| `ec2_imdsv2_transition_payload_enabled` | IMDSv2 preferred; fall back to DMI if unavailable |
| `ec2_use_dmi` | Allow DMI-based instance ID and EC2 detection |
| `ec2_metadata_timeout` | HTTP timeout in milliseconds for IMDS requests |
| `ec2_metadata_token_lifetime` | IMDSv2 token TTL in seconds |
| `collect_ec2_tags` | Enable EC2 tag collection |
| `collect_ec2_tags_use_imds` | Prefer IMDS over API for tag collection |
| `collect_ec2_instance_info` | Enable instance info tag collection |
| `exclude_ec2_tags` | List of tag keys to suppress |
| `ec2_use_windows_prefix_detection` | Include `ec2amaz-` in default hostname detection |

## Usage

`pkg/util/ec2` is imported by several core agent components:

- **`pkg/util/cloudproviders`**: calls `IsRunningOn` and `GetAccountID` during cloud provider detection at agent startup.
- **`pkg/util/hostname`**: calls `GetHostname`, `IsDefaultHostname`, `GetHostAliases`, and `IsDefaultHostnameForIntake` to resolve the agent hostname and decide whether an EC2-assigned hostname should be replaced.
- **`comp/metadata/host/hostimpl/utils`**: calls `GetInstanceType` and `GetInstanceIdentity` to populate inventory metadata payloads.
- **`pkg/util/cloudproviders/network`**: calls `GetNetworkID` to populate the `network.id` tag.
- **`pkg/network/gateway_lookup_linux.go`**: calls `GetSubnetForHardwareAddr` to look up VPC subnets for network flow enrichment.
- **`pkg/databasemonitoring/aws`**: calls `GetInstanceIdentity` and `GetRegion` when building AWS credentials for database monitoring.
- **`pkg/security/common`**: calls `GetAccountID` for CWS host context.

Typical usage pattern for reading IMDS:

```go
import "github.com/DataDog/datadog-agent/pkg/util/ec2"

if ec2.IsRunningOn(ctx) {
    id, err := ec2.GetInstanceID(ctx)
    tags, err := ec2tags.GetTags(ctx)
}
```

For testing, substitute `ec2internal.MetadataURL` (a package-level variable) with a local test server URL.

### Detection priority and fallback sequence

`IsRunningOn` tries three evidence sources in order, stopping at the first positive:

1. **IMDS reachable** — HTTP GET to `http://169.254.169.254/latest/meta-data/hostname` succeeds.
2. **DMI board vendor** — `pkg/util/dmi.GetBoardVendor()` returns `"Amazon EC2"` (Nitro instances).
3. **DMI / hypervisor UUID** — `pkg/util/dmi.GetProductUUID()` or `GetHypervisorUUID()` starts with `"ec2"` (Xen-based instances; requires little-endian byte-swap awareness).

The detected "source" (`UUID`, `DMI`, `IMDSv1`, `IMDSv2`) is tracked via `SetCloudProviderSource` and surfaced in the inventory host metadata payload.

### IMDSv1 vs. IMDSv2 migration guide

| `ec2_prefer_imdsv2` | `ec2_imdsv2_transition_payload_enabled` | Effective behaviour |
|---|---|---|
| `false` (default) | `false` | IMDSv1 only |
| `true` | `false` | IMDSv2 with IMDSv1 fallback |
| `false` | `true` | IMDSv2 preferred; DMI fallback if token unavailable |
| `true` | `true` | IMDSv2 only (no v1 fallback); DMI fallback |

Set `ec2_metadata_timeout` (milliseconds) to control per-request timeouts and `ec2_metadata_token_lifetime` (seconds, default 21600) to control how long an IMDSv2 token is cached.
