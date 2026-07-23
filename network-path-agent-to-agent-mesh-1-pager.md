# Network Path Agent-to-Agent Mesh One-Pager

## Problem

Customers can use Network Path to understand connectivity from an Agent host to a configured destination, and dynamic Network Path can discover paths from observed traffic. What is missing is a Datadog-managed way to continuously validate network reachability between the Agents themselves.

This matters when customers need to understand whether hosts, nodes, clusters, subnets, availability zones, or VPC segments can reach each other. Today, they can configure individual Network Path tests manually, but that does not scale into a healthy mesh of Agent-to-Agent checks.

## Proposal

Introduce an opt-in Agent-to-Agent Network Path mesh.

Each participating Agent exposes a minimal TCP probe listener and advertises candidate endpoint addresses to Datadog. The backend builds a bounded, same-scope mesh and sends each source Agent its assigned peer tests through the `NETWORK_PATH` Remote Config product. Source Agents execute normal scheduled Network Path tests to the selected peer endpoints and report results with explicit Agent-to-Agent mesh identity.

In short:

```text
Agent advertises endpoint candidates
Backend computes safe bounded topology
Backend sends assigned tests via RC
Agent runs scheduled Network Path tests
Datadog displays Agent-to-Agent path health
```

## Customer Value

Agent-to-Agent mesh would help customers:

- Validate east-west reachability between infrastructure segments.
- Detect routing, firewall, security group, NetworkPolicy, or CNI regressions.
- Compare path health across clusters, subnets, availability zones, and VPCs.
- Reduce manual setup for repeated Network Path tests.
- Use Datadog Agent coverage as a natural testing fabric.

The feature is especially useful for large Kubernetes, cloud VM, hybrid, and segmented enterprise environments where connectivity issues are hard to localize.

## Product Shape

The feature should be opt-in and backend-managed.

Local Agent configuration enables the destination-side listener. Remote Config controls test assignment and schedule, but should not remotely open a new inbound listener on a host that was not locally prepared for it.

Recommended user-facing model:

```text
Enable Agent mesh for selected Agents or deployments.
Datadog discovers eligible Agent probe endpoints.
Datadog creates a bounded mesh within selected scopes.
Users view Agent-to-Agent Network Path health by scope, edge, source, and destination.
```

The product should not promise a global full mesh by default. Full mesh is expensive and unreliable at scale. The default should be bounded and scoped.

## Scope and Constraints

V1 should include:

- Explicit local Agent opt-in.
- Minimal TCP-only probe listener in the Core Agent.
- Automatic endpoint candidate advertisement.
- Optional manual address/interface override.
- Backend-mediated peer selection.
- Same-scope mesh policy by default.
- Bounded topology, not full mesh by default.
- Scheduled Network Path tests delivered through `NETWORK_PATH` RC.
- First-class mesh identity in emitted results.

V1 should not include:

- Agent-side peer gossip or peer scanning.
- Agent-local topology computation.
- Reuse of Agent IPC, system-probe APIs, or remote-agent registry as peer targets.
- Global org-wide full mesh by default.
- A guarantee that auto-discovered addresses are reachable in every environment.

## High-Level Architecture

```text
Destination Agent
  - Starts opt-in TCP probe listener.
  - Auto-discovers candidate addresses.
  - Advertises readiness, port, addresses, and scope metadata.

Backend
  - Ingests endpoint advertisements.
  - Groups Agents into conservative reachability scopes.
  - Computes bounded topology.
  - Chooses destination address per edge.
  - Emits scheduled Network Path tests through NETWORK_PATH RC.

Source Agent
  - Receives assigned Network Path tests.
  - Runs existing traceroute/E2E probing.
  - Emits Network Path results with mesh identity.
```

This keeps the Agent implementation focused and lets backend policy evolve without changing the Agent contract.

## Why This Is Feasible

The Agent already has most of the needed building blocks:

- Scheduled Network Path can run tests to a configured hostname and port.
- Dynamic Network Path already schedules path tests from discovered destinations.
- The planned `NETWORK_PATH` RC flow provides the right delivery mechanism for backend-created tests.
- Inventory Agent metadata can advertise Agent capabilities and listener readiness.
- Host and workload metadata already provide useful address and scope candidates.

The main new Agent work is the probe listener, endpoint advertisement, RC metadata handling, and payload identity. The larger product dependency is backend work: endpoint inventory, topology policy, edge generation, RC emission, suppression, and UI/API workflows.

## Key Risks

Address reachability is the main risk. Agents can discover candidate addresses, but only backend context can decide whether one Agent should try to reach another. Even then, firewalls, NAT, Kubernetes policies, overlapping CIDRs, and hybrid routing can break paths.

Scale is the second major risk. A full mesh grows as `O(n^2)`, so the backend must enforce limits on outgoing peers, incoming peers, interval, jitter, and event volume.

Security is also central. The listener changes the host's inbound network posture, so it must be locally enabled, minimal, TCP-only, rate limited, and clearly documented for firewall, security group, and NetworkPolicy setup.

## Rollout

Recommended rollout:

1. Internal Agent listener and advertisement behind hidden config.
2. Backend ingestion of endpoint advertisements without scheduling.
3. Internal same-scope dogfood with small bounded topologies.
4. Private beta with explicit enablement and conservative defaults.
5. UI/API controls for mesh policies and scopes.
6. Gradual expansion of supported environments and address selection logic.

Kill switches should exist at the local Agent, RC, org, policy, scope, and edge levels.

## Success Criteria

The feature is successful if:

- Customers can enable mesh testing without manually defining every peer.
- The backend creates useful low-noise coverage for selected scopes.
- Results clearly identify source Agent, destination Agent, and mesh policy.
- The system avoids runaway test volume in large deployments.
- Customers can override address selection when automatic discovery is insufficient.
- Failures are actionable as network path, firewall, policy, or routing signals.

## Recommendation

Proceed with the design, with one strict framing: Agent-to-Agent mesh should be an opt-in backend-mediated Network Path capability, not an autonomous Agent discovery system.

That framing makes the feature productizable, secure enough to roll out incrementally, and aligned with the Remote Config direction for scheduled Network Path tests.
