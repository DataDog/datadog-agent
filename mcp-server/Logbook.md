# DD-Agent Toolset: Experiment Logbook

**Author**: @Valeri Pliskin
**Period**: 2025-01
**Status**: Phase 1 Complete

## Executive Summary

This gadget explores exposing the Datadog Agent as an LLM-accessible toolset via MCP (Model Context Protocol). While Claude could invoke agent diagnostics directly via kubectl, we introduced an MCP server as a governance layer to provide centralized control, security, audit capabilities, and optimized tool discovery. The experiments validate:

1. **MCP as governance layer** - Centralized component for security, audit, and future rate limiting
2. **Tool organization for LLMs** - Tiered structure and tool-search patterns reduce context window usage
3. **Dynamic agent control** - Proof-of-concept for runtime feature management (pause/resume)
4. **kubectl as transport** - Terminal-based operator workflow using local MCP server

## Experiment 1: MCP as Governance Layer

### Why MCP Instead of Direct kubectl?

While Claude could invoke agent diagnostics directly via kubectl exec, we introduced an MCP server as an architectural component to provide:

1. **Granular Tool Discovery** - Organized 30+ agent tools across 6 tiers (Discovery → Workload → Diagnostics → Specialized → Cluster-Agent → System-Probe). Structured descriptions and layering help LLMs navigate large tool catalogs efficiently.

2. **Security & Audit Posture** - Clear separation between read and write operations. Write operations require explicit opt-in via `DD_ALLOW_WRITE=true`. All tool invocations can be logged centrally for audit and compliance.

3. **Future Capabilities** - Centralized layer enables rate limiting, multi-cluster credential management, policy enforcement, and other governance features without modifying the agent.

### Implementation

**API Approach**: Agent tools are exposed via CLI commands and local HTTP endpoints over Unix Domain Sockets (UDS). The MCP server wraps these APIs with kubectl exec as transport.

**Tool Structure**: 30+ tools organized in tiers (0-5), each with concise descriptions (<200 chars) to minimize context usage. Tools target different agent containers (agent, system-probe, trace-agent, cluster-agent) based on functionality.

### Results ✅

- MCP server successfully acts as governance layer between LLM and agent
- Read/write separation prevents accidental destructive operations
- Tool discovery works efficiently through tiered organization
- Architecture allows future enhancements (rate limiting, audit logging) without agent changes

---

## Experiment 2: Tool Organization & Context Management

### Challenge

With 30+ tools available, MCP servers can consume significant context window space when loading all tool schemas. This becomes problematic when the LLM needs context for actual troubleshooting data.

### Approach

**Tiered Structure**: Organized tools in 6 tiers (0-5) with clear progression:
- Tier 0: Discovery tools (list pods, probe capabilities)
- Tier 1: Workload data (tagger, workload metadata)
- Tier 2: Diagnostics (health checks, config validation)
- Tier 3-5: Specialized tools (trace-agent, cluster-agent, system-probe)

**Tool-to-Search-Tools Pattern**: Experimented with Anthropic's advanced tool use pattern where the LLM can search for relevant tools by keyword instead of loading all schemas upfront. Implemented via `DD_EXPERIMENTAL_TOOL_SEARCH=true` flag.

**Concise Descriptions**: Kept all tool descriptions under 200 characters to minimize schema overhead.

### Results ✅

- Tiered organization helps Claude prioritize - typically starts with Discovery (Tier 0), then progresses to Workload/Diagnostics based on problem domain
- Tool search via keywords works effectively for discovery ("network" finds system-probe tools)
- No context overflow observed with 30+ tools
- Typical troubleshooting session uses 3-5 tools, making the tool-to-search-tools pattern unnecessary for this scale
- MCP native filtering combined with tiered structure was sufficient for context management

---

## Experiment 3: Dynamic Agent Control (Proof of Concept)

### Goal

Demonstrate runtime control of agent features without restart. Traditional approach requires agent restart to enable/disable monitoring modules, which disrupts data collection and requires orchestration tooling.

### Implementation

Implemented `module_control` tool as proof-of-concept for dynamic feature management:

**What it does**: Allows runtime pause/resume of USM (Universal Service Monitoring) and NPM (Network Performance Monitoring) modules via eBPF bypass mechanism.

**What it doesn't do**: This is not full lifecycle control. Modules can only be paused/resumed if already enabled at agent startup. Hot-loading disabled modules is not implemented.

**Agent Changes**:
- Added `/network_tracer/module/control` and `/module/status` HTTP endpoints
- Implemented `PauseUSM()`, `ResumeUSM()`, `PauseNPM()`, `ResumeNPM()` tracer methods
- Made eBPF bypass mechanism configurable

