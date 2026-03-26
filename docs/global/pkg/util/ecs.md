> **TL;DR:** Typed HTTP client layer for all four versions of the ECS Task Metadata Service (TMDS), providing detection, task/container/instance metadata fetching, per-container stats, and cluster/ARN parsing for both ECS EC2 and ECS Fargate workloads.

# pkg/util/ecs

## Purpose

`pkg/util/ecs` provides a typed HTTP client layer for all four versions of the ECS Task Metadata Service (TMDS), plus higher-level helpers built on top of them. The package is used to:

- Detect whether ECS Metadata endpoints are reachable and which versions are available.
- Fetch task, container, and instance metadata for ECS EC2 and ECS Fargate workloads.
- Retrieve per-container CPU, memory, network, and I/O statistics.
- Parse ECS cluster names, container instance ARNs, and task ARNs.
- Build a `MetaECS` object (cluster name, region, account ID, cluster ID, ECS agent version) for use by the inventory/metadata pipeline.
- Expose caching helpers for resource-tag availability (`HasEC2ResourceTags`, `HasFargateResourceTags`).

## Build tags

All files in this package (except `no_ecs.go`) require the `docker` build tag. `no_ecs.go` provides empty stubs for non-Docker builds.

| File / subpackage | Build tag |
|---|---|
| `ecs.go`, `detection.go`, `diagnosis.go` | `docker` |
| `no_ecs.go` | `!docker` (stub) |
| `metadata/clients.go`, `metadata/detection.go` | `docker` |
| `metadata/clients_nodocker.go` | `!docker` (stub) |
| `metadata/v1/client.go` | `docker` |
| `metadata/v1/client_nodocker.go` | `!docker` (stub) |
| `metadata/v2/client.go` | `docker` |
| `metadata/v2/client_nodocker.go` | `!docker` (stub) |
| `metadata/v3or4/client.go` | `docker` |
| `metadata/v3or4/client_nodocker.go` | `!docker` (stub) |

## Key elements

### Key types

**`MetaECS`** struct — top-level cluster metadata used by the inventory pipeline. Fields: `AWSAccountID`, `Region`, `ECSCluster`, `ECSClusterID` (MD5-derived UUID), `ECSAgentVersion`.

**V3/V4 types** (`metadata/v3or4/types.go`) — see `### V3/V4 types` below for `Task`, `Container`, `ContainerStatsV4`, and related structs.

### Key interfaces

**`v1.Client`** — introspection endpoint operations: `GetInstance`, `GetTasks`.

**`v2.Client`** — Fargate basic metadata: `GetTask`, `GetTaskWithTags`, `GetContainerStats`.

**`v3or4.Client`** — task-scoped metadata: `GetTask`, `GetTaskWithTags`, `GetContainer`, `GetContainerStats`.

### Key functions

#### Top-level (`pkg/util/ecs`)

```go
type MetaECS struct {
    AWSAccountID    string
    Region          string
    ECSCluster      string
    ECSClusterID    string  // MD5-derived UUID of accountID/region/clusterName
    ECSAgentVersion string
}

func GetClusterMeta() (*MetaECS, error)
func ParseClusterName(value string) string        // strips ARN prefix, returns short name
func ParseRegionAndAWSAccountID(arn string) (string, string)
func HasEC2ResourceTags() bool
func HasFargateResourceTags(ctx context.Context) bool
```

`GetClusterMeta` is cached with `cache.NoExpiration`. On ECS Fargate it fetches data via `V3orV4FromCurrentTask`; on ECS EC2 it uses the v1 introspection endpoint. `ParseRegionAndAWSAccountID` accepts all valid AWS partitions (`aws`, `aws-us-gov`, `aws-cn`).

### Metadata client factory (`metadata/clients.go`)

Returns lazily-initialised, singleton clients for each API version. Each client initialises once (via `sync.Once`) and uses an exponential backoff retrier (1s initial, 5m max) to handle startup timing.

```go
func V1() (v1.Client, error)
func V2() (v2.Client, error)
func V3orV4FromCurrentTask() (v3or4.Client, error)  // prefers v4 over v3
func V4FromCurrentTask() (v3or4.Client, error)
```

All four functions return an error when `AWS` is disabled via `cloud_provider_no_detect`.

