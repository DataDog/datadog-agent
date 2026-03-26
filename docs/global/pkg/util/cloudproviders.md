> **TL;DR:** Single entry point for detecting the cloud or platform environment (EC2, GCP, Azure, Alibaba, Tencent, Oracle, IBM, CloudFoundry, Kubernetes) and collecting environment-specific metadata such as hostname aliases, NTP servers, public IPs, account IDs, instance types, and Canonical Cloud Resource IDs.

# pkg/util/cloudproviders

## Purpose

`pkg/util/cloudproviders` is the single entry point for detecting the cloud or
platform environment the agent runs on and for collecting environment-specific
metadata: hostname aliases, NTP servers, public IPs, account IDs, instance
types, and Canonical Cloud Resource IDs (CCRIDs).

Rather than scattering provider-specific HTTP calls throughout the codebase,
all callers import this package and call its top-level functions. The package
delegates to provider sub-packages and handles concurrency, caching, and
fallback logic centrally.

### Relationship to other packages

| Package / component | Role |
|---|---|
| [`pkg/util/ec2`](ec2.md) | Implements the `"AWS"` entry in all detector slices. `cloudproviders` calls `ec2.IsRunningOn`, `ec2.GetAccountID`, `ec2.GetHostAliases`, `ec2.GetHostCCRID`, `ec2.GetInstanceType`, `ec2.GetPublicIPv4`, and `ec2.GetSpotTerminationTime` as the EC2 delegate. |
| [`pkg/util/ecs`](ecs.md) | ECS detection is performed by `ec2.IsRunningOn` (which checks `ECSFargate`/`ECSEC2` env features). The `cloudproviders/network` sub-package calls `ec2.GetNetworkID` to populate the `network.id` tag. |
| [`pkg/util/aws`](aws.md) | Provides the `creds` layer (IAM credential retrieval, `IsRunningOnAWS`) used by components that need to sign AWS API calls. `cloudproviders` itself does not import `pkg/util/aws`; the two packages are parallel AWS helpers. |
| [`pkg/util/hostname`](hostname.md) | Uses `cloudproviders` sub-packages directly: `gce` and `azure` are registered as hostname providers (positions 4–5 in the chain). `cloudproviders.GetHostAliases` is also called by host-metadata collection (`comp/metadata/host`) to populate `host_aliases`. |

Supported environments and their sub-packages:

| Provider | Sub-package | `CloudProviderName` |
|---|---|---|
| Amazon EC2 | `pkg/util/ec2` (separate module) | `"AWS"` |
| Google GCE / GKE | `pkg/util/cloudproviders/gce` | `"GCP"` |
| Microsoft Azure | `pkg/util/cloudproviders/azure` | `"Azure"` |
| Alibaba Cloud | `pkg/util/cloudproviders/alibaba` | `"Alibaba"` |
| Tencent Cloud | `pkg/util/cloudproviders/tencent` | `"Tencent"` |
| Oracle Cloud | `pkg/util/cloudproviders/oracle` | `"Oracle"` |
| IBM Cloud | `pkg/util/cloudproviders/ibm` | `"IBM"` |
| Cloud Foundry | `pkg/util/cloudproviders/cloudfoundry` | `"CloudFoundry"` |
| Kubernetes | `pkg/util/cloudproviders/kubernetes` | `"kubernetes"` |

## Key Elements

### Key types

**`OrchestratorName`** — see `fargate` sub-package. At the root level, providers are identified by their `CloudProviderName` string constant (e.g. `"AWS"`, `"GCP"`, `"Azure"`).

**Sentinel errors**

```go
var ErrNotPreemptible      = errors.New("instance is not preemptible")
var ErrPreemptionUnsupported = errors.New("preemption detection not supported for this cloud provider")
```

### Key functions

#### Top-level detection functions (`cloudproviders.go`)

