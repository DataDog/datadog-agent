#!/usr/bin/env python3
"""
DD-Agent MCP Server (Gadget)

MCP Server exposing Datadog Agent diagnostic capabilities as tools.
Designed for agentic SRE workflows with safety guardrails.

Usage:
    # Run directly
    python server.py

    # Or with uvx
    uvx mcp run server.py
"""

import asyncio
import json
import logging
import os
import subprocess
from dataclasses import dataclass
from enum import Enum
from typing import Optional

from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import TextContent, Tool

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("dd-agent-mcp")

# -----------------------------------------------------------------------------
# Configuration
# -----------------------------------------------------------------------------

@dataclass
class Config:
    """MCP Server configuration."""
    kube_context: str = "kind-gadget-dev"
    agent_label: str = "app.kubernetes.io/instance=datadog-agent,agent.datadoghq.com/component=agent"
    agent_container: str = "agent"
    system_probe_container: str = "system-probe"
    cluster_agent_label: str = "agent.datadoghq.com/component=cluster-agent"
    cluster_agent_container: str = "cluster-agent"
    namespace: str = "default"
    kubectl_timeout: int = 30
    # System-probe socket path inside container
    sysprobe_socket: str = "/var/run/sysprobe/sysprobe.sock"
    # Safety: only expose read-only tools by default
    allow_write_operations: bool = False


class ToolType(Enum):
    """Tool execution type."""
    CLI = "cli"
    HTTP = "http"


# -----------------------------------------------------------------------------
# Kubectl Executor
# -----------------------------------------------------------------------------

class KubectlExecutor:
    """Executes commands on Kubernetes pods via kubectl exec."""

    def __init__(self, config: Config):
        self.config = config

    async def get_agent_pods(self) -> list[dict]:
        """List available agent pods."""
        cmd = [
            "kubectl", "--context", self.config.kube_context,
            "get", "pods",
            "-l", self.config.agent_label,
            "-n", self.config.namespace,
            "-o", "json"
        ]
        result = await self._run_kubectl(cmd)
        if result["success"]:
            data = json.loads(result["stdout"])
            pods = []
            for item in data.get("items", []):
                pods.append({
                    "name": item["metadata"]["name"],
                    "node": item["spec"].get("nodeName", "unknown"),
                    "status": item["status"]["phase"],
                    "ready": all(
                        c.get("ready", False)
                        for c in item["status"].get("containerStatuses", [])
                    )
                })
            return pods
        return []

    async def get_cluster_agent_pod(self) -> Optional[str]:
        """Get the cluster agent pod name."""
        cmd = [
            "kubectl", "--context", self.config.kube_context,
            "get", "pods",
            "-l", self.config.cluster_agent_label,
            "-n", self.config.namespace,
            "-o", "jsonpath={.items[0].metadata.name}"
        ]
        result = await self._run_kubectl(cmd)
        if result["success"] and result["stdout"]:
            return result["stdout"].strip()
        return None

    async def exec_cli(
        self,
        pod: str,
        command: list[str],
        container: Optional[str] = None
    ) -> dict:
        """Execute a CLI command on a pod."""
        container = container or self.config.agent_container
        cmd = [
            "kubectl", "--context", self.config.kube_context,
            "exec", pod,
            "-n", self.config.namespace,
            "-c", container,
            "--"
        ] + command
        return await self._run_kubectl(cmd)

    async def exec_http(
        self,
        pod: str,
        endpoint: str,
        method: str = "GET",
        auth_required: bool = True,
        container: Optional[str] = None,
        use_uds: bool = False,
        socket_path: Optional[str] = None
    ) -> dict:
        """Execute an HTTP request inside a pod."""
        container = container or self.config.agent_container

        if use_uds and socket_path:
            # UDS request (e.g., trace-agent)
            curl_cmd = f"curl -s --unix-socket {socket_path} http://localhost{endpoint}"
        elif auth_required:
            # HTTPS with Bearer token auth (use double quotes for variable expansion)
            curl_cmd = (
                f'TOKEN=$(cat /etc/datadog-agent/auth_token) && '
                f'curl -sk -H "Authorization: Bearer $TOKEN" '
                f'https://localhost:5001{endpoint}'
            )
        else:
            # Plain HTTP
            curl_cmd = f"curl -s http://localhost:8126{endpoint}"

        cmd = [
            "kubectl", "--context", self.config.kube_context,
            "exec", pod,
            "-n", self.config.namespace,
            "-c", container,
            "--", "sh", "-c", curl_cmd
        ]
        return await self._run_kubectl(cmd)

    async def exec_http_post(
        self,
        pod: str,
        endpoint: str,
        body: str,
        container: Optional[str] = None,
        use_uds: bool = False,
        socket_path: Optional[str] = None
    ) -> dict:
        """Execute an HTTP POST request with JSON body inside a pod."""
        container = container or self.config.agent_container

        # Escape body for shell
        escaped_body = body.replace("'", "'\\''")

        if use_uds and socket_path:
            curl_cmd = (
                f"curl -s --unix-socket {socket_path} "
                f"-X POST -H 'Content-Type: application/json' "
                f"-d '{escaped_body}' http://localhost{endpoint}"
            )
        else:
            curl_cmd = (
                f"curl -s -X POST -H 'Content-Type: application/json' "
                f"-d '{escaped_body}' http://localhost{endpoint}"
            )

        cmd = [
            "kubectl", "--context", self.config.kube_context,
            "exec", pod,
            "-n", self.config.namespace,
            "-c", container,
            "--", "sh", "-c", curl_cmd
        ]
        return await self._run_kubectl(cmd)

    async def _run_kubectl(self, cmd: list[str]) -> dict:
        """Run a kubectl command."""
        logger.info(f"Executing: {' '.join(cmd)}")
        try:
            proc = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )
            stdout, stderr = await asyncio.wait_for(
                proc.communicate(),
                timeout=self.config.kubectl_timeout
            )
            return {
                "success": proc.returncode == 0,
                "stdout": stdout.decode("utf-8"),
                "stderr": stderr.decode("utf-8"),
                "returncode": proc.returncode
            }
        except asyncio.TimeoutError:
            return {
                "success": False,
                "stdout": "",
                "stderr": f"Command timed out after {self.config.kubectl_timeout}s",
                "returncode": -1
            }
        except Exception as e:
            return {
                "success": False,
                "stdout": "",
                "stderr": str(e),
                "returncode": -1
            }


