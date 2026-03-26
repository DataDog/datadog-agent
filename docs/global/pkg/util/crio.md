# pkg/util/crio

## Purpose

`pkg/util/crio` provides a gRPC client for the CRI-O container runtime. It implements the Kubernetes Container Runtime Interface (CRI) API v1 to query running containers, pod sandboxes, and container images directly from the CRI-O daemon over a Unix domain socket. The package also resolves overlay filesystem layer paths for container images, which is used by SBOM (Software Bill of Materials) scanning.

The package is only compiled when the `crio` build tag is set.

## Key elements

### Build tag

```
//go:build crio
```

All files in the package are gated behind this tag.

### `Client` interface

The central abstraction. Callers work exclusively through this interface:

| Method | Description |
|--------|-------------|
| `RuntimeMetadata(ctx)` | Returns CRI-O runtime version information |
| `GetAllContainers(ctx)` | Lists all containers managed by CRI-O |
| `GetContainerStatus(ctx, containerID)` | Returns state, creation time, and exit codes for one container |
| `GetContainerImage(ctx, imageSpec, verbose)` | Fetches metadata for a specific image |
| `GetPodStatus(ctx, podSandboxID)` | Returns the status of a pod sandbox |
| `GetCRIOImageLayers(imgMeta)` | Returns ordered `diff` directory paths for each layer of an image |
| `ListImages(ctx)` | Lists all images available in the CRI-O runtime |

### `NewCRIOClient() (Client, error)`

Constructor for the concrete `clientImpl`. It reads the socket path from the `cri_socket_path` config key (defaults to `/var/run/crio/crio.sock`), establishes a gRPC connection using Unix domain socket transport, and validates the connection with an initial `RuntimeMetadata` call. Connection establishment is wrapped in a retrier with exponential backoff (1 s initial, 5 min max).

### `clientImpl`

Internal struct holding:
- `runtimeClient` — `v1.RuntimeServiceClient` (container/pod operations)
- `imageClient` — `v1.ImageServiceClient` (image operations)
- `conn` — underlying `*grpc.ClientConn`
- `initRetry` — `retry.Retrier` for connection establishment

### Overlay path helpers

Three exported functions that account for whether the agent runs inside a container (applying host path prefix via `containersimage.SanitizeHostPath`):

- `GetOverlayPath()` — `/var/lib/containers/storage/overlay`
- `GetOverlayImagePath()` — `/var/lib/containers/storage/overlay-images`
- `GetOverlayLayersPath()` — `/var/lib/containers/storage/overlay-layers/layers.json`

### `GetCRIOImageLayers` internals

Reads `layers.json` from the overlay storage to build a digest-to-ID map, then resolves each layer digest in `imgMeta.Layers` to its `<overlayPath>/<layerID>/diff` directory. Layers are returned in bottom-to-top order (innermost first, matching overlay mount semantics).

### Configuration keys

| Key | Default | Effect |
|-----|---------|--------|
| `cri_socket_path` | `/var/run/crio/crio.sock` | Unix socket path for CRI-O |

## Usage

The package is consumed by two parts of the agent:

**Workloadmeta CRI-O collector** (`comp/core/workloadmeta/collectors/internal/crio/`): calls `NewCRIOClient()` at startup and uses `GetAllContainers`, `GetContainerStatus`, `GetPodStatus`, `ListImages`, and `GetContainerImage` on every collection cycle to populate the workloadmeta store with container and image entities.

**SBOM CRI-O collector** (`pkg/sbom/collectors/crio/` and `pkg/util/trivy/crio.go`): calls `GetCRIOImageLayers` to locate the overlay diff directories for a given image so that Trivy can scan the image filesystem without pulling the image.

Typical initialization pattern:

```go
client, err := crio.NewCRIOClient()
if err != nil {
    return fmt.Errorf("failed to create CRI-O client: %w", err)
}
containers, err := client.GetAllContainers(ctx)
```
