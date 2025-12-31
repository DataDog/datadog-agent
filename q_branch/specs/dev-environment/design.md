# Gadget Development Environment - Technical Design

## Architecture Overview

The development environment uses a two-layer virtualization approach:

```
┌─────────────────────────────────────────────────────────┐
│  macOS Host                                             │
│  ├── Lima (VM management)                               │
│  ├── Docker Desktop (optional, for image builds)        │
│  └── kubectl (via port-forwarded kubeconfig)            │
│                                                         │
│  ┌───────────────────────────────────────────────────┐  │
│  │  Lima VM: gadget-k8s-host (Ubuntu 24.04)          │  │
│  │  ├── Docker CE                                    │  │
│  │  ├── Kind                                         │  │
│  │  └── kubectl, helm                                │  │
│  │                                                   │  │
│  │  ┌─────────────────────────────────────────────┐  │  │
│  │  │  Kind Cluster: gadget-dev                   │  │  │
│  │  │  ├── control-plane node                     │  │  │
│  │  │  ├── worker node 1                          │  │  │
│  │  │  └── worker node 2                          │  │  │
│  │  └─────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
```

## Component Details

### Lima VM Configuration

- **OS:** Ubuntu 24.04 LTS (arm64 for Apple Silicon)
- **Resources:** 6 CPUs, 12GB RAM, 80GB disk
- **Port Forwarding:**
  - 6443: Kubernetes API server
  - 30000-30100: NodePort range for services

### Kind Cluster Configuration

- **Cluster Name:** gadget-dev
- **Nodes:** 1 control-plane + 2 workers
- **API Server:** Bound to 127.0.0.1:6443 (accessible via Lima port forward)
- **NodePort Mappings:** 30000, 30080, 30443 exposed to host

### Pre-installed Tools

| Tool | Version | Purpose |
|------|---------|---------|
| Docker CE | Latest stable | Container runtime for Kind |
| Kind | v0.27.0 | Kubernetes-in-Docker cluster |
| kubectl | v1.32 | Kubernetes CLI |
| Helm | Latest | Package management, Datadog Operator install |

## Implementation Notes

### REQ-DE-001: Isolated Environment

- **Location:** `gadget-k8s-host.lima.yaml`, `setup-k8s-host.sh`
- **Approach:** Lima VM provides full isolation from macOS. Kind cluster runs inside VM, further isolating Kubernetes workloads.
- **Trade-offs:** Two-layer virtualization adds overhead but provides complete isolation and a real Linux kernel.

### REQ-DE-002: Run kubectl from macOS

- **Location:** `setup-k8s-host.sh` (merge_kubeconfig function)
- **Approach:** Extract Kind kubeconfig, merge into `~/.kube/config`. Lima forwards port 6443 so kubectl connects via localhost:6443.
- **Trade-offs:** Merging into default kubeconfig means context appears in kubectx. Stale entries cleaned before merge to avoid certificate conflicts.

### REQ-DE-003: Access Linux Kernel for eBPF

- **Location:** `gadget-k8s-host.lima.yaml`
- **Approach:** Lima VM runs Ubuntu 24.04 with full kernel. Kind nodes (Docker containers) share VM's kernel, enabling eBPF program loading and /proc, /sys access.
- **Trade-offs:** VM kernel is Ubuntu 24.04's default. Kernel version may differ from production environments.

### REQ-DE-004: Fast Iteration

- **Location:** README.md (Dev Loop section)
- **Approach:** Three-step image transfer: macOS Docker → Lima Docker → Kind nodes. Uses `docker save | docker load` and `kind load docker-image`.
- **Trade-offs:** Image transfer adds ~30-60s per iteration. Could be optimized with registry but adds complexity.

### REQ-DE-005: Operator Deployment

- **Location:** `gadget-k8s-host.lima.yaml` (Helm repo setup), `test-cluster.yaml`
- **Approach:** Datadog Helm repo added during provisioning. Developer installs operator, then applies DatadogAgent CR.
- **Trade-offs:** Manual operator installation step. Could automate but adds time to initial setup.

## File Locations

| File | Purpose |
|------|---------|
| `gadget-k8s-host.lima.yaml` | Lima VM definition |
| `setup-k8s-host.sh` | One-time setup script |
| `test-cluster.yaml` | Sample DatadogAgent CR for testing |
| `README.md` | Usage instructions |

## Security Considerations

- API keys in test-cluster.yaml are dummy values (`a00000001`) - safe for local testing
- Kubeconfig permissions set to 600 (user-only read/write)
- VM is isolated from macOS filesystem (no mounts configured)

## Limitations

- **Shared Kernel:** All Kind nodes share the VM's kernel. Cannot test kernel version differences.
- **No Swap by Default:** Kubernetes disables swap. Testing swap-related features requires additional configuration.
- **Single VM:** All nodes run in one VM. Cannot simulate network partitions between nodes.
