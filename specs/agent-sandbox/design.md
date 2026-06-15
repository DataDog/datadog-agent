# Agent Sandbox Design

## Traceability Overview

This design describes the technical approach for the Agent Sandbox requirements. Each section lists the requirements it supports. The design covers both sandbox modes as first-class capabilities: the Host Agent sandbox for published host packages and the Kubernetes Agent sandbox for published container images.

| Design section | Requirements |
| --- | --- |
| CLI surface and command semantics | REQ-AS-001, REQ-AS-003, REQ-AS-004, REQ-AS-005, REQ-AS-006, REQ-AS-007, REQ-AS-008, REQ-AS-009, REQ-AS-010, REQ-AS-011, REQ-AS-012 |
| State directory model | REQ-AS-001, REQ-AS-006, REQ-AS-007, REQ-AS-010, REQ-AS-012 |
| VM image and instance lifecycle | REQ-AS-001, REQ-AS-002, REQ-AS-007, REQ-AS-008 |
| Virtualization.framework helper boundary | REQ-AS-002, REQ-AS-005, REQ-AS-006, REQ-AS-007, REQ-AS-008 |
| Guest provisioning model | REQ-AS-001, REQ-AS-003, REQ-AS-004, REQ-AS-008, REQ-AS-009, REQ-AS-011 |
| SSH and command execution model | REQ-AS-005, REQ-AS-006, REQ-AS-010 |
| Host Agent package model | REQ-AS-003, REQ-AS-004, REQ-AS-006, REQ-AS-012 |
| Kubernetes cluster and Agent deployment model | REQ-AS-008, REQ-AS-009, REQ-AS-010, REQ-AS-011, REQ-AS-012 |
| Error handling and cleanup guarantees | REQ-AS-004, REQ-AS-006, REQ-AS-007, REQ-AS-011, REQ-AS-012 |

## CLI Surface and Command Semantics

**Traces to:** REQ-AS-001, REQ-AS-003, REQ-AS-004, REQ-AS-005, REQ-AS-006, REQ-AS-007, REQ-AS-008, REQ-AS-009, REQ-AS-010, REQ-AS-011, REQ-AS-012

The sandbox CLI presents sandbox operations as local commands organized around named sandbox instances and an explicit mode: `host-agent` or `kubernetes-agent`. The mode is persisted in instance metadata and displayed in status output so command behavior is unambiguous.

Core command groups:

- `create`: creates a named sandbox in the selected mode. Host Agent creation accepts a published Agent version and optional host Agent configuration override. Kubernetes Agent creation accepts a published Agent image reference and optional deployment configuration.
- `start` and `stop`: control guest execution for an existing sandbox without changing mutable guest state.
- `status`: reports lifecycle state, mode, selected Agent artifact, configuration source, guest reachability, and mode-specific Agent health.
- `destroy`: stops the sandbox if needed and removes instance-specific mutable state and managed connection metadata.
- `agent`: runs supported host Agent subcommands in a host Agent sandbox and returns command exit status, standard output, and standard error.
- `logs`: retrieves recent host Agent logs or Kubernetes Agent pod logs, depending on mode.
- `shell` and `ssh`: open an interactive guest shell or SSH session using managed connection details.
- `config apply`: applies host Agent configuration or Kubernetes Agent deployment configuration to an existing sandbox.
- `kubeconfig`: writes or prints host-usable Kubernetes access configuration for a running Kubernetes Agent sandbox.

Unsupported capabilities are rejected at the CLI boundary with an explicit out-of-scope message. Examples include Podman-backed sandboxes, cloud installs, QEMU/libvirt hosts, local Agent source overlays, multi-node Kubernetes clusters, local image builds, and Agent Operator deployments.

## State Directory Model

**Traces to:** REQ-AS-001, REQ-AS-006, REQ-AS-007, REQ-AS-010, REQ-AS-012

