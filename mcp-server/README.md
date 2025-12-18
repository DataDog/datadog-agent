# DD-Agent MCP Server (Gadget)

MCP Server exposing Datadog Agent diagnostic capabilities as tools for AI assistants.

Designed for:
- **Human operators** using Claude Desktop for ad-hoc troubleshooting
- **Agentic SRE workflows** (e.g., BITS) for autonomous incident response

## Quick Start

### Prerequisites

- Python 3.10+
- kubectl configured with cluster access
- Datadog Agent running in Kubernetes

### Installation

```bash
cd mcp-server
pip install -r requirements.txt
```

### Running

```bash
# With default settings (kind-gadget-dev context)
python server.py

# With custom Kubernetes context
DD_KUBE_CONTEXT=my-cluster DD_NAMESPACE=datadog python server.py

# Enable write operations (use with caution)
DD_ALLOW_WRITE=true python server.py
```

### Claude Desktop Configuration

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "dd-agent": {
      "command": "python",
      "args": ["/path/to/mcp-server/server.py"],
      "env": {
        "DD_KUBE_CONTEXT": "my-production-cluster",
        "DD_NAMESPACE": "datadog-agent"
      }
    }
  }
}
```

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           OPERATOR'S LAPTOP                                 │
│                                                                             │
│  ┌─────────────┐      ┌─────────────────────┐      ┌──────────────────┐    │
│  │             │      │                     │      │                  │    │
│  │   Human     │─────▶│   Claude Desktop    │─────▶│   MCP Server     │    │
│  │  Operator   │      │   (Claude Code)     │      │   (Python)       │    │
│  │             │◀─────│                     │◀─────│                  │    │
│  └─────────────┘      └─────────────────────┘      └────────┬─────────┘    │
│                                                              │              │
│                                                              │ kubectl exec │
│                                                              ▼              │
└──────────────────────────────────────────────────────────────┼──────────────┘
                                                               │
                          ┌────────────────────────────────────┼───────────────┐
                          │             KUBERNETES CLUSTER     │               │
                          │                                    │               │
┌─────────────────────────┼────────────────────────────────────┼───────────────┼─────────────────────────┐
│                         │                                    │               │                         │
│  ┌──────────────────────┴───────────┐    ┌───────────────────┴──────────────┴──────────┐              │
│  │           Node 1                 │    │           Node 2                            │              │
│  │                                  │    │                                             │              │
│  │  ┌────────────────────────────┐  │    │  ┌────────────────────────────┐             │              │
│  │  │     Datadog Agent Pod      │  │    │  │     Datadog Agent Pod      │◀────────────┘              │
│  │  │  ┌──────────────────────┐  │  │    │  │  ┌──────────────────────┐  │                            │
│  │  │  │  agent container     │  │  │    │  │  │  agent container     │  │                            │
│  │  │  │  - agent status      │  │  │    │  │  │  - agent status      │  │                            │
│  │  │  │  - agent check       │  │  │    │  │  │  - agent check       │  │                            │
│  │  │  │  - agent diagnose    │  │  │    │  │  │  - agent diagnose    │  │                            │
│  │  │  └──────────────────────┘  │  │    │  │  └──────────────────────┘  │                            │
│  │  │  ┌──────────────────────┐  │  │    │  │  ┌──────────────────────┐  │                            │
│  │  │  │  system-probe        │  │  │    │  │  │  system-probe        │  │                            │
│  │  │  │  - process_cache     │  │  │    │  │  │  - process_cache     │  │                            │
│  │  │  │  - network_connections│  │  │    │  │  │  - network_connections│  │                            │
│  │  │  │  - ebpf map list/dump│  │  │    │  │  │  - ebpf map list/dump│  │                            │
│  │  │  └──────────────────────┘  │  │    │  │  └──────────────────────┘  │                            │
│  │  │  ┌──────────────────────┐  │  │    │  │  ┌──────────────────────┐  │                            │
│  │  │  │  trace-agent         │  │  │    │  │  │  trace-agent         │  │                            │
│  │  │  └──────────────────────┘  │  │    │  │  └──────────────────────┘  │                            │
│  │  └────────────────────────────┘  │    │  └────────────────────────────┘             │              │
│  │                                  │    │                                             │              │
│  │  ┌────────────────────────────┐  │    │  ┌────────────────────────────┐             │              │
│  │  │   App Pod (nginx)          │  │    │  │   App Pod (redis)          │             │              │
│  │  └────────────────────────────┘  │    │  └────────────────────────────┘             │              │
│  │  ┌────────────────────────────┐  │    │  ┌────────────────────────────┐             │              │
│  │  │   App Pod (postgres)       │  │    │  │   App Pod (api-server)     │             │              │
│  │  └────────────────────────────┘  │    │  └────────────────────────────┘             │              │
│  └──────────────────────────────────┘    └─────────────────────────────────────────────┘              │
│                                                                                                       │
│  ┌────────────────────────────────────────────────────────────────────────────────────────────────┐   │
│  │                              Cluster Agent (Deployment)                                        │   │
│  │  - Centralized cluster-level metadata                                                          │   │
│  │  - Kubernetes API server communication                                                         │   │
│  └────────────────────────────────────────────────────────────────────────────────────────────────┘   │
│                                                                                                       │
└───────────────────────────────────────────────────────────────────────────────────────────────────────┘
```

## Available Tools

All tools are **read-only (R)** by default. Write operations require explicit opt-in.

### Tier 0: Discovery (Use First)