```go
func DetectCloudProvider(ctx context.Context, collectAccountID bool) (providerName, accountID string)
func GetCloudProviderNTPHosts(ctx context.Context) []string
func GetHostAliases(ctx context.Context) (aliases []string, cloudProvider string)
func GetHostCCRID(ctx context.Context, detectedCloud string) string
func GetInstanceType(ctx context.Context, detectedCloud string) string
func GetPublicIPv4(ctx context.Context) (string, error)
func GetSource(cloudProviderName string) string
func GetHostID(ctx context.Context, cloudProviderName string) string
func GetPreemptionTerminationTime(ctx context.Context, cloudProviderName string) (time.Time, error)
```

**`DetectCloudProvider`** iterates a fixed ordered list of detectors (EC2,
GCP, Azure, Alibaba, Tencent, Oracle, IBM) and returns the name of the first
one that responds. If `collectAccountID` is true it also fetches the
provider-specific account ID (subscription ID for Azure, project ID for GCP,
AWS account ID for EC2).

**`GetHostAliases`** calls all alias detectors **concurrently** via a
`sync.WaitGroup` (provider endpoints can be slow) and merges the results. It
also disambiguates the inferred cloud provider and logs a warning if more than
one provider claims an alias.

**`GetHostCCRID`** returns a fully-qualified Canonical Cloud Resource ID. It
tries the previously detected cloud first; if the detected cloud is empty or
maps to an unknown provider (e.g. kubelet was detected), it fans out
concurrently to all CCRID-capable providers. Supported: EC2, GCP, Azure,
Oracle.

**`GetInstanceType`** returns the VM/instance size string for the detected
cloud (e.g. `"m5.xlarge"`, `"n1-standard-4"`, `"Standard_D2s_v3"`). Supported:
EC2, GCP, Azure, Oracle.

**`GetPreemptionTerminationTime`** polls for a scheduled interruption time on
preemptible instances. Currently only EC2 Spot is supported. Returns
`ErrNotPreemptible` when the instance is not a spot/preemptible instance and
`ErrPreemptionUnsupported` for providers without an implementation.

### Test mock (`mock.go`, build tag `test`)

```go
func Mock(t *testing.T, cloudProviderName, accountID, source, hostID string)
```

Replaces the internal detector slices/maps with stubs for the duration of the
test and restores them via `t.Cleanup`. Use this in unit tests that exercise
code depending on cloud provider detection without hitting any HTTP endpoints.

### Provider sub-packages

Each sub-package exposes a consistent set of functions. Not all providers
implement all functions; the root package only calls what is available.

#### Common pattern across providers

```go
const CloudProviderName = "<Name>"

func IsRunningOn(ctx context.Context) bool
func GetHostAliases(ctx context.Context) ([]string, error)
func GetNTPHosts(ctx context.Context) []string
func GetHostCCRID(ctx context.Context) (string, error)    // where supported
func GetInstanceType(ctx context.Context) (string, error) // where supported
func GetPublicIPv4(ctx context.Context) (string, error)   // where supported
```

Each provider fetches its data from the local instance metadata endpoint (IMDS)
using `pkg/util/cachedfetch.Fetcher`, which caches successful responses for the
process lifetime and prevents duplicate concurrent requests.

Providers check `configutils.IsCloudProviderEnabled(CloudProviderName, ...)` at
the start of every fetch and return an error immediately if the user has
disabled that provider via `cloud_provider_metadata` config.

#### GCP (`gce/`)

Hits `http://169.254.169.254/computeMetadata/v1` with the
`Metadata-Flavor: Google` header. `GetTags` returns instance-level network
tags plus structured attributes (`zone`, `instance-type`, `internal-hostname`,
`instance-id`, `project`, `numeric_project_id`) as `key:value` tag strings.
Instance attributes are forwarded as tags unless they appear in
`exclude_gce_tags`.

Config knobs: `gce_metadata_timeout` (ms), `gce_send_project_id_tag`,
`exclude_gce_tags`.

#### Azure (`azure/`)

Hits `http://169.254.169.254/metadata/instance/compute` with the
`Metadata: true` header. `GetHostname` supports four styles controlled by
`azure_hostname_style`: `vmid`, `name`, `name_and_resource_group`, `full`.
The API version used is controlled by `azure_metadata_api_version`.