**MCP Tool**: `module_control` exposed as write-only tool (requires `DD_ALLOW_WRITE=true`)

### Results ✅

- Pause/resume works instantly (~1ms) without agent restart
- eBPF probes bypass cleanly - maps remain loaded, no data loss on resume
- Demonstrates viability of runtime feature control for cost optimization and troubleshooting

### Future Enhancements

Full lifecycle control would enable:
- **Dynamic Enable/Disable**: Hot-load USM on nodes where it wasn't enabled at startup
- **Resource Reclamation**: Fully unload eBPF programs to reclaim kernel memory
- **Configuration Reloading**: Apply new monitoring configs without restart

This requires implementing a USM controller with proper eBPF program lifecycle management (load/unload), shared map handling, and state machine transitions. See `PLAN_DYNAMIC_USM.md` for detailed design.

---

## Experiment 4: kubectl as Transport for Terminal Workflow

### User Story

An operator wants to troubleshoot production issues from their laptop terminal using their favorite LLM assistant (Claude Desktop, Cursor, etc.). They should be able to investigate live cluster state without SSH access, without switching to a web UI, and without manually running kubectl commands.

### Implementation

**Transport Layer**: kubectl exec bridges local MCP server to remote agent pods. All tool invocations use:
```bash
kubectl exec --context <ctx> -n <ns> <pod> -c <container> \
  -- curl --unix-socket <socket> http://localhost<endpoint>
```

**Workflow**:
1. Operator sets target cluster: `export DD_KUBE_CONTEXT=prod-us-east`
2. Operator asks Claude: "Why is redis on node-2 timing out?"
3. Claude invokes MCP tools (list_agent_pods, workload_list, network_connections)
4. MCP server translates to kubectl exec commands
5. Claude analyzes results and provides diagnosis

### Results ✅

- Latency acceptable for interactive troubleshooting (<2s per call)
- Multi-cluster support via context switching
- No agent modifications required - uses existing CLI/HTTP APIs
- Terminal-first workflow validated

### Alternative: RC Platform

The local MCP server + kubectl pattern could be replaced with a centralized RC (Remote Configuration) platform-based MCP server. This would provide:
- Centralized credential management across clusters
- Better audit and compliance logging
- Rate limiting and policy enforcement at platform level

**Challenge**: If moving to RC platform, we need to preserve the terminal workflow. Operators who prefer working from terminal/IDE shouldn't be forced to use the Datadog web app. Possible solutions:
- RC platform provides MCP endpoint that terminal tools can connect to
- Hybrid model: local MCP forwards to RC platform for auth/audit, but execution remains edge-based

---

## Key Findings

1. **MCP as governance works** - Centralized layer provides security, audit, and extensibility without blocking the LLM-agent interaction

2. **Tool organization scales** - 30+ tools manageable with tiered structure; tool-to-search-tools pattern unnecessary at this scale

3. **Runtime control is viable** - Pause/resume demonstrates feasibility; full lifecycle control would require additional implementation

4. **Terminal workflow validated** - kubectl as transport provides acceptable latency for interactive troubleshooting

5. **RC platform trade-off** - Centralized MCP via RC platform offers better governance but must preserve terminal-first UX

---

## Future Considerations

### Governance & Scale
- Implement audit logging for all tool invocations
- Add rate limiting per operator/cluster
- Multi-cluster credential management
- Policy enforcement layer (who can invoke which tools on which clusters)

### Agent Capabilities
- Full lifecycle control (dynamic enable/disable, not just pause/resume)
- Hot-loading of monitoring modules
- Per-protocol control (e.g., disable HTTP monitoring but keep Kafka)
- Config reloading without restart

### Architecture Evolution
- **RC Platform Integration**: Replace local MCP + kubectl with centralized platform-based MCP
  - Challenge: Must preserve terminal workflow for operators who don't want web UI
  - Solution: RC platform exposes MCP endpoint that terminal tools can connect to
- **Hybrid Model**: Local MCP delegates to RC platform for auth/audit, but execution remains edge-based

### Tool Expansion
Based on operator feedback:
- Expose more diagnostic endpoints (conntrack, network state, eBPF debugging)
- Add write operations (config changes, check execution, flare generation)
- Expand cluster-agent tools (metamap, clusterchecks)

---

## References

- **Gadget Proposal**: `mcp-server/dd-agent-toolset.md`
- **MCP Server Implementation**: `mcp-server/server.py`
- **Anthropic Advanced Tool Use**: https://www.anthropic.com/engineering/advanced-tool-use
