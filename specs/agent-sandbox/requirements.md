# Agent Sandbox Requirements

## Purpose

Agent Sandbox gives Datadog Agent engineering leaders and engineers a fast, disposable, local environment for inspecting production Agent behavior. It supports two user-visible sandbox modes:

- **Host Agent sandbox**: a clean Ubuntu host with the Datadog Agent installed from published host-package artifacts.
- **Kubernetes Agent sandbox**: a clean local VM that runs a lightweight Kubernetes cluster and deploys the Datadog Agent from a published container image.

The system is optimized for answering concrete Agent behavior questions without requiring Podman, Docker as the host substrate, cloud infrastructure, QEMU/libvirt, or source-build replacement workflows.

## Named User Journeys

### JY-AS-01: Clean Host Agent Inspection

A Datadog Agent engineering manager needs to inspect how a production Agent behaves on a clean Ubuntu host. They create a disposable local sandbox, choose a published Agent version, inspect status and logs, and discard the environment after the question is answered.

### JY-AS-02: Configuration Experiment

A former Agent engineer needs to understand how a production Agent reacts to a specific `datadog.yaml` setting. They apply a local configuration override, start or restart the Agent in the sandbox, run inspection commands, and compare the observed behavior against expectations.

### JY-AS-03: Subcommand Triage

An Agent reviewer needs to answer what a common Agent subcommand does on a clean host install. They run the subcommand through a local CLI wrapper without manually managing SSH connection details.

### JY-AS-04: Disposable Environment Cleanup

An engineer needs confidence that a throwaway experiment does not pollute later experiments. They destroy the sandbox and expect its mutable VM state, guest configuration, and local connection metadata to be removed predictably.

### JY-AS-05: Local Kubernetes Agent Inspection

An Agent maintainer needs to reproduce basic Kubernetes Agent behavior locally using a published Agent container image. They create a local single-VM Kubernetes sandbox, provide an Agent image tag and deployment values, use host-side `kubectl`, inspect Agent and cluster state, and destroy the sandbox when done.

## Requirements

### REQ-AS-001: Create a Clean Host Sandbox Quickly

**User benefit:** A user can answer host Agent behavior questions without waiting for cloud infrastructure or rebuilding an operating system image for every experiment.

**Rationale:** JY-AS-01 depends on fast iteration and a clean host baseline. Reusing immutable base image material while creating fresh instance state gives speed without carrying behavior from prior experiments.

**EARS:**

- WHEN the user requests a host Agent sandbox, THE SYSTEM SHALL create a fresh local Ubuntu host instance with isolated mutable state for that sandbox.
- WHERE reusable base image material for the requested Ubuntu host is already available, THE SYSTEM SHALL create the fresh sandbox without downloading that base image material again.
- THE SYSTEM SHALL NOT reuse mutable guest state from a previously destroyed sandbox when creating a new sandbox.

### REQ-AS-002: Stay Local and Apple-Native on macOS

**User benefit:** A macOS user can run the sandbox without installing or operating Podman, a cloud environment, QEMU/libvirt, or a separate Linux virtualization stack.

**Rationale:** JY-AS-01, JY-AS-02, and JY-AS-03 are valuable because the environment is immediately available on the user's Mac and avoids operational overhead unrelated to Agent behavior inspection.

**EARS:**

- WHILE the sandbox is running on macOS, THE SYSTEM SHALL run guest compute using the host operating system's Apple-native virtualization capability.
- THE SYSTEM SHALL NOT require Podman, Docker as the host substrate, cloud infrastructure, QEMU, or libvirt to create, run, inspect, or destroy a sandbox.
- IF the host cannot provide the required Apple-native virtualization capability, THE SYSTEM SHALL report that the sandbox cannot run on that host.

### REQ-AS-003: Install a Published Host Agent Version

**User benefit:** A user observes production Agent behavior rather than behavior from a local checkout or custom build.

**Rationale:** JY-AS-01 and JY-AS-03 require confidence that the inspected binary matches a published Agent host package.

**EARS:**

- WHEN the user requests a host Agent sandbox with an Agent version, THE SYSTEM SHALL install the Datadog Agent from published host-package artifacts for that version.
- WHEN the user requests a host Agent sandbox without an explicit Agent version, THE SYSTEM SHALL use a documented default published Agent version selection policy.
- THE SYSTEM SHALL make the installed Agent version observable to the user from the sandbox CLI.
- THE SYSTEM SHALL NOT replace the installed Agent with binaries from a local source checkout.

