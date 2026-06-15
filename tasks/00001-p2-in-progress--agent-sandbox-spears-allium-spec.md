# Draft unified spEARS and Allium specification for Agent Sandbox

## Context

We want a clear, reviewable specification before implementing the macOS-native Datadog Agent sandbox. The sandbox is for an engineering manager / former Agent engineer who needs a very fast throwaway environment to inspect production Agent behavior, experiment with config options, and run Agent subcommands without using Podman, cloud infrastructure, or QEMU/libvirt.

This task is **specification-only**. Do not implement Stage A until the spEARS requirements are reviewed and signed off.

## User journey anchor

The primary user is a Datadog Agent engineering manager who needs to answer questions such as:

- “How does the production Agent behave when I set this config option?”
- “What does this Agent subcommand do on a clean Ubuntu host install?”
- “Can I quickly boot a clean environment, inspect behavior, and throw it away?”
- “Can I later do the same for a local Kubernetes environment with the Agent deployed from a published container image?”

Every requirement must trace back to one of these journeys or another explicitly named user journey.

## Scope

Create a unified specification under a new spec directory, proposed path:

- `specs/agent-sandbox/requirements.md`
- `specs/agent-sandbox/design.md`
- `specs/agent-sandbox/executive.md`
- optional Allium companion file if appropriate, e.g. `specs/agent-sandbox/agent-sandbox.allium` or an Allium section agreed with the repository’s existing conventions

Before creating files, check whether an existing spec covers this domain. If none exists, create the new `agent-sandbox` spec directory.

## Required workflows and references

Use the spEARS workflow from `/Users/scott.opell/.claude/skills/spears`:

1. Start with **Discover** to confirm user journeys and boundaries.
2. Then use **Write Specs** to draft the three-document spEARS system.
3. Apply the spEARS guardrails:
   - requirements are timeless ideal end state
   - design traces to REQ IDs
   - executive tracks temporal status and MVP staging
   - no implementation phasing language in `design.md`
   - no status or implementation details in `requirements.md`
   - no code blocks in `executive.md`

Also use `/allium` to capture the behavioral model in a way that complements, rather than duplicates, spEARS. The Allium content should make scope boundaries observable: entities, states, transitions, commands/surfaces, and contracts for Stage A and Stage B.

## Stage definitions to encode

### Stage A — Local VM direct host Ubuntu

Stage A defines a macOS Apple Virtualization.framework-backed local Ubuntu VM with the Datadog Agent installed as a host package from production published artifacts.

In scope for Stage A:

- macOS host using Apple Virtualization.framework only
- Apple Silicon first unless discovery identifies a must-have Intel path
- local Ubuntu VM lifecycle
- cached base image / fast reusable image flow
- clean per-sandbox instance state
- SSH access
- production Datadog Agent host package install
- selecting a published Agent version
- applying a local `datadog.yaml` or equivalent Agent config override
- running common Agent commands through a convenient CLI wrapper
- stopping, starting, status inspection, and destroying the sandbox

Out of scope for Stage A:

- Podman
- Docker as the host substrate
- cloud installs
- QEMU/libvirt
- Kubernetes
- multi-VM clusters
- source checkout overlay or local build replacement
- fakeintake unless explicitly justified during discovery
- local package install unless explicitly included as a Stage A requirement

### Stage B — Local VM with Kubernetes distribution inside

Stage B defines the same Virtualization.framework-backed local VM substrate, with a lightweight Kubernetes distribution running inside the VM and the Datadog Agent deployed to that cluster using a published container image.

In scope for Stage B:

- reuse Stage A VM lifecycle and state model
- install and manage a lightweight in-VM Kubernetes distribution, likely k3s unless discovery changes this
- export kubeconfig for host-side `kubectl`
- deploy Datadog Agent with a published Agent image
- support a user-provided Agent image tag/path
- support user-provided Helm values or equivalent configuration
- inspect Agent and cluster status from the CLI
- destroy all Stage B resources with the sandbox

Out of scope for Stage B:

- managed Kubernetes cloud parity
- multi-node clusters
- Kubernetes distribution matrix
- CNI/runtime matrix
- local image builds
- local registry unless required for published-image workflow
- Agent Operator path unless explicitly justified during discovery

## Expected spEARS requirement shape

Use requirement IDs in a single namespace, suggested abbreviation: `REQ-AS-###`.

Requirements should include user-benefit titles, rationale, and EARS statements. Candidate requirement themes to validate during discovery:

- Quickly create a clean local sandbox
- Use only Apple-native virtualization
- Install a production host Agent package
- Select a published Agent version
- Apply Agent configuration before or after creation
- Run Agent subcommands without remembering SSH details
- Inspect Agent logs and service health
- Destroy sandbox state predictably
- Create an in-VM Kubernetes sandbox
- Deploy the Agent from a published container image
- Export kubeconfig for host tooling
- Apply Kubernetes Agent configuration
- Keep Stage A and Stage B scope boundaries explicit

Do not blindly turn this list into requirements. Requirements must be justified by user journeys.

## Expected design content

`design.md` should describe the technical approach for the requirements without phasing language. It should include at least:

- CLI surface and command semantics
- state directory model
- VM image and instance lifecycle
- Virtualization.framework helper responsibility boundary
- guest provisioning approach
- SSH and command execution model
- Agent host install/configuration model
- Kubernetes install/deploy model for Stage B
- error handling and cleanup guarantees
- traceability from every section to REQ IDs

## Expected executive content

`executive.md` should summarize:

- project purpose
- named user journeys
- Stage A MVP scope
- Stage B MVP scope
- out-of-scope list
- requirement status table with user-benefit titles
- signoff checkpoint: Stage A implementation must not begin until requirements are reviewed

Follow spEARS constraints: concise summaries, status table with requirement titles, no code blocks.

## Deliverables

- A self-contained spEARS spec directory for Agent Sandbox.
- A complementary Allium behavioral model or clearly documented decision not to create one if it would duplicate spEARS without adding clarity.
- Requirements that make Stage A and Stage B scope observable and reviewable.
- No source-code implementation changes.
- No invoke task implementation yet.

## Acceptance criteria

- The spec can be understood without reading this conversation.
- Every requirement traces to a named user journey and user benefit.
- Every design section references at least one REQ ID.
- `requirements.md` contains no status, implementation details, migration language, or phasing.
- `design.md` contains no project-plan phasing language.
- `executive.md` contains status and MVP boundaries, with no code blocks.
- Stage A signoff is represented as an explicit checkpoint before implementation.
- The out-of-scope boundaries exclude Podman, cloud installs, and QEMU/libvirt for this project.
