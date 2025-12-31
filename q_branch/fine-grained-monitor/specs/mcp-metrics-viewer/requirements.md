# MCP Metrics Viewer

## User Story

As an LLM agent (Claude Code, AI SRE agent, or future automation) assisting
engineers with container diagnostics, I need to programmatically discover
metrics, find relevant containers on specific cluster nodes, and run analytical
studies so that I can provide actionable insights without requiring the engineer
to manually navigate visual interfaces.

The MCP server runs in-cluster and understands the node-local nature of the
fine-grained-monitor DaemonSet, routing requests to the correct pod based on
which node's data the agent needs to query.

## Requirements

### REQ-MCP-001: Discover Available Metrics

WHEN an agent requests metric discovery
THE SYSTEM SHALL return all available metric names

WHEN analytical studies are available
THE SYSTEM SHALL list each study with a description of what patterns it detects

**Rationale:** Agents need to understand what data is available before they can
help engineers investigate issues. Without discovery, agents would guess at
metric names or require engineers to manually specify them.

---

### REQ-MCP-002: Find Containers by Criteria

WHEN an agent searches for containers on a specific node
THE SYSTEM SHALL return containers matching the search criteria from that node

WHEN filtering by namespace
THE SYSTEM SHALL return only containers in that Kubernetes namespace

WHEN filtering by pod name
THE SYSTEM SHALL return only containers whose pod name starts with the given prefix

WHEN filtering by container name
THE SYSTEM SHALL return only containers whose container name starts with the given prefix

WHEN results exceed the requested limit
THE SYSTEM SHALL return only the top N containers sorted by the specified criteria

**Rationale:** Engineers often start investigations with partial information
("something in the payments namespace is using too much CPU"). Agents need
flexible search to narrow down which specific containers to analyze. The node
parameter ensures the agent queries the correct DaemonSet pod that has data for
the containers of interest.

---

### REQ-MCP-003: Sort Containers by Recency

WHEN an agent requests container search results
THE SYSTEM SHALL sort containers by most recently seen first

**Rationale:** Recently active containers are most relevant for ongoing
investigations. Value-based sorting (by CPU usage, memory, etc.) requires
expensive computation across all containers; recency sorting is fast and
provides a reasonable default for discovery.

---

### REQ-MCP-004: Analyze Container Behavior

WHEN an agent requests analysis of a container on a specific node
THE SYSTEM SHALL run the specified study on that container's metrics

WHEN analyzing a single metric
THE SYSTEM SHALL return findings specific to that metric

WHEN analyzing by metric prefix
THE SYSTEM SHALL return findings for all metrics matching that prefix

WHEN the study completes
THE SYSTEM SHALL return detected patterns with timestamps and magnitudes

**Rationale:** The core value of the metrics viewer is its analytical studies
(periodicity detection, changepoint detection). Agents need programmatic access
to these insights to help engineers understand container behavior patterns. The
node parameter ensures the request reaches the DaemonSet pod that has the
container's data.

---

### REQ-MCP-005: Identify Behavioral Trends

WHEN returning analysis results
THE SYSTEM SHALL classify the overall trend as increasing, decreasing, stable,
or volatile

WHEN a trend is identified
THE SYSTEM SHALL include the trend classification in the summary

**Rationale:** Trend direction is a fundamental diagnostic signal. An increasing
memory trend suggests a leak; a volatile CPU pattern suggests contention. Agents
need this classification to provide useful guidance.

---

### REQ-MCP-006: Operate via Standard Protocol

WHEN an agent connects via HTTP or SSE
THE SYSTEM SHALL communicate using the Model Context Protocol (MCP)

WHEN the MCP server starts
THE SYSTEM SHALL register available tools with their schemas

WHEN a tool is invoked
THE SYSTEM SHALL validate inputs against the tool schema before execution

**Rationale:** MCP is the standard protocol for LLM tool integration. HTTP/SSE
transport enables both local development (via port-forward to the in-cluster
service) and direct access from AI SRE agents running in backend services.

---

### REQ-MCP-007: Discover Cluster Nodes

WHEN an agent requests node discovery
THE SYSTEM SHALL return all nodes running fine-grained-monitor pods

WHEN listing nodes
THE SYSTEM SHALL include node name, pod IP, and availability status

**Rationale:** Data is node-local—each DaemonSet pod only has metrics for
containers on its own node. Agents must understand cluster topology to target
queries to the correct node. This is the first tool agents should call to
understand what nodes are available.

---

### REQ-MCP-008: Route Requests by Node

WHEN an agent specifies a node for container or analysis queries
THE SYSTEM SHALL route the request to that node's DaemonSet pod

WHEN no node is specified for node-scoped queries
THE SYSTEM SHALL return an error indicating the node parameter is required

**Rationale:** Each DaemonSet pod only has its own node's data. Forcing explicit
node targeting prevents confusion and ensures agents understand the data
topology. The MCP server handles the complexity of pod discovery and routing
internally.

---