# -----------------------------------------------------------------------------
# MCP Server
# -----------------------------------------------------------------------------

# Initialize server and executor
config = Config(
    kube_context=os.getenv("DD_KUBE_CONTEXT", "kind-gadget-dev"),
    namespace=os.getenv("DD_NAMESPACE", "default"),
    allow_write_operations=os.getenv("DD_ALLOW_WRITE", "false").lower() == "true"
)
executor = KubectlExecutor(config)
server = Server("dd-agent-mcp")


# -----------------------------------------------------------------------------
# Capability Discovery
# -----------------------------------------------------------------------------

class Capability:
    """Capabilities that can be detected on agent pods."""
    AGENT = "agent"                    # Core agent container
    SYSTEM_PROBE = "system_probe"      # system-probe container (eBPF)
    TRACE_AGENT = "trace_agent"        # trace-agent (APM)
    PROCESS_AGENT = "process_agent"    # process-agent
    CLUSTER_AGENT = "cluster_agent"    # Cluster Agent (separate pod)


# Maps tools to their required capabilities
# Tools not listed here have no special requirements (just need agent container)
TOOL_REQUIREMENTS: dict[str, list[str]] = {
    # System-probe tools (require eBPF container)
    "process_cache": [Capability.SYSTEM_PROBE],
    "network_connections": [Capability.SYSTEM_PROBE],
    "ebpf_map_list": [Capability.SYSTEM_PROBE],
    "ebpf_map_dump": [Capability.SYSTEM_PROBE],
    "module_control": [Capability.SYSTEM_PROBE],
    "module_status": [Capability.SYSTEM_PROBE],

    # Trace agent tools
    "trace_agent_info": [Capability.TRACE_AGENT],

    # Cluster agent tools (require cluster agent pod)
    "cluster_agent_status": [Capability.CLUSTER_AGENT],
    "metamap": [Capability.CLUSTER_AGENT],
    "clusterchecks": [Capability.CLUSTER_AGENT],
}


