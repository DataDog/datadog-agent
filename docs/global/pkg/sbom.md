# pkg/sbom — Software Bill of Materials scanning

## Purpose

`pkg/sbom` provides the infrastructure for generating and handling Software Bill of Materials (SBOMs) in CycloneDX format. It drives vulnerability and compliance workflows in the agent by inventorying every package installed on a container image or a host filesystem. The output is serialized as a `cyclonedx_v1_4.Bom` protobuf and stored in `workloadmeta`.

The package is conditionally compiled: most collector and conversion code requires the `trivy` build tag (Linux/macOS) or `windows && wmi` (Windows). Tests and integration wiring also need those tags.

---

## Key elements

### Root package (`pkg/sbom`)

| Symbol | Description |
|--------|-------------|
| `Report` interface | Single method `ToCycloneDX() *cyclonedx_v1_4.Bom` plus `ID() string`. All scanner back-ends return a `Report`. |
| `ScanRequest` | Type alias for `types.ScanRequest`. Implemented by each collector to describe what should be scanned. |
| `ScanOptions` | Type alias for `types.ScanOptions`. Controls analyzers, disk-usage guard, mount mode, OverlayFS, timeouts, and wait intervals. |
| `ScanResult` | Holds `Report`, `Error`, timestamps, `GenerationMethod`, and a pointer to `workloadmeta.ContainerImageMetadata`. |
| `ScanResult.ConvertScanResultToSBOM()` | Converts a `ScanResult` to a `*workloadmeta.SBOM`, translating errors to `workloadmeta.Failed`. |
| `ScanOptionsFromConfigForContainers(cfg)` | Reads `sbom.container_image.*` config keys into a `ScanOptions`. |
| `ScanOptionsFromConfigForHosts(cfg)` | Reads `sbom.host.*` config keys into a `ScanOptions`. |
| Constants `ScanFilesystemType`, `ScanDaemonType` | Tag values for the `scan_method` tag on metrics. |

### `types/`

Defines the foundational interfaces and structs used by all sub-packages:

- `ScanRequest` interface: `Collector() string`, `Type(ScanOptions) string`, `ID() string`.
- `ScanOptions` struct: fields for `Analyzers`, `CheckDiskUsage`, `MinAvailableDisk`, `Timeout`, `WaitAfter`, `Fast`, `UseMount`, `OverlayFsScan`, `AdditionalDirs`.

### `collectors/`

Defines the `Collector` interface and the global registry:

| Symbol | Description |
|--------|-------------|
| `Collector` interface | `Type()`, `CleanCache()`, `Init()`, `Scan(ctx, ScanRequest) ScanResult`, `Channel() chan ScanResult`, `Options() ScanOptions`, `Shutdown()`. |
| `ScanType` | String enum: `ContainerImageScanType` or `HostScanType`. |
| `Collectors` var | Global `map[string]Collector`. Collectors self-register via `RegisterCollector`. |
| `RegisterCollector(name, collector)` | Called from each collector's `init()` to add itself to the registry. |
| Accessor functions | `GetDockerScanner()`, `GetContainerdScanner()`, `GetCrioScanner()`, `GetHostScanner()`, `GetProcfsScanner()`. |

Built-in collector back-ends (each in its own sub-directory):

| Back-end | Directory | Notes |
|----------|-----------|-------|
| `containerd` | `collectors/containerd/` | Scans images via the containerd API. Requires `trivy`. |
| `crio` | `collectors/crio/` | Scans images via CRI-O. Requires `trivy`. |
| `docker` | `collectors/docker/` | Scans images via the Docker daemon. Requires `trivy`. |
| `host` | `collectors/host/` | Scans the host root filesystem (`/` or `HOST_ROOT`). Requires `trivy` or `windows && wmi`. |
| `procfs` | `collectors/procfs/` | Fargate/procfs-based scan path. |

Each collector's `collectors/<name>/request.go` (or similar) implements `types.ScanRequest`.

### `scanner/`

Orchestrates scan execution. The `Scanner` struct wraps a rate-limiting work queue (exponential backoff) around the collector registry.

| Symbol | Description |
|--------|-------------|
| `Scanner` struct | Holds the scan queue, collectors map, and a `filesystem.Disk` for disk-space checks. |
| `NewScanner(cfg, collectors, wmeta)` | Creates a `Scanner` with configured backoff limits (`sbom.scan_queue.*`). |
| `CreateGlobalScanner(cfg, wmeta)` | Initializes all registered collectors, creates the global `Scanner` singleton. Returns an error if called twice. |
| `GetGlobalScanner()` | Returns the current singleton; returns `nil` if not yet created. |
| `SetGlobalScanner(s)` | Test helper to inject a mock scanner. |
| `Scanner.Start(ctx)` | Starts the scan request handler goroutine and the cache cleaner (interval from `sbom.cache.clean_interval`). Idempotent via `sync.Once`. |
| `Scanner.Scan(request)` | Enqueues a `ScanRequest` into the work queue. |
| `Scanner.PerformScan(ctx, request, collector)` | Synchronously executes a scan and records timing. |

