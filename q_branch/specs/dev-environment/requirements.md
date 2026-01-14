# Gadget Development Environment

## User Story

As a Datadog Agent developer, I need an isolated Kubernetes environment with a real Linux kernel so that I can test Agent features (including eBPF and system-level monitoring) without affecting production systems or requiring cloud infrastructure.

## Requirements

### REQ-DE-001: Test Kubernetes Features Safely

WHEN a developer runs the setup script
THE SYSTEM SHALL create an isolated Kubernetes cluster

WHEN the developer deploys test workloads
THE SYSTEM SHALL isolate all effects to the development environment

**Rationale:** Developers need to test Kubernetes integrations (DaemonSets, service discovery, metadata collection) without risk to production systems. An isolated environment enables fearless experimentation with potentially disruptive configurations.

---

### REQ-DE-002: Run kubectl from macOS

WHEN the cluster is ready
THE SYSTEM SHALL add cluster credentials to ~/.kube/config

WHEN the developer runs `kubectl --context kind-gadget-dev <command>`
THE SYSTEM SHALL execute that command against the Kind cluster inside the VM

**Rationale:** Developers need to inspect pods, view logs, and apply manifests without SSH-ing into the VM for every command. Direct kubectl access from macOS enables rapid debugging.

---

### REQ-DE-003: Access Linux Kernel for eBPF

WHEN the developer deploys an Agent with eBPF-based features (NPM, USM, CWS)
THE SYSTEM SHALL provide a real Linux kernel (not Docker Desktop's VM)

WHEN Agent code accesses /proc, /sys, or loads eBPF programs
THE SYSTEM SHALL expose these kernel interfaces to containers in the Kind cluster

**Rationale:** eBPF programs require a real Linux kernel. Docker Desktop on macOS runs a minimal VM that may lack kernel features. A dedicated Ubuntu VM provides a known, full-featured kernel for reliable testing.

---

### REQ-DE-004: Iterate Quickly on Code Changes

WHEN a developer builds a new Agent image
THE SYSTEM SHALL support deploying that image to the cluster

WHEN a developer updates code and rebuilds
THE SYSTEM SHALL allow redeployment without recreating the cluster

**Rationale:** Fast iteration is critical for productivity. If testing a code change takes 30+ minutes, developers will skip tests or batch too many changes together. A fast loop encourages thorough testing.

---

### REQ-DE-005: Deploy Agents via Operator

WHEN the cluster is ready
THE SYSTEM SHALL support deploying Datadog Agents via the Datadog Operator

WHEN a developer applies a DatadogAgent custom resource
THE SYSTEM SHALL deploy the Agent DaemonSet with specified configuration

**Rationale:** The Datadog Operator is the recommended deployment method for Kubernetes. Developers need to test Agent behavior as customers will actually deploy it, not just as standalone containers.

---