async def probe_pod_capabilities(pod: str) -> dict[str, bool]:
    """
    Probe a specific pod to discover available capabilities.
    Checks which containers are running and accessible.
    """
    capabilities = {
        Capability.AGENT: False,
        Capability.SYSTEM_PROBE: False,
        Capability.TRACE_AGENT: False,
        Capability.PROCESS_AGENT: False,
    }

    # Map capability to container name
    container_map = {
        Capability.AGENT: config.agent_container,
        Capability.SYSTEM_PROBE: config.system_probe_container,
        Capability.TRACE_AGENT: "trace-agent",
        Capability.PROCESS_AGENT: "process-agent",
    }

    # Probe each container
    for cap, container in container_map.items():
        result = await executor.exec_cli(pod, ["true"], container=container)
        capabilities[cap] = result["success"]

    return capabilities


async def check_cluster_agent_available() -> bool:
    """Check if cluster agent is deployed and accessible."""
    ca_pod = await executor.get_cluster_agent_pod()
    return ca_pod is not None


def get_missing_capabilities(tool_name: str, pod_capabilities: dict[str, bool]) -> list[str]:
    """Get list of missing capabilities for a tool."""
    required = TOOL_REQUIREMENTS.get(tool_name, [])
    missing = [cap for cap in required if not pod_capabilities.get(cap, False)]
    return missing


def format_capability_error(tool_name: str, missing: list[str]) -> str:
    """Format a helpful error message for missing capabilities."""
    cap_hints = {
        Capability.SYSTEM_PROBE: (
            "system-probe container is not running. "
            "Enable it with: spec.features.networkMonitoring.enabled=true in DatadogAgent CRD"
        ),
        Capability.TRACE_AGENT: (
            "trace-agent container is not running. "
            "Enable APM with: spec.features.apm.enabled=true in DatadogAgent CRD"
        ),
        Capability.CLUSTER_AGENT: (
            "Cluster Agent is not deployed or not accessible. "
            "Enable it with: spec.features.clusterAgent.enabled=true in DatadogAgent CRD"
        ),
    }

    hints = [cap_hints.get(cap, f"{cap} is not available") for cap in missing]
    return (
        f"Tool '{tool_name}' requires capabilities that are not available:\n"
        + "\n".join(f"  - {hint}" for hint in hints)
        + "\n\nUse 'probe_pod_capabilities' to check what's available on each pod."
    )


# -----------------------------------------------------------------------------
# Tool Definitions
# -----------------------------------------------------------------------------