The scanner enforces a disk-space guard before every scan: it reads `sbom.container_image.check_disk_usage` / `sbom.container_image.min_available_disk` and also requires 1.2× the image size to be free (unless `UseMount` or `OverlayFsScan` is set).

### `bomconvert/` (build tag: `trivy || (windows && wmi)`)

Converts between the `github.com/CycloneDX/cyclonedx-go` types (used by trivy) and the internal `agent-payload/v5/cyclonedx_v1_4` protobuf types used for transmission.

| Symbol | Description |
|--------|-------------|
| `ConvertBOM(in *cyclonedx.BOM, simplifyBomRefs bool) *cyclonedx_v1_4.Bom` | Top-level conversion function. When `simplifyBomRefs` is true, replaces verbose BOM reference strings with sequential integers to reduce payload size. |
| `ConvertDuration(d time.Duration) *durationpb.Duration` | Utility to convert Go durations to protobuf `Duration`. |

All component types, hashes, licenses, severities, compositions, and vulnerabilities are mapped 1-to-1 through type-switch converters.

---

## Usage

### Startup (agent process)

The SBOM scanner is created once during agent startup:

```go
// cmd/agent/subcommands/run/command.go (simplified)
scanner, err := scanner.CreateGlobalScanner(cfg, wmeta)
if err != nil {
    return err
}
scanner.Start(ctx)
```

`CreateGlobalScanner` calls `Init()` on all registered collectors. The `wmeta` handle is stored on the scanner so that `pkg/util/trivy.CacheWithCleaner.clean()` can evict cache entries for images that no longer exist in the workloadmeta store.

Collectors self-register via package-level `init()` functions; importing the collector sub-package side-effect-registers it:

```go
import _ "github.com/DataDog/datadog-agent/pkg/sbom/collectors/containerd"
```

The `containerd` build tag must be set for the containerd collector to be compiled. Similarly, the `docker` and `crio` build tags gate their respective collectors.

### Submitting a scan

```go
request := containerd.NewScanRequest(imageID, imgMeta)
if err := scanner.GetGlobalScanner().Scan(request); err != nil {
    log.Errorf("failed to enqueue SBOM scan: %v", err)
}
```

Results are delivered asynchronously on `collector.Channel()`.

### Consuming results (workloadmeta integration)

`comp/core/workloadmeta/collectors/internal/containerd/image_sbom_trivy.go` (and Docker/CRI-O equivalents) read from `collector.Channel()` and call `result.ConvertScanResultToSBOM()` to update the workloadmeta store. The resulting `*workloadmeta.SBOM` is pushed back via `store.Push(SourceRuntime, EventTypeSet{entity})`, completing the round-trip: trigger flows *from* workloadmeta, result flows *back into* workloadmeta.

**Scan trigger sequence:**
1. The containerd workloadmeta collector detects a new `KindContainerImageMetadata` event (via `Subscribe` with a `ContainerImageMetadata` filter).
2. It builds a `containerd.ScanRequest` and calls `scanner.GetGlobalScanner().Scan(request)`.
3. The scanner enqueues the request; when executed, it delegates to `pkg/util/trivy.Collector.ScanContainerdImage`.
4. The `*trivy.Report` is returned on `collector.Channel()`, converted to `*workloadmeta.SBOM`, and pushed back into the store.

For Docker and CRI-O, the same pattern applies with `ScanDockerImage` and `ScanCRIOImage` respectively.

### SBOM check

`pkg/collector/corechecks/sbom/` (`CheckName = "sbom"`, build tag `trivy || (windows && wmi)`) reads from the collector channels, batches results, and forwards them to the event-platform forwarder. Configuration keys include `chunk_size`, `periodic_refresh_seconds`, and `host_heartbeat_validity_seconds`.

The check is enabled by `sbom.enabled: true` in `datadog.yaml`. Container image scanning is further gated by `sbom.container_image.enabled` and host scanning by `sbom.host.enabled`. The check emits `datadog.agent.sbom.container_images.running` and `datadog.agent.sbom.hosts.running` counters on each run.

Payloads are sent via `sender.EventPlatformEvent(rawBytes, eventplatform.EventTypeContainerSBOM)`, which routes through `pkg/aggregator.BufferedAggregator` to `comp/forwarder/eventplatform`. See [`eventplatform.md`](../comp/forwarder/eventplatform.md) for the full path from the check to the `sbom-intake.` endpoint.

The host scanner additionally supports a **spread refresher** (`sbom.container_image.use_spread_refresher: true`) that distributes periodic re-scans over the full interval to avoid thundering-herd behaviour when many images are present.

### End-to-end data flow

