# Network Path Agent-to-Agent Mesh

Date: 2026-07-06

## Summary

The Agent-to-Agent mesh idea is valid and doable if it is framed as a backend-mediated Network Path feature:

```text
opt-in Agent probe listener
+ Agent-advertised endpoint candidates
+ backend-computed bounded same-scope mesh
+ RC-delivered scheduled Network Path tests
+ first-class mesh identity in results
```

The idea is not viable as an Agent-local peer discovery or global full-mesh system. Agents should not gossip, scan, or independently decide the mesh topology. That would create major problems with scale, tenancy, security boundaries, NAT, Kubernetes networking, overlapping private CIDRs, and hybrid networks.

The recommended model is:

1. Each destination Agent explicitly opts in locally and starts a minimal TCP probe listener.
2. Each destination Agent advertises listener readiness and candidate reachable addresses.
3. The backend builds the eligible peer set from those advertisements and existing host/container/cloud metadata.
4. The backend computes a bounded topology within conservative reachability scopes.
5. The backend sends source Agents scheduled Network Path tests through the `NETWORK_PATH` Remote Config product.
6. Source Agents execute regular Network Path tests to backend-selected peer endpoint addresses.
7. Results carry durable Agent-to-Agent mesh identity, not only free-form tags.

## Context

Network Path already supports scheduled host-level path tests where each Agent runs traceroute to a configured destination. The public setup flow uses `conf.d/network_path.d` or Autodiscovery annotations. Dynamic Network Path tests are driven by observed traffic from Cloud Network Monitoring.

The internal RFC for scheduled/dynamic Network Path tests via Remote Config changes the control plane:

- `NETWORK_PATH` is the intended RC product for backend-created Network Path configs.
- RC becomes the source of truth for scheduled and dynamic Network Path test configs.
- Scheduled tests are planned as an Autodiscovery streaming provider that translates RC payloads into `network_path` integration instances.
- Dynamic tests are a follow-up under the same product.
- Agent subscription is gated by `remote_configuration.enabled` and `network_path.remote_config.enabled`.

Agent-to-Agent mesh should build on that direction instead of introducing a separate Agent-side scheduler or separate RC product.

## Current Code Findings

### Scheduled Network Path

Relevant files:

- `pkg/collector/corechecks/networkpath/config.go`
- `pkg/collector/corechecks/networkpath/networkpath.go`
- `pkg/networkpath/traceroute/runner/runner.go`
- `pkg/networkpath/payload/pathevent.go`

The scheduled Network Path check already consumes a destination hostname and port:

```yaml
instances:
  - hostname: example.com
    port: 443
    protocol: TCP
```

The check builds a traceroute config from the instance:

- destination hostname
- destination port
- protocol
- TCP method
- max TTL
- timeout
- traceroute query count
- E2E query count

It then runs the traceroute component and emits a `network-path` Event Platform payload.

This maps well to Agent-to-Agent mesh: a backend-generated mesh edge can be represented as a scheduled Network Path test whose destination is the selected peer Agent probe endpoint.

### Dynamic Network Path

Relevant files:

- `comp/networkpath/npcollector/def/component.go`
- `comp/networkpath/npcollector/model/connection.go`
- `comp/networkpath/npcollector/impl/npcollector.go`
- `pkg/network/sender/sender_linux.go`
- `comp/netflow/flowaggregator/aggregator.go`

Dynamic Network Path currently schedules tests from observed network connections. The CNM sender converts live connections into `NetworkPathConnection` objects and the Network Path collector schedules traceroutes to the observed destination IP or DNS name.

This is useful prior art, but it does not solve Agent-to-Agent peer discovery. Dynamic tests answer "what remote services is this Agent already seeing?" They do not answer "which Datadog Agents are eligible probe targets?"

### Traceroute Execution Boundary

Relevant files:

- `cmd/system-probe/modules/traceroute.go`
- `comp/networkpath/traceroute/impl-remote/traceroute.go`
- `comp/networkpath/traceroute/impl-local/traceroute.go`

The source Agent can use the existing Network Path traceroute component. Depending on platform and configuration, this may call system-probe or run locally.

The destination Agent should not expose system-probe traceroute APIs. The destination-side requirement is only to expose a minimal endpoint that source Agents can trace toward.

### Payload Data Model

Relevant file:

- `pkg/networkpath/payload/pathevent.go`

Network Path payloads already include concepts that are useful for mesh:

- `test_config_id`
- `test_run_type`
- `origin`
- `source_product`
- `collector_type`
- source and destination identity fields
- traceroute and E2E probe data
- tags

Agent-to-Agent mesh should not rely only on tags for identity. Tags are useful for filtering, but they are too weak as the sole contract for UI behavior, backend suppression, result grouping, billing/accounting, lifecycle management, and debugging.

### Inventory and Metadata

