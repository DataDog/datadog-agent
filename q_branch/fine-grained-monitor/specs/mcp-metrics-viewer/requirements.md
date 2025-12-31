# MCP Metrics Viewer

## User Story

As an LLM agent assisting engineers with container diagnostics, I need to
programmatically discover metrics, find relevant containers, and run analytical
studies so that I can provide actionable insights without requiring the engineer
to manually navigate visual interfaces.

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

WHEN an agent searches for containers
THE SYSTEM SHALL return containers matching the search criteria

WHEN filtering by namespace
THE SYSTEM SHALL return only containers in that Kubernetes namespace

WHEN filtering by text search
THE SYSTEM SHALL match against pod name and container name

WHEN results exceed the requested limit
THE SYSTEM SHALL return only the top N containers sorted by the specified criteria

**Rationale:** Engineers often start investigations with partial information
("something in the payments namespace is using too much CPU"). Agents need
flexible search to narrow down which specific containers to analyze.

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

WHEN an agent requests analysis of a container
THE SYSTEM SHALL run the specified study on that container's metrics

WHEN analyzing a single metric
THE SYSTEM SHALL return findings specific to that metric

WHEN analyzing by metric prefix
THE SYSTEM SHALL return findings for all metrics matching that prefix

WHEN the study completes
THE SYSTEM SHALL return detected patterns with timestamps and magnitudes

**Rationale:** The core value of the metrics viewer is its analytical studies
(periodicity detection, changepoint detection). Agents need programmatic access
to these insights to help engineers understand container behavior patterns.

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

WHEN an agent connects via stdio
THE SYSTEM SHALL communicate using the Model Context Protocol (MCP)

WHEN the MCP server starts
THE SYSTEM SHALL register available tools with their schemas

WHEN a tool is invoked
THE SYSTEM SHALL validate inputs against the tool schema before execution

**Rationale:** MCP is the standard protocol for LLM tool integration. Using stdio
transport enables local execution without network configuration, matching the
existing kubectl port-forward workflow for accessing the metrics viewer API.

---