```
comp/core/workloadmeta (containerd/docker/crio collector)
  │  Subscribe(KindContainerImageMetadata) → new image event
  ▼
pkg/sbom/scanner.Scanner.Scan(ScanRequest)
  │  work queue with exponential backoff
  ▼
pkg/sbom/collectors/<runtime>.Scan(ctx, request)
  │  delegates to pkg/util/trivy.Collector
  │     ├── ScanContainerdImage / ScanDockerImage / ScanCRIOImage
  │     │       └── pkg/util/containerd.ContainerdItf (MountImage / Mounts)
  │     └── BoltDB persistent cache (sbom.cache_directory)
  ▼
pkg/util/trivy.Report.ToCycloneDX()  →  *cyclonedx_v1_4.Bom
  ▼
pkg/sbom.ScanResult.ConvertScanResultToSBOM()  →  *workloadmeta.SBOM
  ▼
comp/core/workloadmeta store.Push(SourceRuntime, EventTypeSet)
  │  (SBOM stored back in workloadmeta)
  ▼
pkg/collector/corechecks/sbom check reads collector.Channel()
  │  sender.EventPlatformEvent(bytes, EventTypeContainerSBOM)
  ▼
pkg/aggregator.BufferedAggregator
  ▼
comp/forwarder/eventplatform  →  sbom-intake. (Datadog backend)
```

---

## Configuration keys

| Key | Scope |
|-----|-------|
| `sbom.container_image.analyzers` | Which analyzers to enable (e.g. `["os"]`) |
| `sbom.container_image.check_disk_usage` | Enable disk-space guard |
| `sbom.container_image.min_available_disk` | Minimum free bytes required |
| `sbom.container_image.scan_timeout` | Per-scan timeout in seconds |
| `sbom.container_image.scan_interval` | Wait after each scan (throttle) |
| `sbom.container_image.use_mount` | Use bind-mount strategy instead of extraction |
| `sbom.container_image.overlayfs_direct_scan` | Scan OverlayFS layers directly |
| `sbom.container_image.additional_directories` | Extra directories to include |
| `sbom.host.analyzers` | Analyzers for host scans |
| `sbom.host.additional_directories` | Extra directories for host scans |
| `sbom.scan_queue.base_backoff` | Exponential backoff base duration |
| `sbom.scan_queue.max_backoff` | Exponential backoff max duration |
| `sbom.cache.clean_interval` | How often to clean the collector cache |

---

## Related packages

- [`pkg/util/trivy`](util/trivy.md) — the Trivy-backed scanning engine consumed exclusively
  by the SBOM collectors. Each collector back-end (`containerd`, `docker`, `crio`, `host`)
  delegates to `pkg/util/trivy.Collector` methods (`ScanContainerdImage`,
  `ScanDockerImage`, `ScanCRIOImage`, `ScanFilesystem`). `pkg/util/trivy.Report.ToCycloneDX()`
  produces the `*cyclonedx_v1_4.Bom` that `ScanResult.ConvertScanResultToSBOM()` wraps.
  Cache configuration keys (`sbom.cache_directory`, `sbom.cache.max_disk_size`,
  `sbom.clear_cache_on_exit`) are interpreted by `pkg/util/trivy`, not by `pkg/sbom` itself.
  The `CacheWithCleaner.clean()` method evicts entries for images no longer present in
  workloadmeta, using the `wmeta` handle passed to `NewCollector(cfg, wmeta)`. The Trivy
  vulnerability database is not downloaded at runtime by default (`OfflineJar: true`); it must
  be pre-populated or shipped separately.
- [`pkg/util/containerd`](util/containerd.md) — the containerd SBOM collector
  (`pkg/sbom/collectors/containerd`) creates a `ContainerdItf` client and passes it to
  `pkg/util/trivy` for image access. `MountImage` / `Mounts` are called when
  `sbom.container_image.use_mount` is enabled to create a read-only overlay snapshot of the
  image layers without invoking `docker save`. The `containerd` build tag is required for
  this collector. For unit tests the `fake.MockedContainerdClient` from `pkg/util/containerd/fake`
  can substitute the real client.
- [`comp/core/workloadmeta`](../comp/core/workloadmeta.md) — SBOM results flow through
  workloadmeta in two directions:
  1. **Inbound trigger**: The containerd workloadmeta collector subscribes to
     `KindContainerImageMetadata` events (via `Subscribe` with `NewFilterBuilder().AddKind(KindContainerImageMetadata)`)
     and enqueues scan requests via `scanner.Scan(request)` when a new image appears.
  2. **Outbound result**: After scanning, `result.ConvertScanResultToSBOM()` produces a
     `*workloadmeta.SBOM` value that is pushed back into the store via `store.Push`. The
     `Scanner.NewScanner(cfg, collectors, wmeta)` constructor receives the `wmeta` handle
     specifically to enable this round-trip. The `SourceRuntime` source tag is used for
     runtime-discovered SBOM results.
- [`comp/forwarder/eventplatform`](../comp/forwarder/eventplatform.md) — the SBOM check
  (`pkg/collector/corechecks/sbom`) forwards serialized `cyclonedx_v1_4.Bom` payloads to the
  Datadog intake using `EventTypeContainerSBOM` (`"sbom-intake."` pipeline). Payloads are
  sent via `sender.EventPlatformEvent`, which routes through
  `pkg/aggregator.BufferedAggregator` to the event-platform forwarder. The pipeline uses the
  `StreamStrategy` (one message per send) because SBOM payloads are protobuf, not JSON-batch.
  See the data-flow diagram in `eventplatform.md` for the full path from check to intake.