Relevant files:

- `comp/metadata/inventoryagent/def/component.go`
- `comp/metadata/inventoryagent/impl/inventoryagent.go`
- `comp/metadata/inventoryhost/impl/inventoryhost.go`
- `comp/metadata/internal/util/inventory_payload.go`
- `comp/metadata/host/impl/utils/meta.go`

Inventory Agent exposes a `Set(name string, value interface{})` extension point for Agent metadata. It already reports Agent version, feature flags, infrastructure mode, hostname source, and similar capability data.

Inventory Host reports host-level network metadata such as primary IP, IPv6 address, MAC address, and serialized interface information. Host metadata also includes host aliases, cloud identity, EC2 hostname, instance ID, cluster name, and canonical cloud resource ID where available.

This gives a plausible mechanism for Agent-to-Agent endpoint advertisement:

- listener enabled
- listener ready
- protocol
- port
- address candidates
- address provenance
- version/capability information
- scope metadata

Inventory refresh is not instantaneous. `Set` triggers a refresh flag, but submission still respects inventory minimum intervals. That is acceptable for endpoint eligibility and capability advertisement, but not for fast liveness. If fast liveness is required later, add a lightweight heartbeat or status signal in addition to inventory.

### Kubernetes Metadata

Relevant files:

- `comp/core/workloadmeta/def/types.go`
- `comp/core/workloadmeta/collectors/util/kubelet.go`
- `comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers/pod.go`

Workload metadata already tracks Kubernetes pod fields that matter for reachability:

- pod IP
- node name
- host IP
- host network mode
- namespace
- labels and annotations
- readiness and phase

These fields can help backend address selection and scope decisions, but they are not enough to prove reachability by themselves. Kubernetes CNI, NetworkPolicy, hostNetwork, DaemonSet deployment mode, service mesh, and cluster boundaries all affect whether a source Agent can reach a destination Agent listener.

### Remote Agent Registry Is Not the Right Mechanism

Relevant files:

- `comp/core/remoteagent/helper/serverhelper.go`
- `comp/core/remoteagent/impl-systemprobe/remoteagent.go`
- `comp/core/remoteagent/impl-trace/remoteagent.go`
- `comp/core/remoteagent/impl-process/remoteagent.go`
- `pkg/config/setup/common_settings.go`

The existing `remote_agent.registry` mechanism is local Agent IPC. It registers local sub-agents, such as system-probe, trace-agent, process-agent, and security-agent, back to the same Core Agent. The helper creates listeners on `127.0.0.1` by default.

It should not be reused as a cross-host Agent-to-Agent probe target or peer discovery layer.

## Recommended Architecture

### High-Level Flow

```text
Destination Agent B:
  1. Local config enables Agent mesh listener.
  2. Core Agent starts a minimal TCP probe listener.
  3. Agent B auto-discovers candidate local endpoint addresses.
  4. Agent B advertises readiness and endpoint candidates to Datadog.

Backend:
  5. Ingests Agent B as an eligible mesh endpoint.
  6. Builds conservative same-scope peer groups.
  7. Computes bounded mesh edges.
  8. Chooses one destination endpoint address per edge.
  9. Emits scheduled Network Path test configs through NETWORK_PATH RC.

Source Agent A:
  10. Receives assigned tests through RC.
  11. Runs regular Network Path traceroute/E2E probes to Agent B's selected endpoint.
  12. Emits Network Path results with first-class Agent-to-Agent mesh identity.
```

### Agent Responsibilities

The Agent should be responsible for:

- honoring local opt-in gates
- running a minimal TCP listener when enabled
- auto-discovering candidate endpoint addresses
- accepting optional configured address/interface overrides
- advertising endpoint candidates and readiness
- subscribing to `NETWORK_PATH` RC when enabled
- executing assigned scheduled Network Path tests
- emitting results with mesh identity

The Agent should not be responsible for:

- discovering all peer Agents
- computing mesh topology
- deciding cross-host reachability globally
- enforcing organization-wide topology policy
- opening the inbound listener solely because of RC
- exposing Agent IPC, remote-agent registry, or system-probe APIs to peers

### Backend Responsibilities

The backend should be responsible for:

- ingesting endpoint advertisements
- determining peer eligibility
- grouping Agents into conservative reachability scopes
- choosing address candidates per source/destination edge
- computing bounded topology
- generating and updating `NETWORK_PATH` RC configs
- suppressing consistently failing edges
- applying global and per-scope limits
- preserving edge lifecycle and result identity
- powering UI/API workflows for mesh policy configuration

## Discovery and Addressing

Each Agent should advertise endpoint candidates. It should not advertise a single canonical address unless explicitly configured by the user.

Example advertised shape:

