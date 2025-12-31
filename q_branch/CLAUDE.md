# q_branch Development Rules

## Fine-Grained Monitor Development

Use `./dev.py` for all fine-grained-monitor (fgm-*) development workflows:

```bash
cd q_branch/fine-grained-monitor

# Local development
./dev.py local build              # Build all release binaries
./dev.py local test               # Run tests
./dev.py local clippy             # Run clippy lints
./dev.py local viewer start       # Start fgm-viewer with default data
./dev.py local viewer start --data /path/to/file.parquet
./dev.py local viewer stop        # Stop fgm-viewer
./dev.py local viewer status      # Check fgm-viewer status

# Cluster deployment (Kind via Lima)
./dev.py cluster deploy           # Build image, load to Kind, restart pods
./dev.py cluster status           # Show cluster pod status
./dev.py cluster forward          # Port-forward to cluster pod
./dev.py cluster forward-stop     # Stop port-forward
```

**Prefer dev.py over raw commands** - it handles image loading into Lima VM, Kind cluster operations, and port management automatically.

## Architecture: aarch64 (ARM64)

All local development and testing runs on Apple Silicon (aarch64/ARM64).

**Do NOT specify `--platform linux/amd64`** in docker build commands during local testing loops. The Lima VM, Kind cluster, and all containers run natively on ARM64.

## Gadget-Dev Kubernetes Cluster

A Kind cluster runs inside a Lima VM (`gadget-k8s-host`) with the API port-forwarded to the host.

### Prefer MCP Tools Over kubectl

**Use kubernetes-mcp-server tools** for all cluster interactions:
- `pods_list`, `pods_list_in_namespace` - List pods
- `pods_log` - Get pod logs
- `pods_get` - Get pod details
- `pods_delete` - Delete pods
- `pods_exec` - Execute commands in pods
- `pods_run` - Run new pods
- `resources_list`, `resources_get`, `resources_create_or_update`, `resources_delete` - Generic resource operations
- `helm_list`, `helm_install`, `helm_uninstall` - Helm operations
- `events_list` - List cluster events

**Only use kubectl via Bash when:**
- MCP tools don't support the operation (e.g., `kubectl apply -f`)
- You need complex label selectors or field selectors
- Debugging MCP connectivity issues

When using kubectl, always specify `--context kind-gadget-dev`.

### VM Operations

The Kind cluster runs inside a Lima VM. For operations inside the VM:

```bash
# Shell into VM
limactl shell gadget-k8s-host -- <command>

# Examples
limactl shell gadget-k8s-host -- docker images
limactl shell gadget-k8s-host -- kind load docker-image <image> --name gadget-dev
```

### Common Workflows

**Check pod status:**
```
Use: pods_list_in_namespace(namespace="default")
```

**View pod logs:**
```
Use: pods_log(name="<pod-name>", namespace="<namespace>")
```

**Restart pods (e.g., after image update):**
```
Use: pods_delete(name="<pod-name>", namespace="<namespace>")
# DaemonSet/Deployment will recreate it
```

**Apply manifests** (MCP doesn't support file-based apply):
```bash
kubectl apply -f <file>.yaml --context kind-gadget-dev
```

**Load images into Kind:**
```bash
docker save <image> | limactl shell gadget-k8s-host -- docker load
limactl shell gadget-k8s-host -- kind load docker-image <image> --name gadget-dev
```