The sandbox stores all local state beneath a single sandbox root directory. The default root is a dedicated user-level directory, `$HOME/.dd-agent-dev/sandbox`, not the current Agent worktree, not the root Agent checkout, and not a Phoenix task worktree. The CLI accepts an explicit state-root override for users who need a different disk location. The root is divided into cache state and instance state.

Cache state contains reusable, immutable base image material keyed by operating system image identity and architecture. Cache entries are not sandbox instances and are not removed by normal instance destruction.

Instance state contains one directory per named sandbox. Each instance directory contains:

- metadata describing sandbox name, mode, guest architecture, lifecycle state, selected Agent artifact, configuration source, and creation timestamp;
- mutable disk overlays or copied disk images for the guest;
- generated SSH keys and connection metadata;
- rendered provisioning inputs used for that instance;
- exported kubeconfig material for Kubernetes Agent sandboxes;
- operation logs produced by the sandbox CLI and helper processes.

Destroy removes only the instance directory and live guest execution associated with that instance. Cache cleanup is a separate user-requested operation because reusable base image material is shared across experiments.

## VM Image and Instance Lifecycle

**Traces to:** REQ-AS-001, REQ-AS-002, REQ-AS-007, REQ-AS-008

The VM lifecycle uses a reusable base image plus per-instance mutable state. Base image preparation obtains Ubuntu guest image material compatible with Apple-native virtualization on the host architecture. Instance creation derives sandbox-specific mutable guest storage from the cached base material and records the relationship in metadata.

Lifecycle states are persisted as `absent`, `created`, `running`, `stopped`, `error`, and `destroyed`. `absent` describes a name with no instance directory. `created` means local instance state exists but the guest is not running. `running` means the virtualization helper has guest execution active. `stopped` means instance state remains but guest execution is inactive. `error` means an operation failed after creating or discovering instance state, leaving the instance inspectable. `destroyed` is an operation result after instance state has been removed; the persisted instance record no longer exists.

Creation always produces isolated mutable guest state. A new sandbox never attaches a writable disk from a previously destroyed sandbox. Start and stop preserve instance state. Destroy stops guest execution, removes instance state, and leaves cache state intact.

## Virtualization.framework Helper Boundary

**Traces to:** REQ-AS-002, REQ-AS-005, REQ-AS-006, REQ-AS-007, REQ-AS-008

A small macOS helper owns interactions with Apple Virtualization.framework. Its responsibility is limited to guest compute lifecycle and host-to-guest connectivity primitives:

- validate that the host supports the required Apple-native virtualization capability;
- create VM configurations from prepared disk images, kernel or bootloader inputs, networking configuration, and generated SSH material;
- start, stop, and report process-level state for guest execution;
- expose connection coordinates needed by the CLI's SSH layer;
- terminate guest execution during destroy.

The helper does not install the Agent, mutate `datadog.yaml`, install Kubernetes, deploy Helm charts, or interpret Agent command output. Those behaviors are guest provisioning and CLI responsibilities. Keeping this boundary narrow ensures the substrate remains Apple-native without embedding Agent-specific logic in the virtualization layer.

## Guest Provisioning Model

**Traces to:** REQ-AS-001, REQ-AS-003, REQ-AS-004, REQ-AS-008, REQ-AS-009, REQ-AS-011

Provisioning is declarative from the CLI's perspective: each instance has rendered provisioning inputs derived from mode, requested Agent artifact, and user-provided configuration. Provisioning scripts run inside the guest over the managed command-execution channel and write operation output to the instance log.

For Host Agent sandboxes, provisioning prepares the Ubuntu guest, installs the selected Datadog Agent host package from published artifacts, applies the host Agent configuration override when supplied, and ensures the Agent service can be started and inspected.

For Kubernetes Agent sandboxes, provisioning prepares the Ubuntu guest, installs and starts a single-node lightweight Kubernetes distribution, configures host-reachable cluster access, deploys the Datadog Agent using the selected published container image, and applies user-provided deployment configuration.

Provisioning records artifact identities and configuration sources in instance metadata. If provisioning fails, the sandbox transitions to `error` and remains available for shell access and log inspection when the guest is reachable.