```json
{
  "network_path_agent_mesh": {
    "enabled": true,
    "listener": {
      "ready": true,
      "protocol": "tcp",
      "port": 5107,
      "addresses": [
        {
          "address": "10.12.3.45",
          "type": "private_ip",
          "source": "interface",
          "scope": "host"
        },
        {
          "address": "172.18.4.22",
          "type": "pod_ip",
          "source": "kubernetes",
          "scope": "cluster"
        },
        {
          "address": "agent-12.internal.example.com",
          "type": "dns",
          "source": "configured",
          "scope": "custom"
        }
      ]
    }
  }
}
```

Manual configuration should be an override, not a requirement. Most environments should get candidate addresses automatically from host interfaces, host inventory, Kubernetes metadata, cloud metadata, or configured hostname data.

However, discovering candidates is easier than proving reachability. The backend should treat auto-discovered addresses as candidates and choose conservatively.

Expected reliability by environment:

| Environment | Auto address selection likelihood | Notes |
|---|---:|---|
| Same VPC/private network VMs | High | Private interface IPs often work if firewall/security groups allow the probe port. |
| Kubernetes same cluster | Medium-high | Depends on choosing Pod IP vs Node/Host IP and on CNI/NetworkPolicy. |
| Kubernetes across clusters | Low-medium | Requires routing, peering, policy, and non-overlapping assumptions. |
| ECS on EC2 | Medium | Depends on network mode and security groups. |
| Fargate/serverless-style environments | Low-medium | Inbound listener exposure may be constrained or undesirable. |
| Hybrid/on-prem/multi-cloud | Low by default | Multi-NIC, NAT, firewalls, overlapping CIDRs, and DNS make blind selection risky. |

## Remote Config Model

Agent-to-Agent mesh assignments should be delivered as backend-generated scheduled Network Path tests through the same `NETWORK_PATH` RC product planned by the scheduled/dynamic tests RFC.

Example conceptual RC payload:

```json
{
  "test_config_id": "mesh-policy-abc-edge-agent-a-agent-b",
  "type": "scheduled",
  "tests": [
    {
      "hostname": "10.12.3.45",
      "port": 5107,
      "protocol": "TCP",
      "interval_sec": 60,
      "timeout_ms": 5000,
      "max_ttl": 30,
      "tcp_method": "SYN",
      "traceroute_queries": 1,
      "e2e_queries": 3,
      "source_service": "datadog-agent",
      "destination_service": "datadog-agent",
      "tags": [
        "network_path_agent_mesh:true",
        "mesh_policy:abc",
        "peer_agent:agent-b"
      ]
    }
  ]
}
```

The exact RFC schema can evolve, but the important design decision is that the source Agent receives only assigned edges. It does not receive the full peer inventory or mesh policy and compute the topology locally.

## Result Identity

Mesh results should have first-class identity in the Network Path payload or associated metadata. Tags alone are not enough.

Recommended semantic model:

```text
test_run_type: scheduled
origin: agent_to_agent_mesh
source_product: network_path
test_config_id: <backend mesh edge or policy id>
mesh:
  policy_id: ...
  edge_id: ...
  source_agent_id: ...
  source_agent_hostname: ...
  destination_agent_id: ...
  destination_agent_hostname: ...
  destination_endpoint_address: ...
  destination_endpoint_type: ...
```

Keeping `test_run_type=scheduled` is appropriate because execution is scheduled. The mesh-specific distinction should capture why the test exists and which Agent endpoint it targets.

## Security Model

The destination listener changes the host's inbound network posture, so it must be explicitly enabled by local Agent configuration. RC should not be able to open a new inbound listener on a host that was not prepared for it.

Recommended gates:

```yaml
remote_configuration.enabled: true
network_path.remote_config.enabled: true

network_path.agent_mesh.enabled: true
network_path.agent_mesh.listener.enabled: true
```

The listener should be:

- TCP-only for V1
- minimal behavior
- no control plane API
- no Agent IPC exposure
- no system-probe exposure
- no remote-agent registry exposure
- rate limited
- bounded by connection/read/write timeouts
- documented with firewall, security group, and NetworkPolicy guidance

The source Agent still uses existing Network Path traceroute execution. The destination Agent only provides a safe endpoint to trace toward.

## Topology Policy

The backend should fail closed to same-scope meshes and bounded topology.

Recommended default constraints:

```text
same org
same site
same cloud account / VPC / subnet when known
same Kubernetes cluster when known
same configured mesh scope tag when provided
compatible address family and endpoint type
bounded outgoing peers per Agent
bounded incoming peers per Agent
backend-controlled interval and jitter
full mesh only for small scopes or explicit advanced policy
```

Full mesh should not be the default. A full directed mesh grows as `O(n^2)`. With 1,000 Agents in a scope, bidirectional testing can approach 1,000,000 directed edges, which is too much RC churn, traceroute load, event volume, backend ingestion, and UI noise.