### V1 client — introspection endpoint (`metadata/v1/`)

The v1 API is the ECS agent introspection endpoint, reachable at port `51678` on the host or gateway address.

```go
type Client interface {
    GetInstance(context.Context) (*Instance, error)
    GetTasks(context.Context) ([]Task, error)
}

func NewClient(agentURL string) Client

type Instance struct {
    Cluster              string
    Version              string
    ContainerInstanceARN string
}

type Task struct {
    Arn, DesiredStatus, KnownStatus, Family, Version string
    Containers []Container
}

type Container struct {
    DockerID, DockerName, Name string
}
```

The v1 URL is auto-detected at runtime: the agent inspects the `ecs-agent` Docker container for its IP addresses, tries the default gateway, then falls back to `localhost:51678` and `169.254.172.1:51678` (awsvpc mode). The detection is controlled by `ecs_agent_url` and `ecs_agent_container_name` config options.

### V2 client — Fargate basic metadata (`metadata/v2/`)

The v2 API is available on Fargate at the static address `http://169.254.170.2`.

```go
type Client interface {
    GetTask(ctx context.Context) (*Task, error)
    GetTaskWithTags(ctx context.Context) (*Task, error)
    GetContainerStats(ctx context.Context, id string) (*ContainerStats, error)
}

func NewDefaultClient() Client
func NewClient(agentURL string) Client
```

`GetTaskWithTags` calls `/v2/metadataWithTags` which includes propagated resource tags; this is the endpoint checked by `HasFargateResourceTags`.

### V3/V4 client — task-scoped metadata (`metadata/v3or4/`)

The v3/v4 endpoint URL is injected per-task via environment variables.

```go
const DefaultMetadataURIv3EnvVariable = "ECS_CONTAINER_METADATA_URI"
const DefaultMetadataURIv4EnvVariable = "ECS_CONTAINER_METADATA_URI_V4"

type Client interface {
    GetTask(ctx context.Context) (*Task, error)
    GetTaskWithTags(ctx context.Context) (*Task, error)
    GetContainer(ctx context.Context) (*Container, error)
    GetContainerStats(ctx context.Context, id string) (*ContainerStatsV4, error)
}

func NewClient(agentURL, apiVersion string, options ...ClientOption) Client
func WithTryOption(initialInterval, maxElapsedTime time.Duration, increaseRequestTimeout func(time.Duration) time.Duration) ClientOption
```

v4 is preferred over v3 when both environment variables are set. `WithTryOption` adds exponential backoff retry with adaptive timeout increase, used in startup sequences where the endpoint may not yet be ready.

Minimum ECS agent versions for v4: `1.39.0` (Linux), `1.54.0` (Windows). The helper `metadata.IsMetadataV4Available(ecsAgentVersion string)` checks this.

### V3/V4 types (`metadata/v3or4/types.go`)

```go
type Task struct {
    ClusterName, TaskARN, Family, Version, KnownStatus, DesiredStatus string
    LaunchType              string             // v4 only
    Containers              []Container
    ContainerInstanceTags   map[string]string  // v4 only, from GetTaskWithTags
    TaskTags                map[string]string  // v4 only, from GetTaskWithTags
    Limits                  map[string]float64
    AvailabilityZone        string
    ServiceName, VPCID      string
    EphemeralStorageMetrics map[string]int64
    Errors                  []AwsError
}

type Container struct {
    Name, DockerID, DockerName, Image, ImageID string
    Type, KnownStatus, DesiredStatus           string
    Labels                                     map[string]string
    Networks                                   []Network
    Ports                                      []Port
    Volumes                                    []Volume
    Health                                     *HealthStatus
    LogDriver                                  string  // v4 only
    ContainerARN                               string  // v4 only
    RestartCount                               *int    // v4 only
    ExitCode                                   *int64
}

type ContainerStatsV4 struct {
    Timestamp string
    CPU       CPUStats
    Memory    MemStats
    IO        IOStats
    Networks  NetStatsMap
}
```

`Network.NetworkMode` is one of `awsvpc` or `bridge`.

### Telemetry (`telemetry/telemetry.go`)

All HTTP calls to TMDS endpoints are tracked via `telemetry.AddQueryToTelemetry(path, *http.Response)` which records request counts and HTTP status codes as metrics.