| Tool | Description | Output |
|------|-------------|--------|
| `list_agent_pods` | List all agent pods with status and node | JSON |
| `probe_pod_capabilities` | Check what containers/features are available on a pod | JSON |
| `get_tool_requirements` | Get capability requirements for all tools | JSON |

**Capability Discovery Pattern:**
```
1. list_agent_pods → Get pod names
2. probe_pod_capabilities(pod) → Check what's available
3. get_tool_requirements → See which tools need what
4. Call appropriate tools based on capabilities
```

### Tier 1: Workload Data (Most Useful)

| Tool | Description | Output |
|------|-------------|--------|
| `tagger_list` | All entities with tags from all sources | JSON |
| `workload_list` | Container/pod metadata store | JSON |
| `status_collector` | Check execution stats, errors, timing | JSON |

### Tier 2: Diagnostics

| Tool | Description | Output |
|------|-------------|--------|
| `health` | Component health status | JSON |
| `config_check` | Loaded & resolved check configs | JSON |
| `inventory_host` | Host metadata (CPU, memory, OS) | JSON |
| `check` | Run a specific check | JSON |
| `diagnose` | Run diagnostic suites | Text |

### Tier 3: Specialized

| Tool | Description | Output |
|------|-------------|--------|
| `status` | Full agent status | Text |
| `version` | Agent version info | JSON |
| `config` | Full runtime config | YAML |
| `trace_agent_info` | Trace-agent config & endpoints | JSON |

### Tier 4: Cluster Agent

| Tool | Description | Output |
|------|-------------|--------|
| `cluster_agent_status` | Cluster agent status | Text |
| `metamap` | Pod/service mapping per node | Text |
| `clusterchecks` | Cluster check distribution | Text |

### Tier 5: System-Probe (eBPF Workload Visibility)

These tools require **system-probe** to be running with NPM enabled. They provide deep visibility into node workloads via eBPF.

| Tool | Description | Output |
|------|-------------|--------|
| `process_cache` | Processes tracked by network tracer (PIDs, commands, container IDs) | JSON |
| `network_connections` | Active TCP/UDP connections with process/container info | JSON |
| `ebpf_map_list` | All eBPF maps loaded on the node | Text |
| `ebpf_map_dump` | Dump eBPF map contents with BTF formatting | JSON |

**Note**: These tools execute in the `system-probe` container and communicate via Unix Domain Socket.

### Write Operations (Disabled by Default)

| Tool | Risk | Description |
|------|------|-------------|
| `config_set` | Medium | Set runtime config value |
| `secret_refresh` | Low | Refresh secrets from backend |

Enable with: `DD_ALLOW_WRITE=true`

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `DD_KUBE_CONTEXT` | `kind-gadget-dev` | Kubernetes context to use |
| `DD_NAMESPACE` | `default` | Namespace where agents run |
| `DD_ALLOW_WRITE` | `false` | Enable write operations |

**Note**: The default namespace is `default` for the kind-gadget-dev test cluster. For production, override with `DD_NAMESPACE=datadog-agent`.

## Safety Features

1. **Read-only by default**: Write operations require explicit opt-in
2. **No lifecycle commands**: Cannot stop/start agents
3. **No external data transfer**: Flare command not exposed
4. **Audit logging**: All tool calls logged with parameters
5. **Timeout protection**: Commands timeout after 30s

## Agentic SRE Integration

For autonomous incident response workflows:

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         BITS SRE Platform                                │
│                                                                          │
│  ┌─────────┐    ┌───────────┐    ┌─────────────────────┐                │
│  │ Alert   │───▶│ AI Agent  │───▶│ DD-Agent MCP Server │                │
│  │ (PD/OG) │    │           │    │ - tagger_list       │                │
│  └─────────┘    │           │    │ - workload_list     │                │
│                 │           │◀───│ - health            │                │
│                 │           │    │ - diagnose          │                │
│                 │           │    └─────────────────────┘                │
│                 │           │                                           │
│                 │           │───▶ Datadog API MCP (metrics, monitors)   │
│                 │           │───▶ Confluence MCP (write RCA)            │
│                 │           │───▶ Slack MCP (notify team)               │
│                 └───────────┘                                           │
└──────────────────────────────────────────────────────────────────────────┘
```

Key benefits for agentic workflows:
- **Policy enforcement**: MCP server controls what AI can access
- **Audit trail**: Every tool call logged for compliance
- **Rate limiting**: Prevent runaway queries (future enhancement)
- **Multi-cluster**: Manage credentials centrally (future enhancement)

## Development Setup

```bash
# Local K8s cluster (Lima + Kind)
./setup-k8s-host.sh

# Build and load agent image
dda inv omnibus.docker-build
docker save localhost/datadog-agent:local | limactl shell gadget-k8s-host docker load
limactl shell gadget-k8s-host -- kind load docker-image localhost/datadog-agent:local --name gadget-dev

# Deploy agent
kubectl --context kind-gadget-dev apply -f test-cluster.yaml

# Test MCP server
python server.py
```

## Files

| File | Description |
|------|-------------|
| `server.py` | MCP server implementation |
| `commands.json` | Full command reference (192 CLI commands) |
| `requirements.txt` | Python dependencies |
| `extract_commands.py` | Tool to extract commands from agent source |

## Agent Command Reference

See [commands.json](./commands.json) for the full list of 192 CLI commands across all agent binaries, including:
- Command metadata with source file references
- MCP tool tier recommendations for workload troubleshooting
- Subcommand trees with flags and options
- Commands to avoid (mutating, lifecycle)
- HTTP endpoints with authentication details