TOOLS = {
    # Tier 0: Discovery (use these first)
    "list_agent_pods": {
        "description": (
            "List all Datadog Agent pods running in the Kubernetes cluster. "
            "Returns pod names, node placement, running status, and readiness state. "
            "Use this first to discover available agents before running other diagnostic commands."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {},
            "required": []
        }
    },
    "probe_pod_capabilities": {
        "description": (
            "Probe a specific agent pod to discover what capabilities are available. "
            "Checks which containers are running: agent, system-probe (eBPF/NPM), trace-agent (APM), process-agent. "
            "Use this BEFORE calling tools that require specific capabilities. For example, "
            "process_cache and network_connections require system-probe; trace_agent_info requires trace-agent. "
            "Returns a map of capability names to boolean availability."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name to probe"}
            },
            "required": ["pod"]
        }
    },
    "get_tool_requirements": {
        "description": (
            "Get the capability requirements for tools. "
            "Returns a mapping of tool names to their required capabilities (e.g., system_probe, trace_agent). "
            "Use this to understand which tools will work on a pod based on its capabilities."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {},
            "required": []
        }
    },

    # Tier 1: Workload Data (most useful for troubleshooting)
    "tagger_list": {
        "description": (
            "Retrieve all entities (containers, pods, services) with their associated tags from all tag sources "
            "(kubernetes, docker, ecs, etc). Essential for troubleshooting missing tags, incorrect labels, "
            "tag propagation issues, or verifying autodiscovery tag templates. "
            "Returns entity IDs mapped to their tag key-value pairs."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name (optional, uses first available if not specified)"}
            },
            "required": []
        }
    },
    "workload_list": {
        "description": (
            "Get container and pod metadata from the agent's workload store. "
            "Shows all containers the agent has discovered including their runtime (containerd, docker), "
            "images, resource limits, and security context. Use to verify container discovery, "
            "check why specific containers are missing, or debug autodiscovery issues."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"},
                "verbose": {"type": "boolean", "description": "Include full container details including env vars and mounts", "default": False}
            },
            "required": []
        }
    },
    "status_collector": {
        "description": (
            "Get metrics check execution statistics including run count, execution time, errors, warnings, "
            "and last run timestamp for each check. Use to identify failing checks, slow checks causing delays, "
            "or checks not running at expected intervals. Returns JSON with per-check stats."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"}
            },
            "required": []
        }
    },

    # Tier 2: Diagnostics
    "health": {
        "description": (
            "Quick health check of all agent components. Returns lists of healthy and unhealthy components "
            "including collector, forwarder, DogStatsD, logs-agent, and workloadmeta. "
            "Use as first diagnostic step to identify component failures before diving deeper."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"}
            },
            "required": []
        }
    },
    "config_check": {
        "description": (
            "Show all check configurations loaded by autodiscovery. "
            "Displays resolved check configs with their source (kubernetes annotations, docker labels, config files). "
            "Use to verify autodiscovery is finding your services, debug why checks aren't being scheduled, "
            "or see effective check parameters after template variable resolution."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"}
            },
            "required": []
        }
    },
    "inventory_host": {
        "description": (
            "Get detailed host/node metadata including CPU model and count, total memory, "
            "OS distribution and version, kernel version, virtualization platform, and cloud provider info. "
            "Use for capacity planning, compatibility checks, or correlating performance issues with hardware."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"}
            },
            "required": []
        }
    },
    "check": {
        "description": (
            "Execute a specific metrics check on-demand and return collected metrics, service checks, and events. "
            "Use to test check configuration, validate connectivity to monitored services, or debug why expected metrics are missing. "
            "Supports all core checks (cpu, memory, disk, network) and integrations (postgres, redis, nginx, mysql, etc)."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"},
                "check_name": {"type": "string", "description": "Check name (e.g., 'cpu', 'postgres', 'nginx', 'redis')"}
            },
            "required": ["check_name"]
        }
    },
    "diagnose": {
        "description": (
            "Run comprehensive diagnostic suites to validate agent connectivity and configuration. "
            "Tests include API key validation, endpoint connectivity, DNS resolution, proxy configuration, "
            "and certificate verification. Use when agent can't send data to Datadog or experiencing connection issues."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"},
                "suite": {"type": "string", "description": "Specific suite: 'connectivity', 'port-conflict', 'metadata-collectors', or empty for all"}
            },
            "required": []
        }
    },

    # Tier 3: Specialized
    "status": {
        "description": (
            "Get comprehensive agent status report including version, uptime, hostname, "
            "all running checks with their status, forwarder queue sizes, and system stats. "
            "Verbose output covering all subsystems. Use for full agent state dump when troubleshooting complex issues."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"},
                "section": {"type": "string", "description": "Limit to section: 'collector', 'forwarder', 'aggregator', 'dogstatsd', 'logs-agent'"}
            },
            "required": []
        }
    },
    "version": {
        "description": (
            "Get Datadog Agent version details including major/minor/patch version, "
            "git commit hash, build date, and Go version. "
            "Use to verify agent version matches expected deployment or check compatibility."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"}
            },
            "required": []
        }
    },
    "config": {
        "description": (
            "Retrieve the agent's complete runtime configuration in YAML format. "
            "Shows all settings including API keys (redacted), endpoints, proxy config, log levels, "
            "check intervals, and feature flags. Use to audit configuration, compare between environments, "
            "or debug config-related issues."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"},
                "setting": {"type": "string", "description": "Get specific setting path (e.g., 'api_key', 'logs_config.container_collect_all')"}
            },
            "required": []
        }
    },

    # Tier 4: Trace Agent (APM)
    "trace_agent_info": {
        "description": (
            "Get APM trace-agent configuration and status. Shows tracing endpoints, sampling rates, "
            "service-to-service mappings, and span processing stats. "
            "Use to debug APM tracing issues, missing spans, trace sampling configuration, or trace-agent connectivity. "
            "Accesses trace-agent's info endpoint via Unix Domain Socket."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"}
            },
            "required": []
        }
    },

    # Cluster Agent tools
    "cluster_agent_status": {
        "description": (
            "Get Datadog Cluster Agent status including leader election state, external metrics provider status, "
            "admission controller status, and cluster-level check scheduling info. "
            "Use to debug HPA with Datadog metrics, cluster checks, admission controller webhook issues, "
            "or Horizontal Pod Autoscaler problems."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {},
            "required": []
        }
    },
    "metamap": {
        "description": (
            "Get the Cluster Agent's pod-to-service mapping for Kubernetes service discovery. "
            "Shows which services map to which pods on each node. "
            "Use to debug service tagging, verify endpoints resolution, or troubleshoot service-based autodiscovery."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "node": {"type": "string", "description": "Filter to specific Kubernetes node name"}
            },
            "required": []
        }
    },
    "clusterchecks": {
        "description": (
            "Show how cluster-level checks are distributed across node agents. "
            "Displays which agent is running each cluster check (e.g., kubernetes_state, kube_apiserver_metrics) and their status. "
            "Use to verify cluster check dispatching, debug load balancing, or identify checks stuck on failed nodes."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {},
            "required": []
        }
    },

    # System-Probe Tools (workload visibility via eBPF)
    "process_cache": {
        "description": (
            "Get the process cache from system-probe's network tracer. "
            "Shows all processes tracked by the eBPF-based network monitor including PIDs, command names, "
            "and container IDs. Use to verify which processes are being monitored for network connections, "
            "debug missing process data, or understand what workloads are running on the node. "
            "Requires system-probe to be running with NPM enabled."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"}
            },
            "required": []
        }
    },
    "network_connections": {
        "description": (
            "Get network connection data from system-probe's eBPF network tracer. "
            "Shows active TCP/UDP connections with source/destination addresses, ports, bytes transferred, "
            "and associated process/container info. Use to debug network issues, verify connectivity between services, "
            "or identify unexpected network traffic. Requires system-probe with NPM (Network Performance Monitoring) enabled."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"}
            },
            "required": []
        }
    },
    "ebpf_map_list": {
        "description": (
            "List all eBPF maps currently loaded on the node. "
            "Shows map ID, name, type (hash, array, perf_event, etc.), key/value sizes, and max entries. "
            "Use to verify eBPF programs are loaded correctly, debug map capacity issues, "
            "or understand what eBPF subsystems are active. Works with ALL eBPF maps on the system, not just Datadog's."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"}
            },
            "required": []
        }
    },
    "ebpf_map_dump": {
        "description": (
            "Dump contents of a specific eBPF map by name or ID. "
            "Returns key-value pairs with BTF-based formatting when available (shows struct field names instead of raw bytes). "
            "Use to inspect eBPF map state for debugging, verify data is being collected correctly, "
            "or analyze specific connection/process tracking data. Specify either map name or numeric ID."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"},
                "map_name": {"type": "string", "description": "eBPF map name (e.g., 'conn_stats', 'tcp_stats')"},
                "map_id": {"type": "integer", "description": "eBPF map ID (alternative to map_name)"},
                "pretty": {"type": "boolean", "description": "Pretty-print JSON output", "default": False}
            },
            "required": []
        }
    },
    "module_status": {
        "description": (
            "Get the current status of USM and NPM modules. "
            "Returns whether each module is 'running', 'paused', or 'disabled'. "
            "A 'disabled' module was not enabled at agent startup and cannot be enabled at runtime."
        ),
        "rw": "R",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"}
            },
            "required": []
        }
    },
}

