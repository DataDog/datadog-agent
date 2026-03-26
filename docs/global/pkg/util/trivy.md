# pkg/util/trivy

## Purpose

`pkg/util/trivy` integrates the [Trivy](https://github.com/aquasecurity/trivy) open-source vulnerability scanner to generate Software Bill of Materials (SBOM) reports for container images and host filesystems. It is used by the SBOM collection pipeline in the Datadog Agent to enumerate packages, language dependencies, and OS metadata, then emit the results as CycloneDX payloads forwarded to the Datadog backend.

All files are guarded by the `trivy` build tag. Container-runtime–specific files additionally require `docker`, `containerd`, or `crio`.

The subpackage `pkg/util/trivy/walker` provides the filesystem walker used during SBOM generation for host and container filesystem scans.

## Key elements

### `Collector` (`trivy.go`)

The central type. Holds scanner configuration, a lazily initialised persistent cache, and OS/language scanner instances.

```
type Collector struct { ... }
```

| Method | Description |
|---|---|
| `NewCollector(cfg, wmeta)` | Creates a `Collector` configured from the agent config (`sbom.*` keys) and an optional `workloadmeta` handle. |
| `NewCollectorForCLI()` | Creates an uncached `Collector` for the `sbomgen` CLI. |
| `GetGlobalCollector(cfg, wmeta)` | Returns (or creates) a process-level singleton collector. |
| `ScanFilesystem(ctx, path, opts, removeLayers)` | Walks a directory tree, produces a `*Report`. |
| `ScanFSTrivyReport(ctx, path, opts, removeLayers)` | Like `ScanFilesystem` but returns the raw `*types.Report` from trivy. |
| `GetCache()` | Returns the lazily-initialised `CacheWithCleaner` (BoltDB + LRU). Initialisation is done exactly once via `sync.Once`. |
| `Close()` | Flushes and closes the cache; optionally clears it based on `sbom.clear_cache_on_exit`. |

Analyzer-group constants (`OSAnalyzers`, `LanguagesAnalyzers`, `SecretAnalyzers`, etc.) identify which trivy analyzers are enabled.

`DefaultDisabledCollectors(enabledAnalyzers)` and `DefaultDisabledHandlers()` return the trivy `analyzer.Type`/`HandlerType` slices to pass as disabled when creating a trivy artifact.

### `Report` (`report.go`)

Wraps a CycloneDX BOM produced from a trivy `types.Report`.

| Method | Description |
|---|---|
| `ToCycloneDX()` | Returns the `*cyclonedx_v1_4.Bom`. |
| `ID()` | Returns the content-hash identifier (SHA-256 of scan results for filesystem scans; image ID for image scans). |

### Cache (`cache.go`)

`CacheWithCleaner` extends trivy's `cache.Cache` interface with a `clean()` method that evicts entries for images that no longer exist in `workloadmeta`.

`ScannerCache` — the production implementation backed by `persistentCache` (BoltDB + in-memory LRU with a configurable max-disk-size eviction policy). Emits `sbom.cache.*` telemetry.

`NewCustomBoltCache(wmeta, cacheDir, maxDiskSize)` — factory used by `GetCache()`.

A `memoryCache` (unexported) is used for filesystem scans where caching intermediate results provides no benefit.

### Container image integration (`docker.go`, `containerd.go`, `crio.go`, `overlayfs.go`)

`ScanDockerImage`, `ScanContainerdImage`, `ScanCRIOImage` (one per runtime) — collect the runtime-specific `ftypes.Image` reference and delegate to `scanImage`.

`scanImage` (`scan_image.go`) — the shared image scanning path: creates a trivy image artifact, looks up the persistent cache, runs the local scanner, and calls `buildReport`.

The `image` type (`image.go`) is a lazy wrapper around a container-registry image that avoids expensive `docker save` operations when layer data is already cached.

`overlayfs.go` implements a `fakeContainer` that reassembles layer paths (from overlayfs or containerd snapshots) into a synthetic image trivy can analyse without invoking the container daemon.

### `walker/FSWalker` (`walker/walker.go`)

`FSWalker` implements trivy's `walker.Walker` interface for the local filesystem.

```
type FSWalker struct{}
func NewFSWalker() *FSWalker
func (w *FSWalker) Walk(ctx, rootPath string, opt walker.Option, fn walker.WalkFunc) error
```

`Walk` uses `os.OpenRoot` (Go 1.23+) to confine traversal to the given root, applies skip/only-dir filters (appending hardcoded defaults for `.git`, `proc`, `sys`, `dev`), and calls `fn` for every non-filtered regular file. Permission errors and missing files are silently skipped; all other errors halt the walk or are forwarded to `opt.ErrorCallback`.

### Database (`db.go`, `sqlite.go`)

Helpers for initialising the trivy vulnerability database used by `vulnClient`. Under normal agent operation the database is not downloaded at runtime (`OfflineJar: true`); it is expected to be shipped or pre-populated separately.

## Usage

`pkg/util/trivy` is consumed exclusively by the SBOM collection component:

- `comp/core/workloadmeta` triggers image scans when a container image is set-or-expired.
- `pkg/sbom/collectors/` (docker, containerd, crio, host filesystem collectors) call `Collector.ScanDockerImage` / `ScanContainerdImage` / `ScanCRIOImage` / `ScanFilesystem` to obtain a `*Report`.
- The report's `ToCycloneDX()` output is serialised and forwarded to the Datadog intake.

### Data flow

```
workloadmeta (KindContainerImageMetadata EventTypeSet)
        │
        │  scanner.Scan(request)
        ▼
pkg/sbom/scanner.Scanner  ──  rate-limited work queue
        │
        │  collector.Scan(ctx, ScanRequest)
        ▼
pkg/sbom/collectors/<runtime>.Collector
        │  ScanDockerImage / ScanContainerdImage / ScanCRIOImage / ScanFilesystem
        ▼
pkg/util/trivy.Collector  ──  lazily-initialised BoltDB+LRU cache
        │
        │  Report.ToCycloneDX()
        ▼
pkg/sbom.ScanResult.ConvertScanResultToSBOM()
        │
        ▼
workloadmeta.SBOM  ──  stored back via store.Push
        │
        ▼
pkg/collector/corechecks/sbom  ──  event-platform forwarder  ──  Datadog intake
```

### Containerd integration

When `sbom.container_image.use_mount` is enabled, the containerd collector calls
`pkg/util/containerd.ContainerdItf.MountImage` / `Mounts` (from `pkg/util/trivy/containerd.go`)
to create a read-only overlay snapshot before scanning, avoiding a full image extraction.
`pkg/util/trivy.ScanContainerdImage` then receives the mount path and proceeds with the
filesystem scanner path rather than pulling layer archives.

### Cache lifecycle

`GetCache()` initialises the singleton `CacheWithCleaner` once (`sync.Once`). The cleaner
goroutine runs on the interval set by `sbom.cache.clean_interval` and calls
`workloadmeta.ListImages()` to discover which images still exist, evicting cache entries for
images that have been removed. This prevents unbounded growth of the BoltDB cache on hosts
with high image churn.

Configuration keys that affect this package:

| Key | Effect |
|---|---|
| `sbom.cache_directory` | Directory for the BoltDB persistent cache |
| `sbom.clear_cache_on_exit` | Purge cache when collector is closed |
| `sbom.cache.max_disk_size` | Maximum on-disk cache size (bytes) |
| `sbom.cache.clean_interval` | How often the cache cleaner evicts stale entries |
| `sbom.compute_dependencies` | Include dependency edges in the BOM |
| `sbom.simplify_bom_refs` | Shorten BOM component references |

## Related packages

| Package | Doc | Relationship |
|---------|-----|--------------|
| `pkg/sbom` | [../sbom.md](../sbom.md) | Orchestration layer. The `pkg/sbom/scanner.Scanner` enqueues scan requests from workloadmeta events and dispatches them to each `pkg/sbom/collectors/<runtime>` backend, which delegates the actual scanning to `pkg/util/trivy.Collector`. `pkg/sbom.ScanResult.ConvertScanResultToSBOM()` wraps the `Report.ToCycloneDX()` output and stores it in workloadmeta. `sbom.bomconvert` then converts the CycloneDX types to the agent-payload protobuf for transmission. |
| `pkg/util/containerd` | [containerd.md](containerd.md) | Provides `ContainerdItf.MountImage` and `Mounts`, called by `pkg/util/trivy/containerd.go` when `sbom.container_image.use_mount` is set. Also provides `ListImages` used by the containerd SBOM collector to enumerate images before calling `ScanContainerdImage`. Requires the `containerd` build tag. |
| `comp/core/workloadmeta` | [../../comp/core/workloadmeta.md](../../comp/core/workloadmeta.md) | Two-way coupling: workloadmeta fires `KindContainerImageMetadata` `EventTypeSet` events that trigger scans, and receives `workloadmeta.SBOM` results back via `store.Push`. The `NewCollector(cfg, wmeta)` constructor stores the `wmeta` handle so the cache cleaner can call `wmeta.ListImages()` to prune evicted entries. |