Useful bounded topologies include:

- ring
- random regular graph
- hub-and-spoke with rotating representatives
- per-subnet or per-AZ representatives
- sampled edges with periodic rotation

The exact algorithm should be backend-owned and can evolve without changing the Agent contract.

## Agent-Side Implementation Sketch

Likely Agent-side pieces:

1. Add config keys for Agent mesh opt-in and listener settings.
2. Add a Core Agent component for the minimal TCP probe listener.
3. Add endpoint candidate discovery:
   - configured address/interface override
   - host interface candidates
   - host inventory candidates
   - Kubernetes pod/host candidates where available
   - cloud private DNS/IP candidates where available
4. Publish listener readiness and candidates through inventory metadata.
5. Add or extend Network Path RC provider support based on the scheduled/dynamic RC RFC.
6. Ensure scheduled tests can carry internal mesh metadata, including `test_config_id`.
7. Extend Network Path payload identity for mesh origin/edge metadata.
8. Add status/flare visibility for listener state and advertised candidates.
9. Add telemetry for listener readiness, accepted connections, rejected connections, and assigned test execution.

## Backend-Side Implementation Sketch

Likely backend pieces:

1. Ingest Agent mesh endpoint advertisements.
2. Store endpoint candidate inventory with freshness and readiness.
3. Join with host, cloud, Kubernetes, tags, and Agent metadata.
4. Compute eligible scopes.
5. Select endpoint address per source/destination edge.
6. Generate bounded topology.
7. Emit RC scheduled test configs through `NETWORK_PATH`.
8. Track edge health and suppress noisy/bad edges.
9. Expose UI/API policy controls.
10. Group and render mesh-specific Network Path results.

## Rollout Strategy

Recommended rollout:

1. Agent hidden/experimental local opt-in.
2. Listener-only advertisement with no backend scheduling.
3. Backend ingestion and internal validation of endpoint candidates.
4. Limited same-scope scheduled mesh assignments via RC.
5. Internal dogfood in controlled environments.
6. Private beta with explicit enablement and tight topology bounds.
7. Gradual expansion of address selection, environment support, and UI policy controls.

Kill switches should exist at multiple levels:

- local listener opt-out
- RC subscription/config opt-out
- backend per-org disable
- backend per-policy disable
- backend per-scope topology limit
- backend edge suppression

## Key Risks

### Address Reachability

The Agent can discover candidate addresses, but only the backend can make a reasonable scoped decision about which source Agents may reach which destination endpoint. Even then, failures are expected in locked-down networks.

Mitigation:

- fail closed to conservative scopes
- prefer explicit configured addresses where present
- prefer private same-scope addresses
- record address provenance
- suppress repeatedly failing edges
- provide manual overrides

### Inbound Listener Exposure

Opening a listener can conflict with customer security posture.

Mitigation:

- local opt-in only
- minimal TCP-only listener
- no control API
- timeouts and rate limits
- documentation for firewall/security group/NetworkPolicy
- status and inventory visibility

### Scale

Full mesh does not scale.

Mitigation:

- bounded topology by default
- backend-controlled intervals and jitter
- per-Agent incoming/outgoing caps
- full mesh only for small or explicit scopes

### Metadata Freshness

Inventory is suitable for eligibility but not fast liveness.

Mitigation:

- treat inventory advertisement as capability/readiness, not perfect health
- add a lightweight heartbeat later if required
- use result feedback to suppress stale edges

### Data Model Ambiguity

If mesh identity is only encoded as tags, downstream systems will have fragile contracts.

Mitigation:

- add first-class mesh origin/edge identity
- preserve `test_config_id`
- keep tags as supplemental filtering, not the primary contract

## Decisions Reached

1. The mesh should be backend-mediated, not Agent-discovered.
2. The destination should expose a dedicated minimal TCP probe listener.
3. Existing Agent IPC, remote-agent registry, and system-probe APIs should not be used as peer targets.
4. Mesh assignments should be delivered as scheduled Network Path tests through `NETWORK_PATH` RC.
5. Results should carry first-class mesh identity, not only tags.
6. Opening the listener requires explicit local opt-in.
7. Default topology should be same-scope and fail closed.
8. Default topology should be bounded, not full mesh.
9. Agents should advertise multiple endpoint candidates with provenance.
10. Manual advertised addresses should be optional overrides, not mandatory setup.

## Conclusion

Agent-to-Agent Network Path mesh is a sound feature direction if the Agent remains a probe executor and endpoint advertiser, while the backend owns discovery, topology, address selection, and scheduling.

The Agent-side work is feasible because Network Path already has scheduled test execution, traceroute integration, payload emission, metadata inventory, and RC integration patterns. The backend-side work is the larger product dependency because it must turn endpoint advertisements into safe, bounded, scoped RC assignments and durable mesh result semantics.