# Write operations (disabled by default)
WRITE_TOOLS = {
    "module_control": {
        "description": (
            "[WRITE] Enable or disable USM/NPM modules at runtime without agent restart. "
            "Use enabled=true to resume a paused module, enabled=false to pause it. "
            "When paused, eBPF probes are bypassed but not unloaded - data collection stops but can quickly resume. "
            "Valid modules: 'usm' (Universal Service Monitoring), 'npm' (Network Performance Monitoring). "
            "Returns previous and current state of the module."
        ),
        "rw": "W",
        "risk": "medium",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"},
                "module": {
                    "type": "string",
                    "description": "Module name: 'usm' or 'npm'",
                    "enum": ["usm", "npm"]
                },
                "enabled": {
                    "type": "boolean",
                    "description": "True to enable/resume the module, False to pause it"
                }
            },
            "required": ["module", "enabled"]
        }
    },
    "config_set": {
        "description": "[WRITE] Set a runtime configuration value. Changes agent behavior.",
        "rw": "W",
        "risk": "medium",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"},
                "setting": {"type": "string", "description": "Setting name"},
                "value": {"type": "string", "description": "New value"}
            },
            "required": ["setting", "value"]
        }
    },
    "secret_refresh": {
        "description": "[WRITE] Refresh secrets from backend.",
        "rw": "W",
        "risk": "low",
        "input_schema": {
            "type": "object",
            "properties": {
                "pod": {"type": "string", "description": "Agent pod name"}
            },
            "required": []
        }
    },
}