### Configuration and build flags

| Key | Description |
|---|---|
| `ecs_agent_url` | Override the v1 endpoint URL |
| `ecs_agent_container_name` | Docker container name of the ECS agent (default: `ecs-agent`) |
| `ecs_metadata_timeout` | HTTP timeout in milliseconds for metadata calls |

All files in this package (except `no_ecs.go`) require the `docker` build tag. `no_ecs.go` provides empty stubs for non-Docker builds.

## Relationship to other packages

| Package | Role |
|---|---|
| [`pkg/util/ec2`](ec2.md) | Provides host-level EC2 metadata (instance ID, region, VPC, IMDS access). `pkg/util/ecs` focuses on task/container-level TMDS metadata. `ec2/tags.GetContainerInstanceARN` bridges the two: it fetches the ECS container instance ARN from the v1 introspection path and is exposed via `GetInstanceInfo` in the EC2 tags sub-package. |
| [`pkg/util/fargate`](fargate.md) | `pkg/util/fargate.IsSidecar()` and `GetFargateHost()` rely on ECS task metadata retrieved via the v2 and v4 clients in this package. `GetFargateHost` calls `V2().GetTask` (ECS Fargate) or `V3orV4FromCurrentTask().GetTask` (ECS Managed Instances) to build the hostname string. |
| [`pkg/util/aws`](aws.md) | `pkg/util/aws/creds` provides IAM credential retrieval and region detection needed when the agent signs AWS API calls. `pkg/util/ecs` does not import it; both packages communicate with link-local HTTP endpoints independently. |
| [`comp/core/workloadmeta`](../../comp/core/workloadmeta.md) | The ECS built-in collector (`comp/core/workloadmeta/collectors/internal/ecs`) is the primary consumer. It calls `metadata.V1()`, `metadata.V2()`, and `metadata.V3orV4FromCurrentTask()` to construct `workloadmeta.ECSTask` and `workloadmeta.Container` entities that are then available to all agent components via the workloadmeta store. |

## Usage

`pkg/util/ecs` is the primary data source for ECS workload metadata across the agent:

- **`comp/core/workloadmeta/collectors/internal/ecs`**: the ECS workloadmeta collector uses `metadata.V1()`, `metadata.V2()`, and `metadata.V3orV4FromCurrentTask()` to build `workloadmeta.ECSTask` and `workloadmeta.Container` entities. The v2 parser handles Fargate, while v1 and v3/v4 parsers handle ECS EC2.
- **`pkg/collector/corechecks/orchestrator/ecs`**: calls `ecs.GetClusterMeta()` to attach cluster-level tags to orchestrator payloads.
- **`comp/core/workloadmeta/collectors/util/ecs_util`**: helper utilities built on top of v3/v4 client calls.
- **`pkg/flare/archive.go`**: includes ECS metadata in agent flares for debugging.
- **`pkg/collector/corechecks/containerlifecycle`**: reads ECS task ARNs for container lifecycle events.
- **`pkg/util/fargate`** (via `GetFargateHost`): calls `V2().GetTask` and `V3orV4FromCurrentTask().GetTask` to build the Fargate hostname (e.g., `fargate_task:<TaskARN>`).

Typical usage pattern:

```go
import (
    "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata"
    v3or4 "github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
)

client, err := metadata.V3orV4FromCurrentTask()
if err != nil {
    // not running on ECS or endpoint not available
}
task, err := client.GetTask(ctx)
```

### Choosing the right client

| Scenario | Client to use |
|---|---|
| ECS EC2 — host-level introspection (cluster, agent version) | `metadata.V1()` |
| ECS Fargate — basic task metadata and stats | `metadata.V2()` |
| ECS Fargate or EC2 — task-scoped metadata with tags | `metadata.V3orV4FromCurrentTask()` |
| Detect Fargate sidecar mode | `pkg/util/fargate.IsSidecar()` (wraps env feature flags, not a direct TMDS call) |

### Telemetry

All TMDS HTTP requests are automatically instrumented. Counts and HTTP status codes are reported via the `telemetry/telemetry.go` helpers and are visible in the agent's debug endpoint.

For testing, `metadata/testutil/dummy_ecs.go` provides a `DummyECS` HTTP server that serves canned v1/v2/v3/v4 responses.