Config knobs: `azure_hostname_style`, `azure_metadata_api_version`,
`azure_metadata_timeout`.

#### CloudFoundry (`cloudfoundry/`)

Unlike IaaS providers, this sub-package does not contact an IMDS endpoint. It
communicates with the **BBS** (bulletin board system) and **CC** (Cloud
Controller) APIs using credentials from the agent config to discover
application containers and their metadata. It is the most complex sub-package.

#### Other providers (Alibaba, Tencent, Oracle, IBM)

Follow the same IMDS pattern as GCE/Azure, each with provider-specific
endpoint URLs and header requirements.

### Configuration and build flags

| Config key | Effect |
|---|---|
| `cloud_provider_metadata` | Explicit allowlist of provider names to query; omit a name to skip that provider's IMDS endpoint |
| `gce_metadata_timeout` | HTTP timeout (ms) for GCP IMDS requests |
| `gce_send_project_id_tag` | Include the GCP project ID as a tag |
| `exclude_gce_tags` | GCP instance attributes to suppress from host tags |
| `azure_hostname_style` | Azure hostname format: `vmid`, `name`, `name_and_resource_group`, or `full` |
| `azure_metadata_api_version` | Azure IMDS API version string |
| `azure_metadata_timeout` | HTTP timeout (ms) for Azure IMDS requests |

Build tag `test` activates `Mock(t, ...)` for replacing detector tables in unit tests.

## Usage

The package is imported wherever the agent needs to know about its environment:

- **`comp/metadata/host/hostimpl/utils/host.go`** — calls `GetHostAliases`,
  `GetHostCCRID`, `GetInstanceType`, and `GetPublicIPv4` while building the
  host metadata payload sent to the Datadog backend on startup.
- **`comp/metadata/inventoryhost`** — calls `DetectCloudProvider` to populate
  the `cloud_provider` and `cloud_provider_account_id` inventory fields.
- **`pkg/collector/corechecks/cloud/hostinfo`** — dedicated check that emits
  host info metrics; calls `DetectCloudProvider` for tagging.
- **`pkg/collector/corechecks/net/ntp/ntp.go`** — calls
  `GetCloudProviderNTPHosts` to override the NTP server list with a
  provider-native NTP endpoint.
- **`comp/metadata/host/hostimpl/utils/meta.go`** — calls `GetHostAliases` to
  populate the `host_aliases` field in the metadata payload.

### Typical detection call

```go
provider, accountID := cloudproviders.DetectCloudProvider(ctx, true)
// provider: "AWS", "GCP", "Azure", ... or "" if unknown
// accountID: AWS account ID, GCP project ID, Azure subscription ID, or ""

instanceType := cloudproviders.GetInstanceType(ctx, provider)
ccrid := cloudproviders.GetHostCCRID(ctx, provider)
```

### Disabling a specific provider

Set `cloud_provider_metadata` in `datadog.yaml` to an explicit list:

```yaml
cloud_provider_metadata:
  - "AWS"
  - "GCP"
```

Omitting a provider name prevents the agent from contacting that provider's
IMDS endpoint, which is useful when running in hybrid environments where
metadata endpoints from multiple clouds might be reachable.

### Caching and concurrency notes

Each provider sub-package wraps its HTTP calls in `pkg/util/cachedfetch.Fetcher`, which caches successful responses for the process lifetime and serialises concurrent requests. This means `DetectCloudProvider` and `GetHostAliases` are safe to call from multiple goroutines simultaneously, but a permanently unreachable IMDS endpoint will not be retried after the first failure (use `cloud_provider_metadata` to skip unreachable providers rather than relying on retries).

`GetHostAliases` fans out to all alias-capable providers concurrently via a `sync.WaitGroup`, so the total latency is bounded by the slowest provider rather than the sum of all provider latencies.

### Testing

Use `cloudproviders.Mock(t, cloudProviderName, accountID, source, hostID)` (build tag `test`) to replace the internal detector tables with stubs for the duration of a test. This prevents any real HTTP calls to IMDS endpoints and makes the test hermetic.