@server.list_tools()
async def list_tools() -> list[Tool]:
    """List available tools."""
    tools = []
    for name, spec in TOOLS.items():
        tools.append(Tool(
            name=name,
            description=f"[{spec['rw']}] {spec['description']}",
            inputSchema=spec["input_schema"]
        ))

    # Include write tools only if enabled
    if config.allow_write_operations:
        for name, spec in WRITE_TOOLS.items():
            tools.append(Tool(
                name=name,
                description=f"[{spec['rw']}] {spec['description']}",
                inputSchema=spec["input_schema"]
            ))

    return tools


async def get_pod_or_first(pod: Optional[str]) -> str:
    """Get specified pod or first available."""
    if pod:
        return pod
    pods = await executor.get_agent_pods()
    if not pods:
        raise ValueError("No agent pods found")
    return pods[0]["name"]


@server.call_tool()
async def call_tool(name: str, arguments: dict) -> list[TextContent]:
    """Handle tool calls."""
    logger.info(f"Tool call: {name} with args: {arguments}")

    try:
        result = await _execute_tool(name, arguments)
        return [TextContent(type="text", text=result)]
    except Exception as e:
        logger.error(f"Tool error: {e}")
        return [TextContent(type="text", text=f"Error: {str(e)}")]


async def _execute_tool(name: str, args: dict) -> str:
    """Execute a tool and return result."""

    # Tier 0: Discovery tools
    if name == "list_agent_pods":
        pods = await executor.get_agent_pods()
        return json.dumps(pods, indent=2)

    elif name == "probe_pod_capabilities":
        pod = args.get("pod")
        if not pod:
            return "Error: pod name is required"
        capabilities = await probe_pod_capabilities(pod)
        # Also check cluster agent
        capabilities[Capability.CLUSTER_AGENT] = await check_cluster_agent_available()
        return json.dumps({
            "pod": pod,
            "capabilities": capabilities,
            "hints": {
                "system_probe": "Required for: process_cache, network_connections, ebpf_map_list, ebpf_map_dump",
                "trace_agent": "Required for: trace_agent_info",
                "cluster_agent": "Required for: cluster_agent_status, metamap, clusterchecks",
            }
        }, indent=2)

    elif name == "get_tool_requirements":
        # Return requirements with helpful grouping
        by_capability: dict[str, list[str]] = {}
        for tool, caps in TOOL_REQUIREMENTS.items():
            for cap in caps:
                by_capability.setdefault(cap, []).append(tool)
        return json.dumps({
            "tool_requirements": TOOL_REQUIREMENTS,
            "tools_by_capability": by_capability,
            "no_special_requirements": [
                t for t in TOOLS.keys()
                if t not in TOOL_REQUIREMENTS and t not in ["list_agent_pods", "probe_pod_capabilities", "get_tool_requirements"]
            ]
        }, indent=2)

    # Get pod (use specified or first available)
    pod = await get_pod_or_first(args.get("pod"))

    # Tier 1: Workload Data
    if name == "tagger_list":
        result = await executor.exec_http(pod, "/agent/tagger-list")
        return _format_result(result)

    elif name == "workload_list":
        if args.get("verbose"):
            result = await executor.exec_cli(pod, ["agent", "workload-list", "-v"])
        else:
            result = await executor.exec_http(pod, "/agent/workload-list")
        return _format_result(result)

    elif name == "status_collector":
        result = await executor.exec_cli(pod, ["agent", "status", "collector", "--json"])
        return _format_result(result)

    # Tier 2: Diagnostics
    elif name == "health":
        result = await executor.exec_http(pod, "/agent/status/health")
        return _format_result(result)

    elif name == "config_check":
        result = await executor.exec_http(pod, "/agent/config-check")
        return _format_result(result)

    elif name == "inventory_host":
        result = await executor.exec_http(pod, "/agent/metadata/inventory-host")
        return _format_result(result)

    elif name == "check":
        check_name = args["check_name"]
        result = await executor.exec_cli(pod, ["agent", "check", check_name, "--json"])
        return _format_result(result)

    elif name == "diagnose":
        cmd = ["agent", "diagnose"]
        if args.get("suite"):
            cmd.extend(["--include", args["suite"]])
        result = await executor.exec_cli(pod, cmd)
        return _format_result(result)

    # Tier 3: Specialized
    elif name == "status":
        cmd = ["agent", "status"]
        if args.get("section"):
            cmd.append(args["section"])
        result = await executor.exec_cli(pod, cmd)
        return _format_result(result)

    elif name == "version":
        result = await executor.exec_http(pod, "/agent/version")
        return _format_result(result)

    elif name == "config":
        if args.get("setting"):
            result = await executor.exec_cli(pod, ["agent", "config", "get", args["setting"]])
        else:
            result = await executor.exec_http(pod, "/agent/config")
        return _format_result(result)

    # Tier 4: Trace Agent
    elif name == "trace_agent_info":
        result = await executor.exec_http(
            pod, "/info",
            auth_required=False,
            use_uds=True,
            socket_path="/var/run/datadog/apm.socket"
        )
        return _format_result(result)

    # Cluster Agent tools
    elif name == "cluster_agent_status":
        ca_pod = await executor.get_cluster_agent_pod()
        if not ca_pod:
            return "Error: No cluster agent pod found"
        result = await executor.exec_cli(
            ca_pod,
            ["datadog-cluster-agent", "status"],
            container=config.cluster_agent_container
        )
        return _format_result(result)

    elif name == "metamap":
        ca_pod = await executor.get_cluster_agent_pod()
        if not ca_pod:
            return "Error: No cluster agent pod found"
        cmd = ["datadog-cluster-agent", "metamap"]
        if args.get("node"):
            cmd.append(args["node"])
        result = await executor.exec_cli(
            ca_pod, cmd,
            container=config.cluster_agent_container
        )
        return _format_result(result)

    elif name == "clusterchecks":
        ca_pod = await executor.get_cluster_agent_pod()
        if not ca_pod:
            return "Error: No cluster agent pod found"
        result = await executor.exec_cli(
            ca_pod,
            ["datadog-cluster-agent", "clusterchecks"],
            container=config.cluster_agent_container
        )
        return _format_result(result)

    # System-Probe Tools (eBPF-based workload visibility)
    elif name == "process_cache":
        # Uses system-probe's /debug/process_cache endpoint via UDS
        result = await executor.exec_http(
            pod, "/debug/process_cache",
            auth_required=False,
            use_uds=True,
            socket_path=config.sysprobe_socket,
            container=config.system_probe_container
        )
        return _format_result(result)

    elif name == "network_connections":
        # Uses system-probe's /debug/net_maps endpoint (no registration required)
        result = await executor.exec_http(
            pod, "/debug/net_maps",
            auth_required=False,
            use_uds=True,
            socket_path=config.sysprobe_socket,
            container=config.system_probe_container
        )
        return _format_result(result)

    elif name == "ebpf_map_list":
        # Uses system-probe CLI to list all eBPF maps on the system
        result = await executor.exec_cli(
            pod,
            ["system-probe", "ebpf", "map", "list"],
            container=config.system_probe_container
        )
        return _format_result(result)

    elif name == "ebpf_map_dump":
        # Uses system-probe CLI to dump eBPF map contents with BTF support
        cmd = ["system-probe", "ebpf", "map", "dump"]
        if args.get("map_name"):
            cmd.extend(["name", args["map_name"]])
        elif args.get("map_id"):
            cmd.extend(["id", str(args["map_id"])])
        else:
            return "Error: Either map_name or map_id is required"
        if args.get("pretty"):
            cmd.append("--pretty")
        result = await executor.exec_cli(
            pod, cmd,
            container=config.system_probe_container
        )
        return _format_result(result)

    # Write operations
    elif name == "module_control":
        if not config.allow_write_operations:
            return "Error: Write operations are disabled. Set DD_ALLOW_WRITE=true to enable."
        module = args.get("module")
        enabled = args.get("enabled")
        if module is None or enabled is None:
            return "Error: Both 'module' and 'enabled' parameters are required"
        body = json.dumps({"module": module, "enabled": enabled})
        result = await executor.exec_http_post(
            pod,
            "/network_tracer/module/control",
            body,
            container=config.system_probe_container,
            use_uds=True,
            socket_path=config.sysprobe_socket
        )
        return _format_result(result)

    elif name == "module_status":
        # module_status is read-only, so no need to check allow_write_operations
        result = await executor.exec_http(
            pod,
            "/network_tracer/module/status",
            auth_required=False,
            container=config.system_probe_container,
            use_uds=True,
            socket_path=config.sysprobe_socket
        )
        return _format_result(result)

    elif name == "config_set":
        if not config.allow_write_operations:
            return "Error: Write operations are disabled. Set DD_ALLOW_WRITE=true to enable."
        result = await executor.exec_cli(
            pod,
            ["agent", "config", "set", args["setting"], args["value"]]
        )
        return _format_result(result)

    elif name == "secret_refresh":
        if not config.allow_write_operations:
            return "Error: Write operations are disabled. Set DD_ALLOW_WRITE=true to enable."
        result = await executor.exec_cli(pod, ["agent", "secret", "refresh"])
        return _format_result(result)

    else:
        return f"Unknown tool: {name}"