### REQ-AS-004: Apply Host Agent Configuration Overrides

**User benefit:** A user can test how production Agent behavior changes under a specific configuration without hand-editing files inside the guest.

**Rationale:** JY-AS-02 centers on configuration experiments, including applying a local `datadog.yaml` or equivalent override before observing Agent behavior.

**EARS:**

- WHEN the user supplies a host Agent configuration override for a sandbox, THE SYSTEM SHALL apply that override to the Agent configuration used in the sandbox.
- WHEN the user changes the host Agent configuration override for an existing sandbox, THE SYSTEM SHALL provide a command that applies the new override to that sandbox.
- WHEN a configuration override is applied to an existing sandbox, THE SYSTEM SHALL make the Agent's resulting running state observable to the user.
- IF the supplied configuration override cannot be applied, THE SYSTEM SHALL report the failure and preserve the sandbox for inspection.

### REQ-AS-005: Run Agent Commands and SSH Without Manual Credential Management

**User benefit:** A user can answer subcommand and diagnostics questions, or directly inspect the guest, without remembering guest addresses, keys, usernames, or service paths.

**Rationale:** JY-AS-03 requires both a convenient command wrapper for common Agent inspection tasks and direct SSH access for ad hoc debugging. In both cases the system manages connection details so the user does not handle credentials manually.

**EARS:**

- WHEN the user requests a supported Agent subcommand for a running host Agent sandbox, THE SYSTEM SHALL execute that subcommand inside the sandbox and return its exit status, standard output, and standard error to the user.
- WHEN the user requests direct SSH access for a running sandbox, THE SYSTEM SHALL open an SSH session using the sandbox's managed connection details.
- WHEN the user requests an interactive shell for a running sandbox, THE SYSTEM SHALL open a guest shell using the sandbox's managed connection details.
- THE SYSTEM SHALL provide a local command wrapper for supported sandbox commands and direct SSH access.
- THE SYSTEM SHALL NOT require the user to manually provide SSH key paths, guest IP addresses, or guest usernames for supported sandbox commands or direct SSH access.

### REQ-AS-006: Inspect Sandbox and Host Agent Health

**User benefit:** A user can decide whether observed Agent behavior is meaningful by checking VM state, service health, and logs from one local interface.

**Rationale:** JY-AS-01, JY-AS-02, and JY-AS-03 require quick visibility into whether the guest and Agent are running correctly before interpreting command output.

**EARS:**

- WHEN the user requests sandbox status, THE SYSTEM SHALL report the sandbox lifecycle state and the information needed to identify the selected Agent version and configuration source.
- WHEN the user requests host Agent health, THE SYSTEM SHALL report the Agent service state from inside the sandbox.
- WHEN the user requests host Agent logs, THE SYSTEM SHALL retrieve recent Agent logs from inside the sandbox.
- IF the sandbox is not reachable, THE SYSTEM SHALL report the last known sandbox lifecycle state and the failed connection operation.

### REQ-AS-007: Destroy Sandbox State Predictably

**User benefit:** A user can run risky or noisy experiments and confidently discard all sandbox-specific effects afterward.

**Rationale:** JY-AS-04 depends on deterministic cleanup so that future experiments start from a clean baseline.

**EARS:**

- WHEN the user destroys a sandbox, THE SYSTEM SHALL stop guest execution associated with that sandbox.
- WHEN the user destroys a sandbox, THE SYSTEM SHALL remove that sandbox's mutable guest state and managed connection metadata.
- WHEN destroy completes, THE SYSTEM SHALL make the sandbox unavailable for further command execution except commands that report its absence.
- THE SYSTEM SHALL NOT remove reusable base image material unless the user requests base image cache cleanup.

### REQ-AS-008: Create a Local Kubernetes Agent Sandbox

**User benefit:** A user can inspect Kubernetes Agent behavior locally without creating cloud infrastructure or a multi-node cluster.

**Rationale:** JY-AS-05 requires a Kubernetes environment that is disposable and local while sharing the same sandbox lifecycle expectations as the host Agent sandbox.

**EARS:**

