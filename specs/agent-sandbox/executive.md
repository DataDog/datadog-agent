# Agent Sandbox Executive Summary

## Project Purpose

Agent Sandbox is a specification for a macOS-native, disposable Datadog Agent inspection environment. It helps Agent leaders and engineers answer production-behavior questions quickly on a local Mac, without Podman, Docker as the host substrate, cloud infrastructure, QEMU/libvirt, source checkout overlays, or local build replacement workflows.

The specification separates stable requirements, technical design, and delivery status. Stage A implementation must not begin until the requirements in `requirements.md` are reviewed and signed off.

## Named User Journeys

- **JY-AS-01: Clean Host Agent Inspection** — inspect a published Agent host package on a clean Ubuntu host and discard the environment.
- **JY-AS-02: Configuration Experiment** — apply a local Agent configuration override and observe production Agent behavior.
- **JY-AS-03: Subcommand Triage** — run common Agent subcommands without manually managing SSH details.
- **JY-AS-04: Disposable Environment Cleanup** — destroy mutable sandbox state predictably after an experiment.
- **JY-AS-05: Local Kubernetes Agent Inspection** — run a local single-VM Kubernetes cluster and deploy the Agent from a published container image.

## Stage A MVP Scope

Stage A covers the Host Agent sandbox: macOS Apple Virtualization.framework substrate, Apple Silicon first, local Ubuntu VM lifecycle, cached base-image reuse, clean per-instance state, SSH access, published Datadog Agent host package installation, published Agent version selection, local Agent configuration override application, convenient Agent command execution, status/log inspection, start/stop, and destroy.

Stage A excludes Podman, Docker as host substrate, cloud installs, QEMU/libvirt, Kubernetes, multi-VM clusters, source checkout overlays, local build replacement, fakeintake, and local package installation.

## Stage B MVP Scope

Stage B covers the Kubernetes Agent sandbox on the same local VM lifecycle and state model: a lightweight in-VM Kubernetes distribution, host-side kubeconfig export, Datadog Agent deployment from a published container image, user-provided Agent image references, user-provided Helm values or equivalent configuration, Agent and cluster status inspection, logs, and destroy.

Stage B excludes managed Kubernetes cloud parity, multi-node clusters, Kubernetes distribution matrices, CNI/runtime matrices, local image builds, local registries except if required for published-image access, and Agent Operator deployment.

## Global Out of Scope

Agent Sandbox is not a general Agent development environment, cloud environment manager, container host abstraction, Kubernetes conformance environment, local build launcher, fakeintake harness, or multi-platform virtualization matrix. The project boundary is fast local inspection of published Agent artifacts in disposable environments.

## Requirement Status

| Requirement | User-benefit title | Status | Stage relevance |
| --- | --- | --- | --- |
| REQ-AS-001 | Create a Clean Host Sandbox Quickly | 🔄 In Progress | Stage A |
| REQ-AS-002 | Stay Local and Apple-Native on macOS | 🔄 In Progress | Stage A, Stage B |
| REQ-AS-003 | Install a Published Host Agent Version | 🔄 In Progress | Stage A |
| REQ-AS-004 | Apply Host Agent Configuration Overrides | 🔄 In Progress | Stage A |
| REQ-AS-005 | Run Agent Commands and SSH Without Manual Credential Management | 🔄 In Progress | Stage A |
| REQ-AS-006 | Inspect Sandbox and Host Agent Health | 🔄 In Progress | Stage A |
| REQ-AS-007 | Destroy Sandbox State Predictably | 🔄 In Progress | Stage A, Stage B |
| REQ-AS-008 | Create a Local Kubernetes Agent Sandbox | ⏭️ Planned | Stage B |
| REQ-AS-009 | Deploy a Published Agent Container Image | ⏭️ Planned | Stage B |
| REQ-AS-010 | Export Kubernetes Access for Host Tooling | ⏭️ Planned | Stage B |
| REQ-AS-011 | Apply Kubernetes Agent Configuration | ⏭️ Planned | Stage B |
| REQ-AS-012 | Keep Scope Boundaries Observable | 🔄 In Progress | Stage A, Stage B |

## Signoff Checkpoint

Stage A implementation signoff is complete for the current MVP scope. The implementation now traces commits, tests, and review discussion back to the relevant `REQ-AS-###` IDs. Stage B implementation remains blocked until its requirements are separately reviewed for execution readiness.