def _format_result(result: dict, tool_name: str = None) -> str:
    """Format kubectl exec result with helpful error messages."""
    if result["success"]:
        output = result["stdout"]
        # Try to pretty-print JSON
        try:
            data = json.loads(output)
            return json.dumps(data, indent=2)
        except json.JSONDecodeError:
            return output
    else:
        stderr = result["stderr"].lower()
        # Detect container-not-found errors and provide helpful guidance
        if "container" in stderr and ("not found" in stderr or "invalid" in stderr):
            container_hints = {
                "system-probe": (
                    "system-probe container is not running on this pod.\n"
                    "This is required for eBPF-based tools (process_cache, network_connections, ebpf_map_*).\n"
                    "Enable NPM with: spec.features.networkMonitoring.enabled=true in DatadogAgent CRD"
                ),
                "trace-agent": (
                    "trace-agent container is not running on this pod.\n"
                    "This is required for APM tools (trace_agent_info).\n"
                    "Enable APM with: spec.features.apm.enabled=true in DatadogAgent CRD"
                ),
                "process-agent": (
                    "process-agent container is not running on this pod.\n"
                    "Enable process monitoring in agent configuration."
                ),
            }
            for container, hint in container_hints.items():
                if container in stderr:
                    return (
                        f"Error: {hint}\n\n"
                        f"Tip: Use 'probe_pod_capabilities' tool to check what's available on this pod."
                    )

        return f"Error (exit {result['returncode']}): {result['stderr']}\n{result['stdout']}"


# -----------------------------------------------------------------------------
# Main
# -----------------------------------------------------------------------------

async def main():
    """Run the MCP server."""
    logger.info(f"Starting DD-Agent MCP Server")
    logger.info(f"  Kube context: {config.kube_context}")
    logger.info(f"  Namespace: {config.namespace}")
    logger.info(f"  Write operations: {'enabled' if config.allow_write_operations else 'disabled'}")

    async with stdio_server() as (read_stream, write_stream):
        await server.run(
            read_stream,
            write_stream,
            server.create_initialization_options()
        )


if __name__ == "__main__":
    asyncio.run(main())