## SSH and Command Execution Model

**Traces to:** REQ-AS-005, REQ-AS-006, REQ-AS-010

Each instance receives generated SSH credentials stored in its instance directory under the sandbox state root. The CLI resolves connection details from metadata and never requires the user to pass key paths, guest addresses, or usernames for supported commands or direct SSH access.

Non-interactive commands execute through a common command runner that captures exit status, standard output, standard error, start time, and end time. The runner streams or prints output according to command type while preserving the underlying exit status for automation.

Direct SSH access is exposed through the same local command surface as other sandbox operations, including an invoke-friendly wrapper suitable for `dda inv` or equivalent task integration. The wrapper supplies the managed key, username, host, and port, then hands control to the user's terminal session. Interactive shell commands may use the direct SSH path or a narrower guest shell command, but both resolve connection details from sandbox metadata. Status and log commands use the same connection model and report the failed connection step when the guest is unreachable.

Kubeconfig export uses managed guest access to retrieve cluster connection material, rewrites it for host-side access through the sandbox connection path, and stores the exported kubeconfig in the instance directory.

## Host Agent Package Model

**Traces to:** REQ-AS-003, REQ-AS-004, REQ-AS-006, REQ-AS-012

The Host Agent sandbox installs Datadog Agent host packages from published package repositories or published package artifact locations. The selected version is stored in metadata and verified from inside the guest after installation. A documented default version selection policy is used when the user does not provide a version.

Host Agent configuration overrides are treated as user inputs owned by the instance. The CLI records the source path or inline configuration identity, transfers the rendered configuration into the guest, applies it to the Agent's configuration location, and makes the service state observable after application.

Host Agent inspection commands include service status, recent logs, `agent status`, and supported Agent subcommands. Command output is returned directly to the user with process exit status. Local source checkout overlays and local replacement binaries are not part of the Host Agent package model.

## Kubernetes Cluster and Agent Deployment Model

**Traces to:** REQ-AS-008, REQ-AS-009, REQ-AS-010, REQ-AS-011, REQ-AS-012

The Kubernetes Agent sandbox runs one lightweight Kubernetes control plane and worker in the same guest VM. Cluster state lives entirely inside the sandbox's mutable instance state and is removed with the sandbox.

The Agent deployment path uses a published container image reference. The user may provide the image reference; otherwise the CLI uses a documented default image selection policy. Deployment configuration is accepted as Helm values or an equivalent declarative configuration input and is recorded in instance metadata.

Cluster inspection commands report node readiness, core system pod state, Agent workload state, selected image reference, and deployment configuration source. Agent logs are read from the Agent workload. The CLI exports a host-usable kubeconfig so the user can run host-side Kubernetes clients without manually copying credentials from the guest.

The Kubernetes model is intentionally single-node and single-distribution for this sandbox. Managed Kubernetes parity, multi-node clusters, Kubernetes distribution matrices, CNI/runtime matrices, local image builds, local registries, and Agent Operator deployment are rejected as out of scope.

## Error Handling and Cleanup Guarantees

**Traces to:** REQ-AS-004, REQ-AS-006, REQ-AS-007, REQ-AS-011, REQ-AS-012

Every mutating operation records an operation log in the instance directory when an instance directory exists. Failures report the operation that failed, the sandbox name, lifecycle state, and the next useful inspection command when one exists.

Configuration-application failures preserve the sandbox and its prior inspectable state. Host Agent configuration failures do not destroy the VM or remove logs. Kubernetes deployment configuration failures do not destroy the cluster or remove deployment diagnostics.

Destroy is idempotent from the user's perspective. Destroying an absent sandbox reports absence. Destroying a stopped sandbox removes instance state. Destroying a running sandbox stops guest execution before removing instance state. Destroying a sandbox in `error` state still removes instance state after terminating any live guest process associated with the instance.

Scope errors occur before mutating state. When the user requests an unsupported substrate or workflow, the CLI reports the unsupported capability and leaves existing sandbox state unchanged.