- WHEN the user requests a Kubernetes Agent sandbox, THE SYSTEM SHALL create a fresh local Ubuntu VM with isolated mutable state for that sandbox.
- WHEN the Kubernetes Agent sandbox is created, THE SYSTEM SHALL run a single-node lightweight Kubernetes cluster inside the sandbox.
- THE SYSTEM SHALL NOT require managed Kubernetes, multiple nodes, or cloud infrastructure to create, run, inspect, or destroy a Kubernetes Agent sandbox.
- THE SYSTEM SHALL remove Kubernetes cluster state when the Kubernetes Agent sandbox is destroyed.

### REQ-AS-009: Deploy a Published Agent Container Image

**User benefit:** A user observes behavior from a published Agent container image and can choose the image under investigation.

**Rationale:** JY-AS-05 depends on reproducing Kubernetes Agent behavior from distributed images, not from local image builds.

**EARS:**

- WHEN the user requests a Kubernetes Agent sandbox with an Agent image reference, THE SYSTEM SHALL deploy the Datadog Agent to the in-sandbox Kubernetes cluster using that published image reference.
- WHEN the user requests a Kubernetes Agent sandbox without an explicit Agent image reference, THE SYSTEM SHALL use a documented default published Agent image selection policy.
- THE SYSTEM SHALL make the deployed Agent image reference observable to the user from the sandbox CLI.
- THE SYSTEM SHALL NOT require a local image build or local image registry to deploy the Agent from a published image reference.

### REQ-AS-010: Export Kubernetes Access for Host Tooling

**User benefit:** A user can use familiar host-side Kubernetes tools to inspect the local sandbox cluster.

**Rationale:** JY-AS-05 includes host-side `kubectl` inspection as part of understanding Agent and cluster behavior.

**EARS:**

- WHEN the user requests Kubernetes access for a running Kubernetes Agent sandbox, THE SYSTEM SHALL provide a kubeconfig that enables host-side Kubernetes clients to reach the in-sandbox cluster.
- WHEN the Kubernetes Agent sandbox is stopped or destroyed, THE SYSTEM SHALL make the exported access state reflect that the cluster is unavailable.
- THE SYSTEM SHALL NOT require the user to manually copy Kubernetes credentials from inside the guest.

### REQ-AS-011: Apply Kubernetes Agent Configuration

**User benefit:** A user can test Kubernetes Agent deployment settings without manually editing manifests inside the cluster.

**Rationale:** JY-AS-05 requires user-provided Helm values or equivalent configuration to inspect how published Agent images behave under chosen settings.

**EARS:**

- WHEN the user supplies Kubernetes Agent deployment configuration for a Kubernetes Agent sandbox, THE SYSTEM SHALL apply that configuration to the Agent deployment in the sandbox cluster.
- WHEN the user changes Kubernetes Agent deployment configuration for an existing Kubernetes Agent sandbox, THE SYSTEM SHALL provide a command that applies the new configuration to the Agent deployment.
- IF Kubernetes Agent deployment configuration cannot be applied, THE SYSTEM SHALL report the failure and preserve the sandbox for inspection.
- WHEN Kubernetes Agent deployment configuration is applied, THE SYSTEM SHALL make the resulting Agent deployment state observable to the user.

### REQ-AS-012: Keep Scope Boundaries Observable

**User benefit:** A user and reviewer can tell whether a behavior belongs to Agent Sandbox without relying on private assumptions or project history.

**Rationale:** All named journeys depend on a narrow disposable inspection tool. Explicit boundaries prevent the sandbox from expanding into a general cloud, container, source-build, or Kubernetes matrix framework.

**EARS:**

- WHEN the user requests a capability outside the documented sandbox modes, THE SYSTEM SHALL report that the capability is outside Agent Sandbox scope.
- THE SYSTEM SHALL make the active sandbox mode observable in status output.
- THE SYSTEM SHALL distinguish host Agent package inspection from Kubernetes Agent image inspection in its CLI output and persisted sandbox metadata.
- THE SYSTEM SHALL NOT present Podman, cloud installs, QEMU/libvirt, multi-VM clusters, local source checkout overlays, local Agent package installation, local image builds, Kubernetes distribution matrices, CNI/runtime matrices, or Agent Operator deployment as Agent Sandbox capabilities.
